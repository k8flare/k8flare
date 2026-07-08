package apiserver

import (
	"net/http"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// ServerConfig is everything a *-wasm entrypoint (this package's own
// cmd/apiserver-wasm)
// must supply to NewServer -- exactly the pieces that can only be
// resolved once a Workers request has actually arrived (env vars, DO
// bindings reached through pkg/cfruntime/cloudflare, which this package
// cannot import -- see currentR2Config's doc comment in r2.go).
type ServerConfig struct {
	// StorageDo routes a kine request to this cluster's Cluster DO.
	StorageDo func(*http.Request) (*http.Response, error)
	// Tokens returns every currently-valid cluster token.
	Tokens TokensFunc
	// ClusterBasePath is this cluster's public URL path prefix ("" for
	// the default cluster, "/c/<id>" for provisioned ones).
	ClusterBasePath func() string
	// R2Config resolves this cluster's R2Config, see r2.go.
	R2Config func() R2Config
	// ClusterStoragePrefix is this cluster's R2 object-key prefix, see
	// r2.go's currentClusterStoragePrefix.
	ClusterStoragePrefix func() string
}

// NewServer builds the full apiserver mux: discovery, cluster DNS, RBAC,
// (self)subject access review / token review, the supervisor protocol,
// R2 credential minting, and one auth+authz-wrapped route per
// GroupVersion in apidef.Table. This is every piece of wiring a *-wasm
// entrypoint's main() used to do by hand; entrypoints now only need to
// supply the small ServerConfig above and call workers.Serve on the
// result.
func NewServer(cfg ServerConfig) *http.ServeMux {
	mux := http.NewServeMux()

	SetR2ConfigFunc(cfg.R2Config)
	SetClusterStoragePrefixFunc(cfg.ClusterStoragePrefix)

	storage := NewStorage(cfg.StorageDo, "/registry")

	// One ResourceStore map per GroupVersion in apidef.Table (replaces
	// what used to be 10 separate hand-written NewXStores calls, one per
	// API group), plus the union of every namespaced store across all of
	// them for namespace cascading delete.
	storesByGV := make(map[schema.GroupVersion]map[string]*ResourceStore, len(apidef.GroupVersions()))
	allStoreMaps := make([]map[string]*ResourceStore, 0, len(apidef.GroupVersions()))
	for _, gv := range apidef.GroupVersions() {
		stores := NewResourceStoresForGroupVersion(storage, gv)
		storesByGV[gv] = stores
		allStoreMaps = append(allStoreMaps, stores)
	}
	namespacedStores := NamespacedResourceStores(allStoreMaps...)

	// CA Manager for supervisor protocol
	cam := NewCAManager(storage)

	// ServiceAccount tokens: the real upstream JWT authenticator (lazy --
	// only reads the signing key from storage the first time a non-
	// cluster-token bearer is actually presented, see
	// serviceaccounttoken.go's lazyServiceAccountAuthenticator).
	SetServiceAccountAuthenticator(
		NewServiceAccountTokenAuthenticator(cam, storesByGV[corev1.SchemeGroupVersion]),
	)
	RegisterServiceAccountTokenHandler(mux, cam, storesByGV[corev1.SchemeGroupVersion], cfg.Tokens)

	// Discovery (no auth)
	RegisterDiscovery(mux)
	RegisterGroupDiscovery(mux)
	RegisterOpenAPIDiscovery(mux)

	// Cluster DNS (DoH synthesis half; the other half is cmd/agent's
	// node-local shim, see dns.go). No Containers/CoreDNS Deployment.
	RegisterDNSHandlers(
		mux,
		storesByGV[corev1.SchemeGroupVersion],
		storesByGV[discoveryv1.SchemeGroupVersion],
		cfg.Tokens,
		"cluster.local",
	)

	// RBAC: the real upstream RBACAuthorizer over this apiserver's own
	// rbac/v1 stores + the real bootstrap policy (rbac.go). The cluster
	// token's system:masters identity bypasses it (zero storage reads on
	// today's hot paths); derived identities (X-Remote-User, future
	// ServiceAccount tokens) get real decisions.
	authz := NewRBACAuthorizer(storesByGV[rbacv1.SchemeGroupVersion])

	// authorization.k8s.io/v1 SelfSubjectAccessReview (`kubectl auth
	// can-i`) + SubjectAccessReview, and authentication.k8s.io/v1
	// TokenReview (the kubelet's webhook authenticator/authorizer, used
	// by the per-Pod node logs/metrics bridge) -- not in apidef.Table, so
	// not covered by the per-GroupVersion loop below. Both answer from
	// the same authorizer that gates live traffic. See
	// selfsubjectaccessreview.go / tokenreview.go for why they live
	// outside the table.
	RegisterAuthorizationHandlers(mux, cfg.Tokens, authz)
	RegisterAuthenticationHandlers(mux, cfg.Tokens)

	// Supervisor endpoints (/cacerts, /v1-k3s/*)
	RegisterSupervisorHandlers(mux, cam, storage, cfg.Tokens, cfg.ClusterBasePath)

	// Internal endpoints, service-binding-only (/internal/*)
	RegisterInternalHandlers(mux, storage)
	// /internal/mint-r2-credentials, called by workers/nodes over its own
	// APISERVER service binding (Phase 8) -- corev1's store map has both
	// persistentvolumeclaims and persistentvolumes, which is all this
	// handler needs.
	RegisterR2Handlers(mux, storesByGV[corev1.SchemeGroupVersion])

	// One auth-wrapped route per GroupVersion in apidef.Table. core/v1
	// additionally bootstraps the cluster's baseline namespaces/
	// ServiceAccounts on first request and sweeps dependents on Namespace
	// delete ("namespaces" never exists as a key in a non-core stores
	// map, so that branch of HandleResource's DELETE case is a no-op for
	// every other group even though they all pass the same
	// namespacedStores). namespacedStores is passed to every group, not
	// just core, because HandleResource's DELETE case also uses it for
	// ownerReferences cascade GC (gc.go) -- e.g. deleting an apps/v1
	// Deployment must be able to find and delete the ReplicaSets
	// (apps/v1) and Pods (core/v1) it owns, which requires the union
	// across every group, not just the deleted object's own.
	// storage.k8s.io/v1 similarly bootstraps the "r2" StorageClass on
	// first request (Phase 8). Pod is core/v1-only, but priority
	// admission (priority.go) needs the scheduling.k8s.io/v1
	// priorityclasses store to resolve spec.priorityClassName --
	// threaded into every group's HandleResource call the same way
	// namespacedStores is (see HandleResource's doc comment).
	priorityClassStore := storesByGV[schedulingv1.SchemeGroupVersion]["priorityclasses"]

	for _, gv := range apidef.GroupVersions() {
		prefix := apidef.APIPrefix(gv)
		stores := storesByGV[gv]
		isCore := gv == corev1.SchemeGroupVersion
		isStorage := gv == storagev1.SchemeGroupVersion

		mux.Handle(prefix, AuthMiddleware(cfg.Tokens, AuthzMiddleware(authz, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isCore {
				BootstrapCluster(r.Context(), stores)
			}
			if isStorage {
				BootstrapStorageClasses(r.Context(), stores)
			}
			HandleResource(w, r, prefix, stores, namespacedStores, priorityClassStore)
		}))))
	}

	return mux
}
