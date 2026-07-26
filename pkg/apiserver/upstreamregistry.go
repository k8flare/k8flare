package apiserver

import (
	"context"

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
	namespaced bool
}

func (g genericStrategy) NamespaceScoped() bool { return g.namespaced }

func (genericStrategy) PrepareForCreate(context.Context, runtime.Object) {}

func (genericStrategy) Validate(context.Context, runtime.Object) field.ErrorList { return nil }

func (genericStrategy) WarningsOnCreate(context.Context, runtime.Object) []string { return nil }

func (genericStrategy) Canonicalize(runtime.Object) {}

func (genericStrategy) AllowCreateOnUpdate() bool { return false }

func (genericStrategy) PrepareForUpdate(context.Context, runtime.Object, runtime.Object) {}

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
		namespaced:    namespaced,
	}
	prefix := "/" + resource
	gr := gv.WithResource(resource).GroupResource()
	attrFunc := storage.DefaultNamespaceScopedAttr
	if !namespaced {
		attrFunc = storage.DefaultClusterScopedAttr
	}
	return &genericregistry.Store{
		NewFunc:                   newFunc,
		NewListFunc:               newListFunc,
		DefaultQualifiedResource:  gr,
		SingularQualifiedResource: gv.WithResource(singular).GroupResource(),
		CreateStrategy:            strat,
		UpdateStrategy:            strat,
		DeleteStrategy:            strat,
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

func (rs *ResourceStore) upstreamCtx(namespace string) context.Context {
	ctx := genericapirequest.NewContext()
	if namespace != "" {
		ctx = genericapirequest.WithNamespace(ctx, namespace)
	}
	return ctx
}

func (rs *ResourceStore) upstreamGet(namespace, name string) (runtime.Object, error) {
	return rs.upstream.Get(rs.upstreamCtx(namespace), name, &metav1.GetOptions{})
}

func (rs *ResourceStore) upstreamList(namespace, fieldSelector, labelSelector string) (runtime.Object, error) {
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
		opts.FieldSelector = sel
	}
	return rs.upstream.List(rs.upstreamCtx(namespace), opts)
}

func (rs *ResourceStore) upstreamCreate(namespace string, obj runtime.Object) (runtime.Object, error) {
	return rs.upstream.Create(rs.upstreamCtx(namespace), obj, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
}

func (rs *ResourceStore) upstreamUpdate(namespace, name string, obj runtime.Object) (runtime.Object, error) {
	out, _, err := rs.upstream.Update(rs.upstreamCtx(namespace), name,
		rest.DefaultUpdatedObjectInfo(obj),
		rest.ValidateAllObjectFunc,
		rest.ValidateAllObjectUpdateFunc,
		false, &metav1.UpdateOptions{})
	return out, err
}

func (rs *ResourceStore) upstreamDelete(namespace, name string) (runtime.Object, error) {
	out, _, err := rs.upstream.Delete(rs.upstreamCtx(namespace), name, rest.ValidateAllObjectFunc, &metav1.DeleteOptions{})
	return out, err
}
