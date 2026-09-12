package apiserver

import (
	"context"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/warning"
	"k8s.io/klog/v2"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// ServerConfig is everything a *-wasm entrypoint (this package's own
// cmd/apiserver-wasm) must supply to NewServer -- exactly the pieces
// that can only be resolved once a Workers request has actually
// arrived (env vars, DO bindings reached through
// pkg/cfruntime/cloudflare, which this package cannot import).
type ServerConfig struct {
	// StorageDo routes a kine request to this cluster's Cluster DO.
	StorageDo func(*http.Request) (*http.Response, error)
	// Tokens returns every currently-valid cluster token.
	Tokens TokensFunc
	// ClusterBasePath is this cluster's public URL path prefix ("" for
	// the default cluster, "/c/<id>" for provisioned ones).
	ClusterBasePath func() string
}

// NewServer builds the full apiserver mux: discovery, cluster DNS, RBAC,
// (self)subject access review / token review, the supervisor protocol,
// and one auth+authz-wrapped route per GroupVersion in apidef.Table.
// This is every piece of wiring a *-wasm entrypoint's main() used to do
// by hand; entrypoints now only need to supply the small ServerConfig
// above and call workers.ServeNonBlock on the result.
func NewServer(cfg ServerConfig) *http.ServeMux {
	mux := http.NewServeMux()

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

	// Internal endpoints, service-binding-only (/internal/*, internal.go)
	RegisterInternalHandlers(mux, storage, namespacedStores)

	// One auth-wrapped route per GroupVersion in apidef.Table. Namespace
	// dependents are swept on Namespace delete ("namespaces" never exists as a key in a non-core stores
	// map, so that branch of HandleResource's DELETE case is a no-op for
	// every other group even though they all pass the same
	// namespacedStores). namespacedStores is passed to every group, not
	// just core, because HandleResource's DELETE case also uses it for
	// propagationPolicy=Orphan (orphan.go) -- e.g. orphaning an apps/v1
	// Deployment's dependents must be able to find its ReplicaSets
	// (apps/v1) and Pods (core/v1), which requires the union across
	// every group, not just the deleted object's own (Background/
	// Foreground cascade delete is the real pkg/controllers/gc
	// garbagecollector controller's job, asynchronously). Pod is
	// core/v1-only, but priority admission (priority.go) needs the
	// scheduling.k8s.io/v1 priorityclasses store to resolve
	// spec.priorityClassName -- threaded into every group's
	// HandleResource call the same way namespacedStores is (see
	// HandleResource's doc comment).
	priorityClassStore := storesByGV[schedulingv1.SchemeGroupVersion]["priorityclasses"]
	// Threaded into every group's HandleResource for namespace-lifecycle
	// admission on create (see handler.go's POST case): only the core map
	// has "namespaces", but apps/batch/... creates need the check too.
	namespaceStore := storesByGV[corev1.SchemeGroupVersion]["namespaces"]

	// Bootstrap runs on the first request to ANY group, not just core/v1
	// (sync.Once-guarded, so the steady-state cost is a no-op check):
	// namespace-lifecycle admission above means a create to e.g. apps/v1
	// must be able to find "default" in the namespaces store, even when
	// no core/v1 request has arrived yet in this instance's lifetime.
	coreStores := storesByGV[corev1.SchemeGroupVersion]

	// The REST surface comes from k8s.io/apiserver's own installer over the
	// same genericregistry.Store instances (installer.go). Built once here
	// rather than per request: InstallREST walks every resource's storage
	// and builds a route table, which is startup work, not request work.
	admit := &k8flareAdmission{
		namespaces:       namespaceStore,
		priorityClasses:  priorityClassStore,
		limitRanges:      coreStores["limitranges"],
		namespacedStores: namespacedStores,
	}

	// Namespace creation still has to seed the default ServiceAccount and the
	// root CA ConfigMap. Wired here rather than where the store is built,
	// because it needs the sibling stores of its own group, which only exist
	// once every group has been assembled.
	if namespaceStore != nil && namespaceStore.upstream != nil {
		namespaceStore.upstream.AfterCreate = func(obj runtime.Object, options *metav1.CreateOptions) {
			if len(options.DryRun) > 0 {
				return
			}
			ApplyPostCreateEffects(context.Background(), coreStores, obj)
		}
		// Deleting a Namespace takes its contents with it. Upstream leaves
		// that to the namespace controller; this apiserver does it inline,
		// and the hook is where that survives the handler it used to live in.
		namespaceStore.upstream.AfterDelete = func(obj runtime.Object, options *metav1.DeleteOptions) {
			if len(options.DryRun) > 0 {
				return
			}
			ns, ok := obj.(*corev1.Namespace)
			if !ok {
				return
			}
			ctx := context.Background()
			if err := DeleteNamespaceDependents(ctx, namespacedStores, ns.Name); err != nil {
				klog.ErrorS(err, "sweeping namespace dependents", "namespace", ns.Name)
			}
			if err := SweepNamespaceEventsAfterDelete(ctx, namespacedStores, ns.Name); err != nil {
				klog.ErrorS(err, "sweeping namespace events", "namespace", ns.Name)
			}
		}
	}

	// A foreground cascade must not finish while a dependent is still
	// alive. The guard used to sit in the DELETE handler; in the store's own
	// hook it also covers the PUT and PATCH that clear the finalizer, which
	// is how a stale garbage collector actually issues it.
	for _, rs := range namespacedStores {
		if rs == nil || rs.upstream == nil {
			continue
		}
		owner := rs
		owner.upstream.BeginUpdate = func(ctx context.Context, obj, old runtime.Object, _ *metav1.UpdateOptions) (genericregistry.FinishFunc, error) {
			if err := RefuseForegroundFinalizeOn(ctx, owner, namespacedStores, old, obj); err != nil {
				return nil, err
			}
			// Orphaning is decided here and done after: the object still
			// carries the orphan finalizer in `old`, and by the time the
			// delete has completed it does not.
			sweep := SweepOrphansOnFinalize(owner, namespacedStores, old, obj)
			return func(finishCtx context.Context, success bool) {
				if success && sweep != nil {
					sweep(finishCtx)
				}
			}, nil
		}

		// ...and the other half: the owner the guard was holding open has to
		// finish once its last blocking dependent goes, or it waits for the
		// real garbage collector to retry a finalizer patch it has already
		// been refused (docs/platform-verification.md S36).
		//
		// Chained rather than assigned: this store may already carry a hook
		// of its own, and Services do (releasing the ClusterIP).
		existing := owner.upstream.AfterDelete
		owner.upstream.AfterDelete = func(obj runtime.Object, options *metav1.DeleteOptions) {
			if existing != nil {
				existing(obj, options)
			}
			if len(options.DryRun) > 0 {
				return
			}
			m := getObjectMeta(obj)
			if m == nil {
				return
			}
			FinishUnblockedForegroundOwners(context.Background(), namespacedStores, owner, m.Namespace, obj)
		}
	}

	installed, err := NewRESTContainer(storesByGV, admit)
	if err != nil {
		panic(fmt.Sprintf("apiserver: install REST routes: %v", err))
	}
	// Upstream's REST handlers read the parsed request out of the context
	// rather than the URL; a generic apiserver puts it there in its handler
	// chain, and without it every route answers "missing requestInfo".
	//
	// The body is endpoints/filters.WithRequestInfo's, inlined rather than
	// imported: that package's authentication filter reaches
	// apiserver/pkg/util/webhook, which needs the OTLP tracing wrapper this
	// build deliberately does not link (pkg/k8s-js-overlays/component-base,
	// worth 20.3MB), and client-go/tools/events, which the leanwidth
	// clientset does not have. Deviating from rule #3 for a six-line
	// wrapper is cheaper than carrying either back.
	resolver := &request.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
	}
	restHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, err := resolver.NewRequestInfo(r)
		if err != nil {
			writeInternalError(w, fmt.Errorf("request info: %w", err))
			return
		}
		ctx := request.WithRequestInfo(r.Context(), info)
		// Every PATCH calls audit.LogRequestPatch, which dereferences the
		// audit context without checking it; a request that arrives without
		// one panics before it reaches storage.
		ctx = audit.WithAuditContext(ctx)
		// Strict decoding reports unknown fields through the warning
		// recorder. With none installed the warnings are dropped and a
		// fieldValidation=Warn request looks like it validated clean.
		ctx = warning.WithWarningRecorder(ctx, headerWarnings{w})
		installed.ServeHTTP(w, withStrictByDefault(r).WithContext(ctx))
	})

	for _, gv := range apidef.GroupVersions() {
		prefix := apidef.APIPrefix(gv)

		mux.Handle(prefix, AuthMiddleware(cfg.Tokens, AuthzMiddleware(authz, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			BootstrapCluster(r.Context(), coreStores)
			restHandler.ServeHTTP(w, r)
		}))))
	}

	return mux
}

// withStrictByDefault makes a request that says nothing about field
// validation behave as Strict. Upstream defaults to Warn; this apiserver has
// rejected unknown fields by default since fieldvalidation.go, and a silently
// accepted typo in a manifest is the thing that default exists to prevent.
// An explicit ?fieldValidation= is left alone.
func withStrictByDefault(r *http.Request) *http.Request {
	q := r.URL.Query()
	if q.Get("fieldValidation") != "" {
		return r
	}
	q.Set("fieldValidation", metav1.FieldValidationStrict)
	out := r.Clone(r.Context())
	out.URL.RawQuery = q.Encode()
	return out
}

// headerWarnings is the recorder upstream's warning filter installs: warnings
// raised while serving become Warning response headers.
type headerWarnings struct {
	w http.ResponseWriter
}

func (h headerWarnings) AddWarning(agent, text string) {
	if text == "" {
		return
	}
	if agent == "" {
		agent = "-"
	}
	h.w.Header().Add("Warning", fmt.Sprintf("299 %s %q", agent, text))
}
