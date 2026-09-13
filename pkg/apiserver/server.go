package apiserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/authentication/token/union"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/client-go/kubernetes/scheme"
)

type Config struct {
	// Kine is the HTTP client that reaches the Cluster Durable Object.
	Kine *http.Client
	// AdminToken authenticates kubectl.
	AdminToken string
	// JoinToken is what a k3s agent presents to the supervisor endpoints.
	JoinToken string
	// Kubelet is how pods/log reaches a node.
	Kubelet KubeletProxy
}

var versionInfo = version.Info{Major: "1", Minor: "36", GitVersion: "v1.36.4+k8flare", Platform: "js/wasm", GoVersion: "go1.26", Compiler: "gc"}

func NewHandler(cfg Config) (http.Handler, error) {
	kine := &KineClient{HTTP: cfg.Kine}
	v := newVault(kine)
	tokens := union.New(adminToken(cfg.AdminToken), nodeToken{v})
	mux := http.NewServeMux()
	stores, err := installAPI(mux, kine, tokens, cfg.Kubelet)
	if err != nil {
		return nil, err
	}
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
	root := http.NewServeMux()
	(&supervisor{vault: v, joinToken: cfg.JoinToken}).register(root)
	root.Handle("/", withAuth(ensureNamespaces(stores["namespaces"], mux), tokens))
	return recoverPanics(root), nil
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
