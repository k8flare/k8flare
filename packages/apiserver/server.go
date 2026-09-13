package apiserver

import (
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	authz "github.com/k8flare/k8flare/packages/apiserver-authz"
	installer "github.com/k8flare/k8flare/packages/apiserver-installer"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/authentication/token/union"
	"k8s.io/apiserver/pkg/endpoints/discovery"
	"k8s.io/apiserver/pkg/endpoints/handlers/negotiation"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/client-go/kubernetes/scheme"
)

type Config struct {
	Kine            *http.Client
	AdminToken      string
	ReadonlyToken   string
	JoinToken       string
	Groups          *http.Client
	OpenAPI         *http.Client
	CustomResources *http.Client
}

const (
	groupsBase          = "https://apigroups.internal"
	openAPIBase         = "https://openapi.internal"
	customResourcesBase = "https://customresources.internal"
	workerHeader        = "X-K8flare-Worker"
)

var versionInfo = version.Info{Major: "1", Minor: "36", GitVersion: "v1.36.4+k8flare", Platform: "js/wasm", GoVersion: "go1.26", Compiler: "gc"}

func NewHandler(cfg Config) (http.Handler, error) {
	client := &kine.Client{HTTP: cfg.Kine}
	v := supervisor.NewVault(client)
	tokens := union.New(auth.AdminToken(cfg.AdminToken), auth.ReadonlyToken(cfg.ReadonlyToken), auth.NodeToken{Vault: v})
	authorizer := authz.New(client)
	mux := http.NewServeMux()
	for _, p := range []string{"/healthz", "/readyz", "/livez"} {
		mux.HandleFunc(p, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("ok"))
		})
	}
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(versionInfo)
	})
	addresses := discovery.DefaultAddresses{DefaultAddress: "k8flare"}
	mux.Handle("/api", discovery.NewLegacyRootAPIHandler(addresses, scheme.Codecs, "/api"))
	mux.Handle("/api/", forwardTo(cfg.Groups, groupsBase, "apiserver-core"))
	mux.Handle("/apis", rootAPIs(addresses, cfg.CustomResources))
	mux.Handle("/apis/", groupRouter(cfg))
	mux.Handle("/openapi/v2", forwardTo(cfg.OpenAPI, openAPIBase, ""))
	mux.Handle("/openapi/v2/", forwardTo(cfg.OpenAPI, openAPIBase, ""))
	mux.Handle("/openapi/v3", openAPIV3Root(cfg))
	mux.Handle("/openapi/v3/", openAPIV3Router(cfg))
	root := http.NewServeMux()
	supervisor.New(v, cfg.JoinToken).Register(root)
	root.Handle("/", auth.WithAuth(auth.WithRequestInfo(auth.WithAuthorization(mux, authorizer)), tokens))
	return recoverPanics(root), nil
}

func groupRouter(cfg Config) http.Handler {
	workers := map[string]string{}
	for _, sgv := range registry.Served {
		group, _, _ := strings.Cut(sgv.GV.Group, ".")
		workers[sgv.GV.Group] = "apiserver-" + group
	}
	custom := forwardTo(cfg.CustomResources, customResourcesBase, "")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		group, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/apis/"), "/")
		if worker, ok := workers[group]; ok {
			forwardTo(cfg.Groups, groupsBase, worker).ServeHTTP(w, r)
			return
		}
		custom.ServeHTTP(w, r)
	})
}

func openAPIV3GroupFromPath(path string) (group string, ok bool) {
	rest := strings.TrimPrefix(path, "/openapi/v3/")
	switch {
	case rest == "api/v1" || strings.HasPrefix(rest, "api/v1/"):
		return "", true
	case strings.HasPrefix(rest, "apis/"):
		parts := strings.SplitN(strings.TrimPrefix(rest, "apis/"), "/", 2)
		if len(parts) < 2 {
			return "", false
		}
		return parts[0], true
	default:
		return "", false
	}
}

