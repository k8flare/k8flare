package apiserver

import (
	"fmt"

	"context"
	corev1 "k8s.io/api/core/v1"

	batchv1 "k8s.io/api/batch/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"
)

// genericStrategy is the one create/update/delete strategy every resource
// migrated onto genericregistry.Store shares: external types straight from
// Scheme, upstream name generation, no per-type Prepare/Validate hooks --
// those live either upstream (BeforeCreate/BeforeUpdate's ObjectMeta
// handling, exactly what the hand-written store.go re-implemented) or in
// this project's ApplyDefaults/admission layer, which runs in the HTTP
// handler before the store is reached.
type genericStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
	resource   string
	namespaced bool
	// gvk is the Kind this store persists, used by stampTypeMeta.
	gvk schema.GroupVersionKind
	// storage is needed by the create-time effects that allocate from a
	// cluster-wide pool -- today only the Service ClusterIP allocator.
	// Upstream's own service registry holds an allocator the same way;
	// this project's pool lives in the Durable Object, so the strategy
	// holds the client rather than an in-memory bitmap.
	storage *Storage
}

// stampTypeMeta fills in an absent apiVersion/kind before the object is
// handed to storage. Real kube-apiserver gets this for free: it converts to
// an internal type and back, and the versioning encoder sets the GVK on
// everything it writes. This project stores external types with
// EncodeToStorage, which never consults the Scheme (see its doc comment),
// so an object whose TypeMeta happens to be empty is persisted without one
// -- true of anything this apiserver constructs in Go (bootstrap
// namespaces, the "kubernetes" Service, the default ServiceAccount, the
// root CA ConfigMap) and equally of any request body that carried no
// apiVersion/kind, which is how kube-controller-manager's event
// broadcaster POSTs Events (see ResourceStore.decodeDefaults).
//
// Watch is served in TypeScript straight from the stored bytes (no
// re-encode), so those objects reached every informer as Kind-less events.
// client-go's reflector cannot decode such an event, warns "Object 'Kind'
// is missing" and falls back to a full relist -- observed from a real
// kube-scheduler at startup for both Namespace and Service.
func (g genericStrategy) stampTypeMeta(obj runtime.Object) {
	if g.gvk.Empty() {
		return
	}
	kind := obj.GetObjectKind()
	if kind.GroupVersionKind().Empty() {
		kind.SetGroupVersionKind(g.gvk)
	}
}

func (g genericStrategy) NamespaceScoped() bool { return g.namespaced }

// PrepareForCreate carries over the two create-time normalizations the
// hand-written store.Create did after assigning a UID. Upstream's
// Store.create fills the system metadata fields (UID included) before
// calling BeforeCreate, which is what invokes this -- so the UID that
// prepareJobForCreate needs is already set here.
func (g genericStrategy) PrepareForCreate(_ context.Context, obj runtime.Object) {
	g.stampTypeMeta(obj)
	if _, ok := specForGeneration(g.resource, obj); ok {
		if m := getObjectMeta(obj); m != nil {
			m.Generation = 1
		}
	}
	if job, ok := obj.(*batchv1.Job); ok {
		prepareJobForCreate(job)
	}
}

func (genericStrategy) Validate(context.Context, runtime.Object) field.ErrorList { return nil }

func (genericStrategy) WarningsOnCreate(context.Context, runtime.Object) []string { return nil }

func (genericStrategy) Canonicalize(runtime.Object) {}

func (genericStrategy) AllowCreateOnUpdate() bool { return false }

// PrepareForUpdate replicates the hand-written store.Update's
// metadata.generation bump. BeforeUpdate has already reset the incoming
// object's generation to the stored one (clients can't set it), so this
// only has to decide whether to move it: bump when the spec actually
// changed, for the resources that declare a status subresource. See
// specForGeneration (store.go) for why that gate, and why the comparison
// is Semantic.DeepEqual.
func (g genericStrategy) PrepareForUpdate(_ context.Context, obj, old runtime.Object) {
	// Also on update, or a PUT whose body carried no apiVersion/kind would
	// put a stamped object back to Kind-less.
	g.stampTypeMeta(obj)
	newSpec, ok := specForGeneration(g.resource, obj)
	if !ok {
		return
	}
	oldSpec, oldOk := specForGeneration(g.resource, old)
	if oldOk && apiequality.Semantic.DeepEqual(oldSpec.Interface(), newSpec.Interface()) {
		return
	}
	if m := getObjectMeta(obj); m != nil {
		m.Generation++
	}
}

func (genericStrategy) ValidateUpdate(context.Context, runtime.Object, runtime.Object) field.ErrorList {
	return nil
}

func (genericStrategy) WarningsOnUpdate(context.Context, runtime.Object, runtime.Object) []string {
	return nil
}

func (genericStrategy) AllowUnconditionalUpdate() bool { return true }

