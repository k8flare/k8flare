package registry

import (
	"context"
	"fmt"
	"net/http"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"

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

func (s strategy) NamespaceScoped() bool                                          { return s.namespaced }
func (strategy) PrepareForCreate(context.Context, runtime.Object)                 {}
func (strategy) Validate(context.Context, runtime.Object) field.ErrorList         { return nil }
func (strategy) WarningsOnCreate(context.Context, runtime.Object) []string        { return nil }
func (strategy) Canonicalize(runtime.Object)                                      {}
func (strategy) AllowCreateOnUpdate() bool                                        { return false }
func (strategy) PrepareForUpdate(context.Context, runtime.Object, runtime.Object) {}
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
	Kine    *kine.Client
	Tokens  authenticator.Token
	Kubelet KubeletProxy
}

type Store = genericregistry.Store

var (
	Resources          = map[string]func(gv schema.GroupVersion, res metav1.APIResource, deps Deps) rest.Storage{}
	Customizers        = map[string]func(store *Store, deps Deps){}
	Deleters           = map[string]func(store *Store) rest.GracefulDeleter{}
	Subresources       = map[string]func(stores map[string]*Store, deps Deps) rest.Storage{}
	Middleware         []func(stores map[string]*Store) func(http.Handler) http.Handler
	EnqueueControllers func(ctx context.Context, delay time.Duration)
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
	kineStorage := kine.NewStorage(client, codec, newFunc)
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
		Storage: genericregistry.DryRunnableStorage{Storage: kineStorage, Codec: codec},
	}
	return store, nil
}

type OrphanByDefault struct {
	rest.RESTDeleteStrategy
}

func (OrphanByDefault) DefaultGarbageCollectionPolicy(context.Context) rest.GarbageCollectionPolicy {
	return rest.OrphanDependents
}
