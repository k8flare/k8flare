package registry

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/kubernetes/scheme"
)

// ServedGroupVersion is one group/version and the resources served in it.
type ServedGroupVersion struct {
	GV        schema.GroupVersion
	Resources []metav1.APIResource
}

type strategy struct {
	runtime.ObjectTyper
	names.NameGenerator
	namespaced bool
}

func (s strategy) NamespaceScoped() bool { return s.namespaced }
func (strategy) PrepareForCreate(_ context.Context, obj runtime.Object) {
	if a, err := meta.Accessor(obj); err == nil {
		a.SetGeneration(1)
	}
	if _, ok := obj.(*corev1.Node); ok {
		return
	}
	clearStatus(obj)
}

func clearStatus(obj runtime.Object) {
	val := reflect.ValueOf(obj)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return
	}
	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return
	}
	status := val.FieldByName("Status")
	if status.IsValid() && status.CanSet() {
		status.Set(reflect.Zero(status.Type()))
	}
}
func (strategy) Validate(context.Context, runtime.Object) field.ErrorList  { return nil }
func (strategy) WarningsOnCreate(context.Context, runtime.Object) []string { return nil }
func (strategy) Canonicalize(runtime.Object)                               {}
func (strategy) AllowCreateOnUpdate() bool                                 { return false }
func (strategy) PrepareForUpdate(_ context.Context, obj, old runtime.Object) {
	newA, err1 := meta.Accessor(obj)
	oldA, err2 := meta.Accessor(old)
	if err1 != nil || err2 != nil {
		return
	}
	newA.SetGeneration(oldA.GetGeneration())
	if specChanged(obj, old) {
		newA.SetGeneration(oldA.GetGeneration() + 1)
	}
	keepStatus(obj, old)
}

func keepStatus(obj, old runtime.Object) {
	newVal := reflect.ValueOf(obj)
	oldVal := reflect.ValueOf(old)
	if newVal.Kind() != reflect.Ptr || oldVal.Kind() != reflect.Ptr || newVal.IsNil() || oldVal.IsNil() {
		return
	}
	newVal, oldVal = newVal.Elem(), oldVal.Elem()
	if newVal.Kind() != reflect.Struct || newVal.Type() != oldVal.Type() {
		return
	}
	status := newVal.FieldByName("Status")
	previous := oldVal.FieldByName("Status")
	if status.IsValid() && status.CanSet() && previous.IsValid() {
		status.Set(previous)
	}
}
func (strategy) ValidateUpdate(context.Context, runtime.Object, runtime.Object) field.ErrorList {
	return nil
}
func (strategy) WarningsOnUpdate(context.Context, runtime.Object, runtime.Object) []string {
	return nil
}
func (strategy) AllowUnconditionalUpdate() bool { return true }

func WithNames(store *genericregistry.Store, res metav1.APIResource) rest.Storage {
	var deleter rest.GracefulDeleter
	if build, ok := Deleters[res.Name]; ok {
		deleter = build(store)
	}
	return storeWithNames{store, res.ShortNames, res.Categories, deleter}
}

// KubeletProxy is where pods/log and friends reach the kubelet: through the
// TUNNEL binding's NodeTunnel Durable Object, which holds the node's
// remotedialer session, rather than dialing the node's address directly.
type KubeletProxy struct {
	Transport http.RoundTripper
	Base      string
}

type storeWithNames struct {
	*genericregistry.Store
	shortNames []string
	categories []string
	deleter    rest.GracefulDeleter
}

func (s storeWithNames) ShortNames() []string { return s.shortNames }
func (s storeWithNames) Categories() []string { return s.categories }

func (s storeWithNames) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	if s.deleter != nil {
		return s.deleter.Delete(ctx, name, deleteValidation, options)
	}
	return s.Store.Delete(ctx, name, deleteValidation, options)
}

type Deps struct {
	Kine      *kine.Client
	Tokens    authenticator.Token
	Kubelet   KubeletProxy
	Admission *http.Client
	TokenHMAC []byte
}

type Store = genericregistry.Store

