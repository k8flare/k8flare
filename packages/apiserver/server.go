package apiserver

import (
	"context"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	authz "github.com/k8flare/k8flare/packages/apiserver-authz"
	installer "github.com/k8flare/k8flare/packages/apiserver-installer"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	"github.com/k8flare/k8flare/packages/edgehost"
	"github.com/k8flare/k8flare/packages/metricsapi"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/authentication/request/bearertoken"
	requnion "k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/authentication/token/union"
	"k8s.io/apiserver/pkg/endpoints/discovery"
	"k8s.io/apiserver/pkg/endpoints/handlers/negotiation"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/client-go/kubernetes/scheme"
)

type Config struct {
	Kine                        *http.Client
	AdminToken                  string
	ReadonlyToken               string
	JoinToken                   string
	Groups                      *http.Client
	OpenAPI                     *http.Client
	CustomResources             *http.Client
	Outbound                    *http.Client
	Tunnel                      *http.Client
	Admission                   *http.Client
	Hooks                       *http.Client
	AccessTeam                  string
	AccessAUD                   string
	AccessGroupsClaim           string
	AccessGroupsPrefix          string
	OIDC                        auth.OIDC
	MaxRequestsInflight         int
	MaxMutatingRequestsInflight int
	ClusterUID                  string
	AuditPolicy                 string

	SecretsEncryptionKeys string
}

const (
	groupsBase          = "https://apigroups.internal"
	openAPIBase         = "https://openapi.internal"
	customResourcesBase = "https://customresources.internal"
	workerHeader        = "X-K8flare-Worker"
)

var versionInfo = version.Info{Major: "1", Minor: "36", GitVersion: "v1.36.4+k8flare", Platform: "js/wasm", GoVersion: "go1.26", Compiler: "gc"}

func seedVaultTokens(ctx context.Context, v *supervisor.Vault, cfg Config) {
	if cfg.ClusterUID != "" && cfg.ClusterUID != "default" {
		return
	}
	if cfg.AdminToken != "" {
		_, _ = v.EnsureToken(ctx, "admin", cfg.AdminToken)
	}
	if cfg.ReadonlyToken != "" {
		_, _ = v.EnsureToken(ctx, "readonly", cfg.ReadonlyToken)
	}
	if cfg.JoinToken != "" {
		_, _ = v.EnsureToken(ctx, "join", cfg.JoinToken)
	}
}

