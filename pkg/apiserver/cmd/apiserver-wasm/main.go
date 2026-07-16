//go:build js && wasm

package main

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
	"sync"

	"github.com/k8flare/k8flare/pkg/apiserver"
	"github.com/k8flare/k8flare/pkg/cfruntime"
	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
	cffetch "github.com/k8flare/k8flare/pkg/cfruntime/cloudflare/fetch"
)

// clusterDOName is this dynamic-worker instance's multi-cluster identity
// (one Loader isolate per cluster, loader/apiserver.ts). Env is only
// readable during request handling, so this can't be resolved at init.
func clusterDOName() string {
	return cloudflare.GetenvDefault("CLUSTER_DO_NAME", "default")
}

// storageDo routes a kine request to THIS cluster's Cluster DO: the
// STORAGE env Fetcher is the parent script's ClusterLoopback entrypoint,
// which dispatches on the X-K8flare-Cluster header (DO namespaces cannot
// cross the Loader env clone, S2 item 3a). The client is built lazily
// (env bindings are only reachable once a request has actually arrived)
// and memoized for this instance's lifetime via sync.OnceValue.
var storageClient = sync.OnceValue(func() *http.Client {
	return cffetch.NewClient(cffetch.WithBinding(cloudflare.GetBinding("STORAGE"))).
		HTTPClient(cffetch.RedirectModeFollow)
})

func storageDo(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-K8flare-Cluster", clusterDOName())
	return storageClient().Do(req)
}

// getTokens returns every currently-valid token for this cluster: the
// env token for the default cluster (rotation is `wrangler secret put`),
// or the per-cluster token vault (a kine value at /ca/cluster-tokens,
// read through storageDo and parsed by apiserver.DecodeVaultTokens) for
// provisioned ones. Memoized via sync.OnceValue -- this binary is
// instantiated fresh per request (S19's per-request worker.mjs
// contract), so the memo only spans one request's lifetime anyway.
var getTokens = sync.OnceValue(func() []string {
	if clusterDOName() == "default" {
		return []string{cloudflare.GetenvDefault("K3S_TOKEN", "k8flare-dev-token")}
	}
	req, err := http.NewRequest(http.MethodGet, "http://do.internal/key/ca/cluster-tokens", nil)
	if err != nil {
		return nil // fail closed: no readable vault means nothing authenticates
	}
	resp, err := storageDo(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	tokens, err := apiserver.DecodeVaultTokens(resp.Body)
	if err != nil {
		return nil
	}
	return tokens
})

// recoverMiddleware turns a panic in any request handler into a 500
// Status response instead of letting it crash the whole Go program.
// This is REQUIRED for the resident execution shape below: unlike the
// old per-request shape (a fresh instance per request, discarded after),
// this binary now serves EVERY request from one long-lived instance, so
// an unrecovered panic would take down the shared instance and fail
// every subsequent request until the isolate is recycled (S19's
// reused-instance warning, docs/platform-verification.md). Real
// kube-apiserver installs the same panic-recovery filter regardless.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				println("apiserver: panic serving " + r.Method + " " + r.URL.Path + ": ")
				println(string(debug.Stack()))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"kind": "Status", "apiVersion": "v1", "status": "Failure",
					"message": "internal server error", "reason": "InternalError", "code": 500,
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func main() {
	mux := apiserver.NewServer(apiserver.ServerConfig{
		StorageDo: storageDo,
		Tokens:    getTokens,
		ClusterBasePath: func() string {
			return cloudflare.Getenv("CLUSTER_BASE_PATH")
		},
	})
	// Resident, not per-request: ONE Go instance serves every request
	// over this isolate's lifetime (paired with the resident bootstrap in
	// loader/apiserver.ts). The old per-request shape instantiated a
	// fresh ~40MB Go linear memory PER concurrent request on top of the
	// shared compiled module, and under real controller load those stacked
	// past the 128MiB production isolate cap -- the S24 OOM that stalled
	// KCM's writes. One resident instance means one linear memory that Go
	// GCs between requests, regardless of concurrency; concurrent requests
	// are handled by concurrent goroutines in that single instance, the
	// same as any Go http server. The apiserver has NO background
	// goroutines (verified: no `go func`/ticker anywhere in pkg/apiserver),
	// so each request is fully handled within its own dispatch's IoContext
	// and no pump window is needed for correctness -- the bootstrap's pump
	// only keeps the instance warm between requests (event-armed, parks on
	// idle, so scale-to-zero holds).
	workers.ServeNonBlock(recoverMiddleware(mux))
	workers.Ready()
	select {} // park forever; serve every dispatch from this one instance
}
