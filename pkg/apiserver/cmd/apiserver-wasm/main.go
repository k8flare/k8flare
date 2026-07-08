//go:build js && wasm

package main

import (
	"net/http"
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

// getR2Config resolves this cluster's R2Config from the Workers
// environment, falling back to apiserver.ApplyR2DevFallback's dev
// placeholders for any unset field. Passed to pkg/apiserver via
// apiserver.ServerConfig.R2Config rather than called directly there --
// see r2.go's currentR2Config doc comment for why.
var getR2Config = sync.OnceValue(func() apiserver.R2Config {
	return apiserver.ApplyR2DevFallback(apiserver.R2Config{
		AccountID:       cloudflare.Getenv("R2_ACCOUNT_ID"),
		AccessKeyID:     cloudflare.Getenv("R2_ACCESS_KEY_ID"),
		SecretAccessKey: cloudflare.Getenv("R2_SECRET_ACCESS_KEY"),
		Bucket:          cloudflare.Getenv("R2_BUCKET"),
	})
})

func main() {
	mux := apiserver.NewServer(apiserver.ServerConfig{
		StorageDo: storageDo,
		Tokens:    getTokens,
		R2Config:  getR2Config,
		ClusterBasePath: func() string {
			return cloudflare.Getenv("CLUSTER_BASE_PATH")
		},
		// Scopes this cluster's R2 object keys under clusters/<doName>/
		// (default keeps unprefixed keys) -- see
		// pkg/apiserver/r2.go's currentClusterStoragePrefix.
		ClusterStoragePrefix: func() string {
			if n := clusterDOName(); n != "default" {
				return "clusters/" + n + "/"
			}
			return ""
		},
	})
	workers.Serve(mux)
}
