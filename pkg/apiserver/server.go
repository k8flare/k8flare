package apiserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
)

type Config struct {
	// Kine is the HTTP client that reaches the Cluster Durable Object.
	Kine *http.Client
	// AdminToken authenticates kubectl.
	AdminToken string
	// JoinToken is what a k3s agent presents to the supervisor endpoints.
	JoinToken string
}

var versionInfo = map[string]string{
	"major": "1", "minor": "36", "gitVersion": "v1.36.4+k8flare",
	"platform": "js/wasm", "goVersion": "go1.26", "compiler": "gc",
}

func NewHandler(cfg Config) (http.Handler, error) {
	kine := &KineClient{HTTP: cfg.Kine}
	v := newVault(kine)
	authenticators := []Authenticator{adminAuthenticator(cfg.AdminToken), nodeAuthenticator(v)}
	mux := http.NewServeMux()
	if err := installAPI(mux, kine, authenticators); err != nil {
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
	root.Handle("/", withAuth(ensureNamespaces(kine, mux), authenticators...))
	return recoverPanics(root), nil
}

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				println("apiserver: panic serving", r.Method, r.URL.Path, ":", fmt.Sprint(rec), "\n", string(debug.Stack()))
				writeStatus(w, http.StatusInternalServerError, "InternalError", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