// NewUpstreamStore builds a genericregistry.Store for one resource on top
// of KineStorage. Keys match the hand-written ResourceStore exactly
// ("/<resource>/<ns>/<name>" under the client's "/registry" prefix), so a
// resource can flip between layers with no data migration.
func NewUpstreamStore(
	storageClient *Storage,
	gv schema.GroupVersion,
	resource, singular string,
	namespaced bool,
	newFunc, newListFunc func() runtime.Object,
) *genericregistry.Store {
	strat := genericStrategy{
		ObjectTyper:   Scheme,
		NameGenerator: names.SimpleNameGenerator,
		resource:      resource,
		namespaced:    namespaced,
		gvk:           routeGVK(gv, newFunc),
		storage:       storageClient,
	}
	prefix := "/" + resource
	gr := gv.WithResource(resource).GroupResource()
	// Upstream's DefaultNamespaceScopedAttr only exposes metadata.name and
	// metadata.namespace to field selectors; every real upstream store
	// supplies its own attr func for the rest. This project's equivalent is
	// selectableFieldsFor (store.go) -- without it a
	// "spec.nodeName=<node>" list (kube-scheduler, kubelet) silently
	// matches nothing, which is what happened when the migration first
	// ran: create a Pod on n1, list with that selector, get zero items.
	attrFunc := func(obj runtime.Object) (labels.Set, fields.Set, error) {
		var lbls labels.Set
		if m := getObjectMeta(obj); m != nil {
			lbls = labels.Set(m.Labels)
		}
		return lbls, selectableFieldsFor(obj), nil
	}
	return &genericregistry.Store{
		NewFunc:                   newFunc,
		NewListFunc:               newListFunc,
		DefaultQualifiedResource:  gr,
		SingularQualifiedResource: gv.WithResource(singular).GroupResource(),
		// Upstream's default DELETE response is a metav1.Status; the
		// hand-written store returned the deleted object, and this
		// project's callers depend on that (DeleteCollection assembles a
		// typed list from it, settleDeletedObject type-switches on
		// *corev1.Service to release the ClusterIP). Keeping the object makes the migration a no-op at the
		// HTTP boundary; upstream sets this flag on its own stores
		// wherever the deleted object matters.
		ReturnDeletedObject: true,
		// Lets Store.Delete run upstream's own graceful-deletion
		// bookkeeping for Orphan/Foreground: stamp deletionTimestamp and
		// the policy's finalizer, and keep the object. Without it,
		// markForDeletion has to write the timestamp itself through
		// Update, which upstream rejects
		// ("metadata.deletionTimestamp: field is immutable" -- only the
		// registry may set it).
		EnableGarbageCollection: true,
		// ClusterIP allocation and release, at the two extension points
		// upstream provides for exactly this. They used to live in
		// handler.go's POST and DELETE cases, which meant a create that
		// did not go through that handler got a Service with no
		// ClusterIP. Moving them here makes the effect a property of the
		// store, so k8s.io/apiserver's own installer (installer.go)
		// serves Services correctly without handler.go in the path.
		BeginCreate: func(ctx context.Context, obj runtime.Object, options *metav1.CreateOptions) (genericregistry.FinishFunc, error) {
			svc, ok := obj.(*corev1.Service)
			if !ok {
				return func(context.Context, bool) {}, nil
			}
			// Not under dry-run: the allocation is a real persisted write
			// and nothing releases it afterwards, so a dry-run Service
			// create would leak an address per call. The reply then
			// carries no ClusterIP, which is what upstream's dry-run does
			// too.
			if len(options.DryRun) > 0 {
				return func(context.Context, bool) {}, nil
			}
			if err := AssignClusterIP(ctx, storageClient, svc); err != nil {
				return nil, fmt.Errorf("allocate ClusterIP: %w", err)
			}
			return func(context.Context, bool) {}, nil
		},
		AfterDelete: func(obj runtime.Object, _ *metav1.DeleteOptions) {
			if svc, ok := obj.(*corev1.Service); ok {
				ReleaseClusterIP(context.Background(), storageClient, svc)
			}
		},
		CreateStrategy: strat,
		UpdateStrategy: strat,
		DeleteStrategy: strat,
		// Normally filled in by CompleteWithOptions, which this project
		// bypasses (it requires RESTOptions -> the etcd storagebackend
		// factory). Everything it would default must be set explicitly;
		// a nil ObjectNameFunc panics on the first create.
		ObjectNameFunc: func(obj runtime.Object) (string, error) {
			accessor, err := meta.Accessor(obj)
			if err != nil {
				return "", err
			}
			return accessor.GetName(), nil
		},
		KeyRootFunc: func(ctx context.Context) string {
			if namespaced {
				if ns, ok := genericapirequest.NamespaceFrom(ctx); ok && ns != "" {
					return prefix + "/" + ns
				}
			}
			return prefix
		},
		KeyFunc: func(ctx context.Context, name string) (string, error) {
			if namespaced {
				return genericregistry.NamespaceKeyFunc(ctx, prefix, name)
			}
			return genericregistry.NoNamespaceKeyFunc(ctx, prefix, name)
		},
		PredicateFunc: func(label labels.Selector, f fields.Selector) storage.SelectionPredicate {
			return storage.SelectionPredicate{Label: label, Field: f, GetAttrs: attrFunc}
		},
		Storage: genericregistry.DryRunnableStorage{
			Storage: NewKineStorage(storageClient, newFunc),
			Codec:   Codecs.LegacyCodec(gv),
		},
	}
}

