//go:build js && wasm

package main

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

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
// cross the Loader env clone, S2 item 3a).
//
// The STORAGE Fetcher is pulled from the CURRENT request's env
// (BindingFromContext) and the client built per call -- NOT memoized.
// This binary is resident now (one instance serves every request, see
// main()), so a Fetcher captured from an earlier request would be reused
// from a later request's dispatch, which Cloudflare rejects with "Cannot
// perform I/O on behalf of a different request" once the capturing
// request's IoContext ends. That was the S24 resident-apiserver
// regression: the old sync.OnceValue-memoized client worked only within
// the ~15s pump window of whichever request first instantiated the
// isolate, then every storage write threw and stalled KCM's pod creation.
// Sourcing the binding from req.Context() also attributes each storage
// subrequest to its originating request, keeping per-invocation
// subrequest counts bounded (an apiserver fans out to kine on behalf of
// kubectl + kubelet + KCM + scheduler + gc). req.Context() carries the
// env because pkg/apiserver handlers thread r.Context() into storage.go's
// NewRequestWithContext calls and cfruntime's dispatch attaches this
// request's env to it.
func storageDo(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-K8flare-Cluster", clusterDOName())
	client := cffetch.NewClient(cffetch.WithBinding(cloudflare.BindingFromContext(req.Context(), "STORAGE"))).
		HTTPClient(cffetch.RedirectModeFollow)
	return client.Do(req)
}

// getTokens returns every currently-valid token for this cluster: the
// env token for the default cluster (rotation is `wrangler secret put`),
// or the per-cluster token vault (a kine value at /ca/cluster-tokens,
// read through storageDo and parsed by apiserver.DecodeVaultTokens) for
// provisioned ones.
//
// Cached for tokenCacheTTL, NOT memoized for the isolate's lifetime. It
// was a sync.OnceValue until 2026-07-27, which froze a provisioned
// cluster's token list at the first read: the loader id for this binary
// is apiserver:<doName>@<sha> with no token fingerprint (unlike the
// controllers', which re-key on rotation), so nothing ever replaced the
// isolate and a rotated token was rejected here forever -- found live
// bringing up the cluster operator's rotation path (the gateway door
// accepted the new token, this layer 401'd it). The TTL matches the TS
// door's own token cache (clusters/tokens.ts), so revocation propagates
// on the same documented timescale at both layers.
//
// KNOWN GAP (unchanged, resident shape): the vault read calls storageDo
// with a context-less http.NewRequest, so BindingFromContext falls back
// to the Go.run-time global STORAGE binding instead of the caller's
// request-scoped one. The refresh cadence does not change that hazard,
// it just exercises it more than once.
const tokenCacheTTL = 60 * time.Second

var (
	tokenCacheMu      sync.Mutex
	tokenCacheValue   []string
	tokenCacheExpires time.Time
)

func getTokens() []string {
	tokenCacheMu.Lock()
	defer tokenCacheMu.Unlock()
	if time.Now().Before(tokenCacheExpires) {
		return tokenCacheValue
	}
	tokenCacheValue = readTokens()
	tokenCacheExpires = time.Now().Add(tokenCacheTTL)
	return tokenCacheValue
}

func readTokens() []string {
	// Every cluster -- "default" included -- reads its own vault: the
	// K3S_TOKEN Worker secret is abolished (2026-07-27); tokens are
	// minted by the cluster operator (pkg/controllers/clusterop). An empty or
	// unreadable DEFAULT vault falls back to the K3S_TOKEN secret, then the dev token (the
	// secretless dev/CI posture -- and, on a transient vault-read
	// failure, the TS gateway has already door-verified the caller, so
	// this layer degrading to dev-only is defense-in-depth, not the
	// gate). Provisioned clusters keep failing closed.
	tokens := func() []string {
		req, err := http.NewRequest(http.MethodGet, "http://do.internal/key/ca/cluster-tokens", nil)
		if err != nil {
			return nil
		}
		resp, err := storageDo(req)
		if err != nil {
			return nil
		}
		defer resp.Body.Close()
		decoded, err := apiserver.DecodeVaultTokens(resp.Body)
		if err != nil {
			return nil
		}
		return decoded
	}()
	if clusterDOName() == "default" {
		// The default cluster ALSO accepts the K3S_TOKEN Worker secret
		// (baked pristine by loader/apiserver.ts) -- the one intuitive
		// always-valid root token (2026-07-27 decision). Dev fallback
		// only when neither a vault token nor the secret exists.
		if envTok := cloudflare.Getenv("K3S_TOKEN"); envTok != "" {
			tokens = append(tokens, envTok)
		}
		if len(tokens) == 0 {
			tokens = []string{"k8flare-dev-token"}
		}
	}
	return tokens
}

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
