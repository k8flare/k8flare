//go:build js && wasm

package main

import (
	"net/http"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/k8flare/k8flare/pkg/apiserver"
	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
	"github.com/syumai/workers"
	"github.com/syumai/workers/cloudflare"
)

// getToken returns the K3S_TOKEN from Workers env binding.
// Must be called during request handling (not at init time).
var cachedToken string

func getToken() string {
	if cachedToken != "" {
		return cachedToken
	}
	t := cloudflare.Getenv("K3S_TOKEN")
	if t == "" {
		t = "k8flare-dev-token" // fallback for dev
	}
	cachedToken = t
	return t
}

// r2DevBucket/r2DevAccountID/r2DevAccessKeyID/r2DevSecretAccessKey are dev
// fallbacks for R2_BUCKET/R2_ACCOUNT_ID/R2_ACCESS_KEY_ID/R2_SECRET_ACCESS_KEY,
// mirroring getToken's k8flare-dev-token fallback: local `wrangler dev`
// exercises the full PVC-bind -> mint-credential -> JWT-signing mechanism
// end to end without a real R2 bucket or R2 API token configured. A
// credential minted from these values will not authenticate against real
// R2 (the "secret" isn't a real R2 API token's secret) -- see
// docs/cost-model.md's R2 section and CLAUDE.md's local-dev-pitfalls list.
const (
	r2DevAccountID       = "dev-account-id"
	r2DevAccessKeyID     = "dev-access-key-id"
	r2DevSecretAccessKey = "dev-secret-access-key"
	r2DevBucket          = "k8flare-dev-bucket"
)

// cachedR2Config memoizes getR2Config's result the same way cachedToken
// does for getToken, and for the same reason (Workers env bindings are
// only reachable once a request has actually arrived).
var (
	cachedR2Config   apiserver.R2Config
	cachedR2Resolved bool
)

// getR2Config returns this cluster's R2Config, falling back to the r2Dev*
// constants above for any field left unset, so local `wrangler dev`
// exercises the same code path a real deployment does. Passed to
// pkg/apiserver via SetR2ConfigFunc rather than called directly there --
// see r2.go's currentR2Config doc comment for why.
func getR2Config() apiserver.R2Config {
	if cachedR2Resolved {
		return cachedR2Config
	}
	cfg := apiserver.R2Config{
		AccountID:       cloudflare.Getenv("R2_ACCOUNT_ID"),
		AccessKeyID:     cloudflare.Getenv("R2_ACCESS_KEY_ID"),
		SecretAccessKey: cloudflare.Getenv("R2_SECRET_ACCESS_KEY"),
		Bucket:          cloudflare.Getenv("R2_BUCKET"),
	}
	if cfg.AccountID == "" {
		cfg.AccountID = r2DevAccountID
	}
	if cfg.AccessKeyID == "" {
		cfg.AccessKeyID = r2DevAccessKeyID
	}
	if cfg.SecretAccessKey == "" {
		cfg.SecretAccessKey = r2DevSecretAccessKey
	}
	if cfg.Bucket == "" {
		cfg.Bucket = r2DevBucket
	}
	cachedR2Config = cfg
	cachedR2Resolved = true
	return cfg
}

func main() {
	mux := http.NewServeMux()

	doFetch := func(req *http.Request) (*http.Response, error) {
		ns, err := cloudflare.NewDurableObjectNamespace("CLUSTER")
		if err != nil {
			return nil, err
		}
		id := ns.IdFromName("default")
		stub, err := ns.Get(id)
		if err != nil {
			return nil, err
		}
		return stub.Fetch(req)
	}

	storage := apiserver.NewStorage(doFetch, "/registry")

	// One ResourceStore map per GroupVersion in apidef.Table (replaces what
	// used to be 10 separate hand-written NewXStores calls, one per API
	// group), plus the union of every namespaced store across all of them
	// for namespace cascading delete.
	storesByGV := make(map[schema.GroupVersion]map[string]*apiserver.ResourceStore, len(apidef.GroupVersions()))
	allStoreMaps := make([]map[string]*apiserver.ResourceStore, 0, len(apidef.GroupVersions()))
	for _, gv := range apidef.GroupVersions() {
		stores := apiserver.NewResourceStoresForGroupVersion(storage, gv)
		storesByGV[gv] = stores
		allStoreMaps = append(allStoreMaps, stores)
	}
	namespacedStores := apiserver.NamespacedResourceStores(allStoreMaps...)

	// CA Manager for supervisor protocol
	cam := apiserver.NewCAManager(storage)

	// R2Config (Phase 8): resolved lazily, same reason as getToken -- see
	// r2.go's currentR2Config doc comment for why this is a settable
	// func-var rather than a parameter threaded through HandleResource.
	apiserver.SetR2ConfigFunc(getR2Config)

	// Discovery (no auth)
	apiserver.RegisterDiscovery(mux)
	apiserver.RegisterGroupDiscovery(mux)
	apiserver.RegisterOpenAPIDiscovery(mux)

	// authorization.k8s.io/v1 SelfSubjectAccessReview (`kubectl auth can-i`)
	// -- not in apidef.Table, so not covered by the per-GroupVersion loop
	// below. See selfsubjectaccessreview.go for why.
	apiserver.RegisterAuthorizationHandlers(mux, getToken)

	// Supervisor endpoints (/cacerts, /v1-k3s/*)
	apiserver.RegisterSupervisorHandlers(mux, cam, storage, getToken)

	// Internal endpoints, service-binding-only (/internal/*)
	apiserver.RegisterInternalHandlers(mux, storage)
	// /internal/mint-r2-credentials, called by workers/nodes over its own
	// APISERVER service binding (Phase 8) -- corev1's store map has both
	// persistentvolumeclaims and persistentvolumes, which is all this
	// handler needs.
	apiserver.RegisterR2Handlers(mux, storesByGV[corev1.SchemeGroupVersion])

	// One auth-wrapped route per GroupVersion in apidef.Table. core/v1
	// additionally bootstraps the cluster's baseline namespaces/
	// ServiceAccounts on first request and sweeps dependents on Namespace
	// delete ("namespaces" never exists as a key in a non-core stores map,
	// so that branch of HandleResource's DELETE case is a no-op for every
	// other group even though they all pass the same namespacedStores).
	// namespacedStores is passed to every group, not just core, because
	// HandleResource's DELETE case also uses it for ownerReferences cascade
	// GC (gc.go) -- e.g. deleting an apps/v1 Deployment must be able to find
	// and delete the ReplicaSets (apps/v1) and Pods (core/v1) it owns, which
	// requires the union across every group, not just the deleted object's
	// own. storage.k8s.io/v1 similarly bootstraps the "r2" StorageClass on
	// first request (Phase 8).
	for _, gv := range apidef.GroupVersions() {
		prefix := apidef.APIPrefix(gv)
		stores := storesByGV[gv]
		isCore := gv == corev1.SchemeGroupVersion
		isStorage := gv == storagev1.SchemeGroupVersion

		mux.Handle(prefix, apiserver.AuthMiddleware(getToken, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isCore {
				apiserver.BootstrapCluster(r.Context(), stores)
			}
			if isStorage {
				apiserver.BootstrapStorageClasses(r.Context(), stores)
			}
			apiserver.HandleResource(w, r, prefix, stores, namespacedStores)
		})))
	}

	workers.Serve(mux)
}
