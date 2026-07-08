//go:build js && wasm

package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/k8flare/k8flare/pkg/apiserver"
	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
	"github.com/k8flare/k8flare/pkg/cfruntime"
	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
	cffetch "github.com/k8flare/k8flare/pkg/cfruntime/cloudflare/fetch"
)

// clusterDOName / clusterBasePath: multi-cluster identity of THIS
// dynamic-worker instance (one Loader isolate per cluster,
// loader/apiserver.ts). Env is only readable during request handling.
func clusterDOName() string {
	if n := cloudflare.Getenv("CLUSTER_DO_NAME"); n != "" {
		return n
	}
	return "default"
}

func clusterBasePath() string {
	return cloudflare.Getenv("CLUSTER_BASE_PATH")
}

// getTokens returns every currently-valid token for this cluster.
//
// Default cluster: STRICTLY the env token (mirrors clusters/tokens.ts --
// rotation there is `wrangler secret put`). Provisioned clusters: the
// token vault at /ca/cluster-tokens inside this cluster's own Cluster
// DO, read per request. No cross-request cache is possible here: this
// binary is instantiated fresh per request (S19's per-request worker.mjs
// contract), so the read costs one extra subrequest to the same DO the
// request is about to talk to anyway -- recorded in docs/cost-model.md.
var cachedTokens []string // per-instance memo (one request's lifetime)

func getTokens() []string {
	if cachedTokens != nil {
		return cachedTokens
	}
	name := clusterDOName()
	if name == "default" {
		t := cloudflare.Getenv("K3S_TOKEN")
		if t == "" {
			t = "k8flare-dev-token" // fallback for dev
		}
		cachedTokens = []string{t}
		return cachedTokens
	}
	tokens, err := readVaultTokens()
	if err != nil {
		// Fail closed: no readable vault means nothing authenticates.
		cachedTokens = []string{}
		return cachedTokens
	}
	cachedTokens = tokens
	return cachedTokens
}

// storageDo routes a kine request to THIS cluster's Cluster DO: the
// STORAGE env Fetcher is the parent script's ClusterLoopback entrypoint,
// which dispatches on the X-K8flare-Cluster header (DO namespaces cannot
// cross the Loader env clone, S2 item 3a). Same GetBinding+cffetch
// pattern pkg/controllers.RestConfig proved in production. Lazily
// initialized: env bindings are only reachable once a request has
// actually arrived.
var storageClient *http.Client

func storageDo(req *http.Request) (*http.Response, error) {
	if storageClient == nil {
		binding := cloudflare.GetBinding("STORAGE")
		storageClient = cffetch.NewClient(cffetch.WithBinding(binding)).
			HTTPClient(cffetch.RedirectModeFollow)
	}
	req.Header.Set("X-K8flare-Cluster", clusterDOName())
	return storageClient.Do(req)
}