func NewHandler(cfg Config) (http.Handler, error) {
	secrets, err := kine.ParseSecretKeys(cfg.SecretsEncryptionKeys)
	if err != nil {
		return nil, err
	}
	client := &kine.Client{HTTP: cfg.Kine, Secrets: secrets}
	v := supervisor.NewVault(client)
	access := auth.Access{Team: cfg.AccessTeam, Audience: cfg.AccessAUD, HTTP: cfg.Outbound, GroupsClaim: cfg.AccessGroupsClaim, GroupsPrefix: cfg.AccessGroupsPrefix}
	oidc := cfg.OIDC
	oidc.HTTP = cfg.Outbound
	sa := auth.ServiceAccountToken{HMAC: []byte(cfg.AdminToken), Objects: auth.NewServiceAccountObjects(auth.KineObjects{Client: client})}
	tokens := union.New(auth.AdminToken(cfg.AdminToken), auth.ReadonlyToken(cfg.ReadonlyToken), auth.ComponentTokens{Key: []byte(cfg.AdminToken)}, auth.VaultToken{Vault: v}, auth.NodeToken{Vault: v}, sa, oidc, access)
	edgeCert := auth.EdgeClientCert{ClientCA: func(ctx context.Context) ([]byte, error) { return v.CAPEM(ctx, "client-ca") }}
	authn := requnion.New(bearertoken.New(tokens), edgeCert, access)
	authorizer := authz.New(client)
	audits, err := newAuditor(cfg.AuditPolicy, os.Stdout)
	if err != nil {
		return nil, fmt.Errorf("audit policy: %w", err)
	}
	mux := http.NewServeMux()
	installHealth(mux, client)
	mux.HandleFunc("/.well-known/openid-configuration", sa.ServeOpenID)
	mux.HandleFunc("/openid/v1/jwks", sa.ServeJWKS)
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(versionInfo)
	})
	addresses := discovery.DefaultAddresses{DefaultAddress: "k8flare"}
	legacyAPI := wrapAggregated(discovery.NewLegacyRootAPIHandler(addresses, scheme.Codecs, "/api"), true, nil)
	mux.Handle("/api", legacyAPI)
	mux.Handle("/api/", serveAPIRoot(legacyAPI, forwardTo(cfg.Groups, groupsBase, "apiserver-core")))
	apisRoot := wrapAggregated(rootAPIs(addresses, cfg), false, addDynamicAggregated(cfg))
	mux.Handle("/apis", apisRoot)
	metrics := metricsapi.Handler{Store: client, Tunnel: cfg.Tunnel}
	mux.Handle("/apis/metrics.k8s.io", metrics)
	mux.Handle("/apis/metrics.k8s.io/", metrics)
	mux.HandleFunc("/internal/metrics/scrape", metrics.Scrape)
	mux.HandleFunc("/internal/loadbalancer/provision", func(w http.ResponseWriter, r *http.Request) {
		edgehost.ProvisionServices(w, r, client)
	})
	mux.HandleFunc("/internal/gateway/provision", func(w http.ResponseWriter, r *http.Request) {
		edgehost.ProvisionGateways(w, r, client)
	})
	mux.HandleFunc("/internal/extensions/dispatch", func(w http.ResponseWriter, r *http.Request) {
		edgehost.DispatchExtensions(w, r, client, cfg.Hooks)
	})
	mux.HandleFunc("/internal/queue/plan", edgehost.PlanQueue)
	mux.HandleFunc("/internal/queue/followup", edgehost.FollowUp)
	mux.HandleFunc("/internal/r2/mint", edgehost.MintR2)
	mux.HandleFunc("/internal/pods/merge-env", edgehost.MergePodEnvHandler)
	mux.HandleFunc("/internal/nodes/vm-plan", edgehost.PlanVMsHandler)
	mux.Handle("/apis/", serveAPIs(apisRoot, groupRouter(cfg)))
	serveDiscovery(mux)
	mux.Handle("/openapi/v2", openAPIV2(cfg))
	mux.Handle("/openapi/v2/", openAPIV2(cfg))
	mux.Handle("/openapi/v3", openAPIV3Root(cfg))
	mux.Handle("/openapi/v3/", openAPIV3Router(cfg))
	root := http.NewServeMux()
	kubeletSupervisor := supervisor.New(v, cfg.JoinToken)
	kubeletSupervisor.ClientCerts = edgeCert
	kubeletSupervisor.ServiceAccounts = bearertoken.New(tokens)
	mux.HandleFunc("/internal/kubelet-client", kubeletSupervisor.KubeletClient)
	installSecretsEncrypt(mux, client)
	installTokens(mux, v)
	installEdgeCertificate(mux, v)
	installCertificates(mux, v)
	installSnapshots(mux, client)
	kubeletSupervisor.Register(root)
	root.Handle("/", withFrontFilters(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-K8flare-Stream-Locate") == "1" {
			edgehost.LocateStream(w, r, client, cfg.Admission)
			return
		}
		mux.ServeHTTP(w, r)
	}), authn, authorizer, audits, cfg.MaxRequestsInflight, cfg.MaxMutatingRequestsInflight))
	var once sync.Once
	var keyReady atomic.Bool
	return recoverPanics(redirectBareProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			seedVaultTokens(r.Context(), v, cfg)
		})
		if !keyReady.Load() && auth.InstallServiceAccountKey(r.Context(), client, []byte(cfg.AdminToken)) == nil {
			keyReady.Store(true)
		}
		if edgehost.Proxy(w, r, client, cfg.Tunnel) {
			return
		}
		root.ServeHTTP(w, r)
	}))), nil
}

func serveAPIRoot(legacy, rest http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/" {
			legacy.ServeHTTP(w, r)
			return
		}
		rest.ServeHTTP(w, r)
	})
}

func serveAPIs(root, rest http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apis" || r.URL.Path == "/apis/" {
			root.ServeHTTP(w, r)
			return
		}
		rest.ServeHTTP(w, r)
	})
}