func openAPIV3Router(cfg Config) http.Handler {
	served := map[string]bool{}
	for _, sgv := range registry.Served {
		served[sgv.GV.Group] = true
	}
	openAPIWorker := forwardTo(cfg.OpenAPI, openAPIBase, "")
	customWorker := forwardTo(cfg.CustomResources, customResourcesBase, "")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		group, ok := openAPIV3GroupFromPath(r.URL.Path)
		if ok && served[group] {
			openAPIWorker.ServeHTTP(w, r)
			return
		}
		customWorker.ServeHTTP(w, r)
	})
}

func openAPIV3Root(cfg Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths := map[string]json.RawMessage{}
		sources := []struct {
			client *http.Client
			base   string
		}{{cfg.OpenAPI, openAPIBase}}
		if cfg.CustomResources != nil {
			sources = append(sources, struct {
				client *http.Client
				base   string
			}{cfg.CustomResources, customResourcesBase})
		}
		for _, src := range sources {
			req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, src.base+"/openapi/v3", nil)
			if err != nil {
				responsewriters.InternalError(w, r, err)
				return
			}
			req.Header.Set("Accept", "application/json")
			resp, err := src.client.Do(req)
			if err != nil {
				responsewriters.InternalError(w, r, err)
				return
			}
			var discovery struct {
				Paths map[string]json.RawMessage `json:"paths"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&discovery)
			resp.Body.Close()
			if decodeErr != nil {
				responsewriters.InternalError(w, r, fmt.Errorf("%s /openapi/v3: %w", src.base, decodeErr))
				return
			}
			for k, v := range discovery.Paths {
				paths[k] = v
			}
		}
		body, err := json.Marshal(struct {
			Paths map[string]json.RawMessage `json:"paths"`
		}{Paths: paths})
		if err != nil {
			responsewriters.InternalError(w, r, err)
			return
		}
		etag := fmt.Sprintf("%x", sha512.Sum512(body))
		w.Header().Set("Etag", strconv.Quote(etag))
		if r.Header.Get("If-None-Match") == strconv.Quote(etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
}

func rootAPIs(addresses discovery.Addresses, customResources *http.Client) http.Handler {
	root := discovery.NewRootAPIsHandler(addresses, scheme.Codecs)
	for _, sgv := range registry.Served {
		if sgv.GV.Group != "" {
			root.AddGroup(installer.APIGroup(sgv.GV))
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		groups, err := root.Groups(r.Context(), r)
		if err != nil {
			responsewriters.InternalError(w, r, err)
			return
		}
		if customResources != nil {
			req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, customResourcesBase+"/apis", nil)
			req.Header.Set("Accept", "application/json")
			resp, err := customResources.Do(req)
			if err != nil {
				responsewriters.InternalError(w, r, err)
				return
			}
			defer resp.Body.Close()
			var list metav1.APIGroupList
			if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
				responsewriters.InternalError(w, r, fmt.Errorf("customresources /apis: %w", err))
				return
			}
			groups = append(groups, list.Groups...)
		}
		responsewriters.WriteObjectNegotiated(scheme.Codecs, negotiation.DefaultEndpointRestrictions, schema.GroupVersion{}, w, r, http.StatusOK, &metav1.APIGroupList{Groups: groups}, false)
	})
}

func forwardTo(client *http.Client, base, worker string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			http.NotFound(w, r)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, base+r.URL.RequestURI(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		req.Header = r.Header.Clone()
		req.Header.Del("Accept-Encoding")
		req.ContentLength = r.ContentLength
		auth.ForwardRemoteUser(r.Context(), req.Header)
		if worker != "" {
			req.Header.Set(workerHeader, worker)
		}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.Header().Del("Content-Encoding")
		w.Header().Del("Content-Length")
		w.WriteHeader(resp.StatusCode)
		flusher, _ := w.(http.Flusher)
		if flusher != nil {
			flusher.Flush()
		}
		buf := make([]byte, 32<<10)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			if err != nil {
				return
			}
		}
	})
}

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				println("apiserver: panic serving", r.Method, r.URL.Path, ":", fmt.Sprint(rec), "\n", string(debug.Stack()))
				responsewriters.ErrorNegotiated(apierrors.NewInternalError(fmt.Errorf("%v", rec)), scheme.Codecs, schema.GroupVersion{}, w, r)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
