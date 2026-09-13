package registry

import (
	"context"
	"fmt"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"net"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
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
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubernetes/pkg/controller/nodeipam/ipam/cidrset"
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

// podStrategy is upstream's graceful deletion rule, which lives on the
// internal Pod type upstream: a scheduled, still running Pod is only marked
// for deletion, and the kubelet removes it with gracePeriodSeconds=0 once
// its containers are gone.
type podStrategy struct{ strategy }

func (podStrategy) CheckGracefulDelete(_ context.Context, obj runtime.Object, options *metav1.DeleteOptions) bool {
	if options == nil {
		return false
	}
	pod := obj.(*corev1.Pod)
	period := int64(0)
	if options.GracePeriodSeconds != nil {
		period = *options.GracePeriodSeconds
	} else if pod.Spec.TerminationGracePeriodSeconds != nil {
		period = *pod.Spec.TerminationGracePeriodSeconds
	}
	if pod.Spec.NodeName == "" {
		period = 0
	}
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		period = 0
	}
	if period < 0 {
		period = 1
	}
	options.GracePeriodSeconds = &period
	return true
}

// WithNames lets the installer's discovery see upstream's short names and
// categories for a resource.
func WithNames(store *genericregistry.Store, res metav1.APIResource) rest.Storage {
	return storeWithNames{store, res.ShortNames, res.Categories}
}

type storeWithNames struct {
	*genericregistry.Store
	shortNames []string
	categories []string
}

func (s storeWithNames) ShortNames() []string { return s.shortNames }
func (s storeWithNames) Categories() []string { return s.categories }

// NewStore builds the generic store for one served resource. clusterCIDR is
// where new Nodes get their PodCIDR from.
func NewStore(client *kine.Client, gv schema.GroupVersion, res metav1.APIResource, clusterCIDR *net.IPNet) (*genericregistry.Store, error) {
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
	var deleteStrategy rest.RESTDeleteStrategy = strat
	if res.Name == "pods" {
		deleteStrategy = podStrategy{strat}
	}
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
		DeleteStrategy:            deleteStrategy,
		ReturnDeletedObject:       true,
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
	if res.Name == "nodes" {
		store.BeginCreate = func(ctx context.Context, obj runtime.Object, _ *metav1.CreateOptions) (genericregistry.FinishFunc, error) {
			if err := assignPodCIDR(ctx, kineStorage, obj.(*corev1.Node), clusterCIDR); err != nil {
				return nil, err
			}
			return func(context.Context, bool) {}, nil
		}
	}
	return store, nil
}

// assignPodCIDR stands in for the nodeipam controller until controllers run
// here: a new Node gets the lowest free /24 of the cluster CIDR. It assumes
// one writer.
func assignPodCIDR(ctx context.Context, s *kine.Storage, node *corev1.Node, clusterCIDR *net.IPNet) error {
	if node.Spec.PodCIDR != "" {
		return nil
	}
	list := &corev1.NodeList{}
	if err := s.GetList(ctx, "/nodes", storage.ListOptions{Recursive: true, Predicate: storage.Everything}, list); err != nil {
		return err
	}
	set, err := cidrset.NewCIDRSet(clusterCIDR, 24)
	if err != nil {
		return err
	}
	for _, n := range list.Items {
		if _, used, err := net.ParseCIDR(n.Spec.PodCIDR); err == nil {
			if err := set.Occupy(used); err != nil {
				return err
			}
		}
	}
	cidr, err := set.AllocateNext()
	if err != nil {
		return err
	}
	node.Spec.PodCIDR = cidr.String()
	node.Spec.PodCIDRs = []string{cidr.String()}
	return nil
}