func redirectBareProxy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && strings.HasSuffix(r.URL.Path, "/proxy") && !strings.HasSuffix(r.URL.Path, "/proxy/") {
			loc := r.URL.Path + "/"
			if r.URL.RawQuery != "" {
				loc += "?" + r.URL.RawQuery
			}
			w.Header().Set("Location", loc)
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serveDiscovery answers the per-group discovery documents from the
// generated table instead of forwarding them. A client walks every group
// before its first request, and forwarding that walk cold-starts every
// group worker at once, which is what exhausts the isolates in production.
func serveDiscovery(mux *http.ServeMux) {
	versions := map[string][]string{}
	for _, sgv := range registry.Served {
		gv, resources := sgv.GV, sgv.Resources
		lister := discovery.APIResourceListerFunc(func() []metav1.APIResource { return resources })
		handler := discovery.NewAPIVersionHandler(scheme.Codecs, gv, lister)
		if gv.Group == "" {
			mux.Handle("/api/"+gv.Version, handler)
			continue
		}
		mux.Handle("/apis/"+gv.Group+"/"+gv.Version, handler)
		versions[gv.Group] = append(versions[gv.Group], gv.Version)
	}
	for group, vers := range versions {
		g := metav1.APIGroup{Name: group}
		for _, v := range vers {
			g.Versions = append(g.Versions, metav1.GroupVersionForDiscovery{GroupVersion: group + "/" + v, Version: v})
		}
		g.PreferredVersion = g.Versions[0]
		mux.Handle("/apis/"+group, discovery.NewAPIGroupHandler(scheme.Codecs, g))
	}
}

func groupRouter(cfg Config) http.Handler {
	workers := map[string]string{}
	for _, sgv := range registry.Served {
		if sgv.GV.Group == "" {
			continue
		}
		group, _, _ := strings.Cut(sgv.GV.Group, ".")
		workers[sgv.GV.Group] = "apiserver-" + group
	}
	custom := forwardTo(cfg.CustomResources, customResourcesBase, "")
	aggregate := forwardTo(cfg.Groups, groupsBase, "apiserver-apiregistration")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		group, version := apiGroupAndVersion(r.URL.Path)
		if worker, ok := workers[group]; ok {
			forwardTo(cfg.Groups, groupsBase, worker).ServeHTTP(w, r)
			return
		}
		store := kineStore(cfg.Kine)
		if version != "" {
			if _, ok := remoteAPIService(r.Context(), store, group, version); ok {
				aggregate.ServeHTTP(w, r)
				return
			}
		} else if hasRemoteAPIServiceGroup(r.Context(), store, group) {
			aggregate.ServeHTTP(w, r)
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
				continue
			}
			req.Header.Set("Accept", "application/json")
			resp, err := src.client.Do(req)
			if err != nil {
				continue
			}
			var discovery struct {
				Paths map[string]json.RawMessage `json:"paths"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&discovery)
			resp.Body.Close()
			if decodeErr != nil || resp.StatusCode != http.StatusOK {
				continue
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
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(body)
	})
}

func rootAPIs(addresses discovery.Addresses, cfg Config) http.Handler {
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
		if cfg.CustomResources != nil {
			req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, customResourcesBase+"/apis", nil)
			if err == nil {
				req.Header.Set("Accept", "application/json")
				resp, err := cfg.CustomResources.Do(req)
				if err == nil {
					var list metav1.APIGroupList
					decodeErr := json.NewDecoder(resp.Body).Decode(&list)
					resp.Body.Close()
					if decodeErr == nil && resp.StatusCode == http.StatusOK {
						groups = append(groups, list.Groups...)
					}
				}
			}
		}
		groups = append(groups, remoteAPIServiceGroups(r.Context(), kineStore(cfg.Kine))...)
		groups = append(groups, metricsapi.APIGroup())
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

func openAPIV2(cfg Config) http.Handler {
	return cacheOpenAPIJSON(openAPIV2Uncached(cfg))
}

func cacheOpenAPIJSON(next http.Handler) http.Handler {
	var mu sync.Mutex
	var cached []byte
	var at time.Time
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantsProtobufOpenAPI(r) {
			next.ServeHTTP(w, r)
			return
		}
		mu.Lock()
		if cached != nil && time.Since(at) < 30*time.Second {
			body := cached
			mu.Unlock()
			writeOpenAPIJSON(w, body)
			return
		}
		mu.Unlock()
		rec := &openAPICapture{header: http.Header{}}
		next.ServeHTTP(rec, r)
		if rec.code == 0 {
			rec.code = http.StatusOK
		}
		if rec.code == http.StatusOK {
			mu.Lock()
			cached = append([]byte(nil), rec.body...)
			at = time.Now()
			mu.Unlock()
		}
		for k, vs := range rec.header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set("Cache-Control", "public, max-age=30")
		w.WriteHeader(rec.code)
		_, _ = w.Write(rec.body)
	})
}

func writeOpenAPIJSON(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

type openAPICapture struct {
	header http.Header
	code   int
	body   []byte
}

func (c *openAPICapture) Header() http.Header { return c.header }
func (c *openAPICapture) Write(b []byte) (int, error) {
	if c.code == 0 {
		c.code = http.StatusOK
	}
	c.body = append(c.body, b...)
	return len(b), nil
}
func (c *openAPICapture) WriteHeader(status int) { c.code = status }

func openAPIV2Uncached(cfg Config) http.Handler {
	builtin := forwardTo(cfg.OpenAPI, openAPIBase, "")
	if cfg.CustomResources == nil {
		return builtin
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantsProtobufOpenAPI(r) {
			builtin.ServeHTTP(w, r)
			return
		}
		base, err := fetchOpenAPI(r.Context(), cfg.OpenAPI, openAPIBase+"/openapi/v2")
		if err != nil {
			builtin.ServeHTTP(w, r)
			return
		}
		extra, err := fetchOpenAPI(r.Context(), cfg.CustomResources, customResourcesBase+"/openapi/v2")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(base)
			return
		}
		merged, err := mergeOpenAPIV2Definitions(base, extra)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(base)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(merged)
	})
}

func wantsProtobufOpenAPI(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "protobuf") || strings.Contains(accept, "application/com.github.proto-openapi")
}

func fetchOpenAPI(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openapi %s: %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func mergeOpenAPIV2Definitions(base, extra []byte) ([]byte, error) {
	var dst, src map[string]any
	if err := json.Unmarshal(base, &dst); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(extra, &src); err != nil {
		return nil, err
	}
	dstDefs, _ := dst["definitions"].(map[string]any)
	srcDefs, _ := src["definitions"].(map[string]any)
	if dstDefs == nil {
		dstDefs = map[string]any{}
		dst["definitions"] = dstDefs
	}
	for name, def := range srcDefs {
		if _, ok := dstDefs[name]; !ok {
			dstDefs[name] = def
		}
	}
	return json.Marshal(dst)
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