var (
	Resources    = map[string]func(gv schema.GroupVersion, res metav1.APIResource, deps Deps) rest.Storage{}
	Customizers  = map[string]func(store *Store, deps Deps){}
	Deleters     = map[string]func(store *Store) rest.GracefulDeleter{}
	Wrappers     = map[string]func(storage rest.Storage, deps Deps) rest.Storage{}
	Subresources = map[string]func(stores map[string]*Store, deps Deps) rest.Storage{}
	Middleware   []func(stores map[string]*Store) func(http.Handler) http.Handler
)

func NewStore(client *kine.Client, gv schema.GroupVersion, res metav1.APIResource) (*genericregistry.Store, error) {
	gvk := gv.WithKind(res.Kind)
	newFunc := func() runtime.Object {
		obj, _ := scheme.Scheme.New(gvk)
		return obj
	}
	newListFunc := func() runtime.Object {
		obj, _ := scheme.Scheme.New(gv.WithKind(res.Kind + "List"))
		return obj
	}
	if newFunc() == nil || newListFunc() == nil {
		return nil, fmt.Errorf("%s %s is not registered in the scheme", gv, res.Kind)
	}
	strat := strategy{ObjectTyper: scheme.Scheme, NameGenerator: names.SimpleNameGenerator, namespaced: res.Namespaced}
	prefix := "/" + res.Name
	gr := gv.WithResource(res.Name).GroupResource()
	codec := scheme.Codecs.LegacyCodec(gv)
	persist := storageCodec(gv)
	kineStorage := kine.NewStorage(client, persist, newFunc)
	store := &genericregistry.Store{
		NewFunc:                   newFunc,
		NewListFunc:               newListFunc,
		DefaultQualifiedResource:  gr,
		SingularQualifiedResource: gv.WithResource(res.SingularName).GroupResource(),
		CreateStrategy:            strat,
		UpdateStrategy:            strat,
		DeleteStrategy:            strat,
		ReturnDeletedObject:       true,
		EnableGarbageCollection:   true,
		TableConvertor:            newTableConvertor(gv, gr, codec),
		ObjectNameFunc: func(obj runtime.Object) (string, error) {
			a, err := meta.Accessor(obj)
			if err != nil {
				return "", err
			}
			return a.GetName(), nil
		},
		KeyRootFunc: func(ctx context.Context) string {
			if res.Namespaced {
				if ns, ok := genericapirequest.NamespaceFrom(ctx); ok && ns != "" {
					return prefix + "/" + ns
				}
			}
			return prefix
		},
		KeyFunc: func(ctx context.Context, name string) (string, error) {
			if res.Namespaced {
				return genericregistry.NamespaceKeyFunc(ctx, prefix, name)
			}
			return genericregistry.NoNamespaceKeyFunc(ctx, prefix, name)
		},
		PredicateFunc: func(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
			return storage.SelectionPredicate{Label: label, Field: field, GetAttrs: attrsFor(field)}
		},
		Storage: genericregistry.DryRunnableStorage{Storage: kineStorage, Codec: persist},
	}
	return store, nil
}

type OrphanByDefault struct {
	rest.RESTDeleteStrategy
}

func (OrphanByDefault) DefaultGarbageCollectionPolicy(context.Context) rest.GarbageCollectionPolicy {
	return rest.OrphanDependents
}

func specChanged(obj, old runtime.Object) bool {
	if newSpec, oldSpec, ok := specFields(obj, old); ok {
		return !apiequality.Semantic.DeepEqual(newSpec, oldSpec)
	}
	newU, err1 := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	oldU, err2 := runtime.DefaultUnstructuredConverter.ToUnstructured(old)
	if err1 != nil || err2 != nil {
		return false
	}
	return !apiequality.Semantic.DeepEqual(newU["spec"], oldU["spec"])
}

func specFields(obj, old runtime.Object) (any, any, bool) {
	newSpec, ok1 := specField(obj)
	oldSpec, ok2 := specField(old)
	return newSpec, oldSpec, ok1 && ok2
}

func specField(obj runtime.Object) (any, bool) {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, false
	}
	f := v.FieldByName("Spec")
	if !f.IsValid() || !f.CanInterface() {
		return nil, false
	}
	return f.Interface(), true
}