// readVaultTokens reads the per-cluster token vault (a single kine value
// at /ca/cluster-tokens, which the storage keyspace routes into the
// ca-vault facet). Shape owned by workers/k8flare/src/clusters/tokens.ts.
func readVaultTokens() ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, "http://do.internal/key/ca/cluster-tokens", nil)
	if err != nil {
		return nil, err
	}
	resp, err := storageDo(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		KV *struct {
			Value string `json:"value"`
		} `json:"kv"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.KV == nil {
		return []string{}, nil
	}
	raw, err := base64.StdEncoding.DecodeString(body.KV.Value)
	if err != nil {
		return nil, err
	}
	var vault struct {
		Tokens []struct {
			Secret string `json:"secret"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &vault); err != nil {
		return nil, err
	}
	secrets := make([]string, 0, len(vault.Tokens))
	for _, t := range vault.Tokens {
		secrets = append(secrets, t.Secret)
	}
	return secrets, nil
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

	storage := apiserver.NewStorage(storageDo, "/registry")

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

	// ServiceAccount tokens: the real upstream JWT authenticator (lazy --
	// only reads the signing key from storage the first time a non-
	// cluster-token bearer is actually presented, see
	// serviceaccounttoken.go's lazyServiceAccountAuthenticator).
	apiserver.SetServiceAccountAuthenticator(
		apiserver.NewServiceAccountTokenAuthenticator(cam, storesByGV[corev1.SchemeGroupVersion]),
	)
	apiserver.RegisterServiceAccountTokenHandler(mux, cam, storesByGV[corev1.SchemeGroupVersion], getTokens)

	// R2Config (Phase 8): resolved lazily, same reason as getToken -- see
	// r2.go's currentR2Config doc comment for why this is a settable
	// func-var rather than a parameter threaded through HandleResource.
	apiserver.SetR2ConfigFunc(getR2Config)
	// Multi-cluster: scope this cluster's R2 object keys under
	// clusters/<doName>/ (default keeps unprefixed keys -- see
	// pkg/apiserver/r2.go's currentClusterStoragePrefix).
	apiserver.SetClusterStoragePrefixFunc(func() string {
		if n := clusterDOName(); n != "default" {
			return "clusters/" + n + "/"
		}
		return ""
	})

	// Discovery (no auth)
	apiserver.RegisterDiscovery(mux)
	apiserver.RegisterGroupDiscovery(mux)
	apiserver.RegisterOpenAPIDiscovery(mux)

	// Cluster DNS (DoH synthesis half; the other half is cmd/agent's
	// node-local shim, see dns.go). No Containers/CoreDNS Deployment.
	apiserver.RegisterDNSHandlers(
		mux,
		storesByGV[corev1.SchemeGroupVersion],
		storesByGV[discoveryv1.SchemeGroupVersion],
		getTokens,
		"cluster.local",
	)

	// RBAC: the real upstream RBACAuthorizer over this apiserver's own
	// rbac/v1 stores + the real bootstrap policy (pkg/apiserver/rbac.go).
	// The cluster token's system:masters identity bypasses it (zero
	// storage reads on today's hot paths); derived identities
	// (X-Remote-User, future ServiceAccount tokens) get real decisions.
	authz := apiserver.NewRBACAuthorizer(storesByGV[rbacv1.SchemeGroupVersion])

	// authorization.k8s.io/v1 SelfSubjectAccessReview (`kubectl auth can-i`)
	// + SubjectAccessReview, and authentication.k8s.io/v1 TokenReview (the
	// kubelet's webhook authenticator/authorizer, used by the per-Pod node
	// logs/metrics bridge) -- not in apidef.Table, so not covered by the
	// per-GroupVersion loop below. Both answer from the same authorizer
	// that gates live traffic. See selfsubjectaccessreview.go /
	// tokenreview.go for why they live outside the table.
	apiserver.RegisterAuthorizationHandlers(mux, getTokens, authz)
	apiserver.RegisterAuthenticationHandlers(mux, getTokens)

	// Supervisor endpoints (/cacerts, /v1-k3s/*)
	apiserver.RegisterSupervisorHandlers(mux, cam, storage, getTokens, clusterBasePath)

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
	// Pod is core/v1-only, but priority admission (priority.go) needs the
	// scheduling.k8s.io/v1 priorityclasses store to resolve
	// spec.priorityClassName -- threaded into every group's HandleResource
	// call the same way namespacedStores is (see HandleResource's doc
	// comment). storesByGV[schedulingv1.SchemeGroupVersion]["priorityclasses"]
	// is always present once apidef.Table lists it.
	priorityClassStore := storesByGV[schedulingv1.SchemeGroupVersion]["priorityclasses"]

	for _, gv := range apidef.GroupVersions() {
		prefix := apidef.APIPrefix(gv)
		stores := storesByGV[gv]
		isCore := gv == corev1.SchemeGroupVersion
		isStorage := gv == storagev1.SchemeGroupVersion

		mux.Handle(prefix, apiserver.AuthMiddleware(getTokens, apiserver.AuthzMiddleware(authz, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isCore {
				apiserver.BootstrapCluster(r.Context(), stores)
			}
			if isStorage {
				apiserver.BootstrapStorageClasses(r.Context(), stores)
			}
			apiserver.HandleResource(w, r, prefix, stores, namespacedStores, priorityClassStore)
		}))))
	}

	workers.Serve(mux)
}