// --- ResourceStore delegation -------------------------------------------
//
// While the migration is per-resource, ResourceStore keeps its signature
// and the HTTP handler stays unchanged; a store with `upstream` set routes
// the five single-object verbs through genericregistry.Store (upstream
// ObjectMeta lifecycle: UID/creationTimestamp/resourceVersion/generation,
// preconditions, finalizer-aware delete). Collection operations
// (DeleteCollection / DeleteAllInNamespace) stay on the raw-bytes path --
// byte-compatible storage makes that safe.

func (rs *ResourceStore) upstreamCtx(ctx context.Context, namespace string) context.Context {
	// Derives from the REQUEST context so cancellation propagates into
	// storage round-trips. Always stamp the namespace, empty included:
	// rest.BeforeCreate / BeforeUpdate treat a context with no namespace
	// VALUE AT ALL as an internal error, so a cluster-scoped resource
	// (which legitimately has "") must still carry the key. Leaving it
	// off made every PriorityClass create fail with a 500 (found by the
	// suite).
	return genericapirequest.WithNamespace(ctx, namespace)
}

func (rs *ResourceStore) upstreamGet(ctx context.Context, namespace, name string) (runtime.Object, error) {
	return rs.upstream.Get(rs.upstreamCtx(ctx, namespace), name, &metav1.GetOptions{})
}

func (rs *ResourceStore) upstreamList(ctx context.Context, namespace, fieldSelector, labelSelector string) (runtime.Object, error) {
	opts := &metainternalversion.ListOptions{}
	if labelSelector != "" {
		sel, err := labels.Parse(labelSelector)
		if err != nil {
			return nil, &StatusError{Status: badRequestStatus("invalid labelSelector: " + err.Error())}
		}
		opts.LabelSelector = sel
	}
	if fieldSelector != "" {
		sel, err := fields.ParseSelector(fieldSelector)
		if err != nil {
			return nil, &StatusError{Status: badRequestStatus("invalid fieldSelector: " + err.Error())}
		}
		// Same rejection the hand-written path applied (applyFieldSelector):
		// a selector naming a field nothing populates is a client error,
		// not an always-empty match.
		for _, req := range sel.Requirements() {
			if !knownSelectableFields[req.Field] {
				return nil, &StatusError{Status: badRequestStatus("field label not supported: " + req.Field)}
			}
		}
		opts.FieldSelector = sel
	}
	return rs.upstream.List(rs.upstreamCtx(ctx, namespace), opts)
}

// upstreamCreate and upstreamUpdate pass the CALLER'S options through,
// same as upstreamDelete below and for the same reason. Unlike
// Store.Delete, Store.Create/Update dereference options unconditionally,
// so a nil (no options) caller gets an empty struct rather than a panic.
func (rs *ResourceStore) upstreamCreate(ctx context.Context, namespace string, obj runtime.Object, opts *metav1.CreateOptions) (runtime.Object, error) {
	if opts == nil {
		opts = &metav1.CreateOptions{}
	}
	return rs.upstream.Create(rs.upstreamCtx(ctx, namespace), obj, rest.ValidateAllObjectFunc, opts)
}

func (rs *ResourceStore) upstreamUpdate(ctx context.Context, namespace, name string, obj runtime.Object, opts *metav1.UpdateOptions) (runtime.Object, error) {
	if opts == nil {
		opts = &metav1.UpdateOptions{}
	}
	out, _, err := rs.upstream.Update(rs.upstreamCtx(ctx, namespace), name,
		rest.DefaultUpdatedObjectInfo(obj),
		rest.ValidateAllObjectFunc,
		rest.ValidateAllObjectUpdateFunc,
		false, opts)
	return out, err
}

// upstreamDelete is the single DELETE path for a migrated resource: both
// the outright removal and the graceful-deletion half (propagationPolicy
// Orphan/Foreground, where upstream stamps deletionTimestamp + the
// policy's finalizer and returns the still-visible terminating object)
// are the same Store.Delete call, told apart only by what opts carries.
//
// opts is the CALLER'S options, unaltered. A hand-built substitute used
// to stand in here and it silently dropped the caller's Preconditions --
// a UID-guarded DELETE, which is how a client avoids deleting a recreated
// object of the same name and what upstream's own garbage collector
// sends, deleted the wrong object instead of conflicting (TODO.md P0-3).
// Grace period and dryRun rode the same path.
func (rs *ResourceStore) upstreamDelete(ctx context.Context, namespace, name string, opts *metav1.DeleteOptions) (runtime.Object, error) {
	out, _, err := rs.upstream.Delete(rs.upstreamCtx(ctx, namespace), name, rest.ValidateAllObjectFunc, opts)
	return out, err
}
