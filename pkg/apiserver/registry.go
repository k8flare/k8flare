package apiserver

import (
	"context"
	"fmt"
	"net"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	nodev1 "k8s.io/api/node/v1"
	storagev1 "k8s.io/api/storage/v1"
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
)

// Kind is one served resource. The table below is the whole API surface.
type Kind struct {
	GV          schema.GroupVersion
	Resource    string
	Singular    string
	Kind        string
	Namespaced  bool
	Status      bool
	ShortNames  []string
	NewFunc     func() runtime.Object
	NewListFunc func() runtime.Object
}

var Kinds = []Kind{
	{corev1.SchemeGroupVersion, "namespaces", "namespace", "Namespace", false, true, []string{"ns"}, func() runtime.Object { return &corev1.Namespace{} }, func() runtime.Object { return &corev1.NamespaceList{} }},
	{corev1.SchemeGroupVersion, "nodes", "node", "Node", false, true, []string{"no"}, func() runtime.Object { return &corev1.Node{} }, func() runtime.Object { return &corev1.NodeList{} }},
	{corev1.SchemeGroupVersion, "pods", "pod", "Pod", true, true, []string{"po"}, func() runtime.Object { return &corev1.Pod{} }, func() runtime.Object { return &corev1.PodList{} }},
	{corev1.SchemeGroupVersion, "services", "service", "Service", true, true, []string{"svc"}, func() runtime.Object { return &corev1.Service{} }, func() runtime.Object { return &corev1.ServiceList{} }},
	{corev1.SchemeGroupVersion, "configmaps", "configmap", "ConfigMap", true, false, []string{"cm"}, func() runtime.Object { return &corev1.ConfigMap{} }, func() runtime.Object { return &corev1.ConfigMapList{} }},
	{corev1.SchemeGroupVersion, "secrets", "secret", "Secret", true, false, nil, func() runtime.Object { return &corev1.Secret{} }, func() runtime.Object { return &corev1.SecretList{} }},
	{corev1.SchemeGroupVersion, "serviceaccounts", "serviceaccount", "ServiceAccount", true, false, []string{"sa"}, func() runtime.Object { return &corev1.ServiceAccount{} }, func() runtime.Object { return &corev1.ServiceAccountList{} }},
	{corev1.SchemeGroupVersion, "events", "event", "Event", true, false, []string{"ev"}, func() runtime.Object { return &corev1.Event{} }, func() runtime.Object { return &corev1.EventList{} }},
	{coordinationv1.SchemeGroupVersion, "leases", "lease", "Lease", true, false, nil, func() runtime.Object { return &coordinationv1.Lease{} }, func() runtime.Object { return &coordinationv1.LeaseList{} }},
	{discoveryv1.SchemeGroupVersion, "endpointslices", "endpointslice", "EndpointSlice", true, false, nil, func() runtime.Object { return &discoveryv1.EndpointSlice{} }, func() runtime.Object { return &discoveryv1.EndpointSliceList{} }},
	{nodev1.SchemeGroupVersion, "runtimeclasses", "runtimeclass", "RuntimeClass", false, false, nil, func() runtime.Object { return &nodev1.RuntimeClass{} }, func() runtime.Object { return &nodev1.RuntimeClassList{} }},
	{storagev1.SchemeGroupVersion, "csidrivers", "csidriver", "CSIDriver", false, false, nil, func() runtime.Object { return &storagev1.CSIDriver{} }, func() runtime.Object { return &storagev1.CSIDriverList{} }},
}

type strategy struct {
	runtime.ObjectTyper
	names.NameGenerator
	kind Kind
}

func (s strategy) NamespaceScoped() bool { return s.kind.Namespaced }

func (s strategy) PrepareForCreate(_ context.Context, obj runtime.Object) {
	obj.GetObjectKind().SetGroupVersionKind(s.kind.GV.WithKind(s.kind.Kind))
}

func (strategy) Validate(context.Context, runtime.Object) field.ErrorList  { return nil }
func (strategy) WarningsOnCreate(context.Context, runtime.Object) []string { return nil }
func (strategy) Canonicalize(runtime.Object)                               {}
func (strategy) AllowCreateOnUpdate() bool                                 { return false }

func (s strategy) PrepareForUpdate(_ context.Context, obj, _ runtime.Object) {
	obj.GetObjectKind().SetGroupVersionKind(s.kind.GV.WithKind(s.kind.Kind))
}

func (strategy) ValidateUpdate(context.Context, runtime.Object, runtime.Object) field.ErrorList {
	return nil
}
func (strategy) WarningsOnUpdate(context.Context, runtime.Object, runtime.Object) []string {
	return nil
}
func (strategy) AllowUnconditionalUpdate() bool { return true }

// podStrategy adds upstream's graceful deletion rule: a scheduled, still
// running Pod is only marked for deletion, and the kubelet removes it with
// gracePeriodSeconds=0 once its containers are gone.
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

func selectableFields(obj runtime.Object) fields.Set {
	m, err := meta.Accessor(obj)
	if err != nil {
		return nil
	}
	f := fields.Set{"metadata.name": m.GetName(), "metadata.namespace": m.GetNamespace()}
	switch o := obj.(type) {
	case *corev1.Pod:
		f["spec.nodeName"] = o.Spec.NodeName
		f["status.phase"] = string(o.Status.Phase)
	case *corev1.Service:
		f["spec.clusterIP"] = o.Spec.ClusterIP
	case *corev1.Event:
		f["involvedObject.name"] = o.InvolvedObject.Name
		f["involvedObject.namespace"] = o.InvolvedObject.Namespace
		f["involvedObject.kind"] = o.InvolvedObject.Kind
		f["involvedObject.uid"] = string(o.InvolvedObject.UID)
		f["reason"] = o.Reason
		f["type"] = o.Type
	}
	return f
}

func newStore(kine *KineClient, k Kind) *genericregistry.Store {
	var strat rest.RESTDeleteStrategy = strategy{ObjectTyper: scheme.Scheme, NameGenerator: names.SimpleNameGenerator, kind: k}
	if k.Resource == "pods" {
		strat = podStrategy{strat.(strategy)}
	}
	prefix := "/" + k.Resource
	gr := k.GV.WithResource(k.Resource).GroupResource()
	codec := scheme.Codecs.LegacyCodec(k.GV)
	kineStorage := NewKineStorage(kine, codec, k.NewFunc)
	store := &genericregistry.Store{
		NewFunc:                   k.NewFunc,
		NewListFunc:               k.NewListFunc,
		DefaultQualifiedResource:  gr,
		SingularQualifiedResource: k.GV.WithResource(k.Singular).GroupResource(),
		CreateStrategy:            strat.(rest.RESTCreateStrategy),
		UpdateStrategy:            strat.(rest.RESTUpdateStrategy),
		DeleteStrategy:            strat,
		ReturnDeletedObject:       true,
		TableConvertor:            rest.NewDefaultTableConvertor(gr),
		ObjectNameFunc: func(obj runtime.Object) (string, error) {
			a, err := meta.Accessor(obj)
			if err != nil {
				return "", err
			}
			return a.GetName(), nil
		},
		KeyRootFunc: func(ctx context.Context) string {
			if k.Namespaced {
				if ns, ok := genericapirequest.NamespaceFrom(ctx); ok && ns != "" {
					return prefix + "/" + ns
				}
			}
			return prefix
		},
		KeyFunc: func(ctx context.Context, name string) (string, error) {
			if k.Namespaced {
				return genericregistry.NamespaceKeyFunc(ctx, prefix, name)
			}
			return genericregistry.NoNamespaceKeyFunc(ctx, prefix, name)
		},
		PredicateFunc: func(label labels.Selector, f fields.Selector) storage.SelectionPredicate {
			return storage.SelectionPredicate{Label: label, Field: f, GetAttrs: func(obj runtime.Object) (labels.Set, fields.Set, error) {
				a, err := meta.Accessor(obj)
				if err != nil {
					return nil, nil, err
				}
				return a.GetLabels(), selectableFields(obj), nil
			}}
		},
		Storage: genericregistry.DryRunnableStorage{Storage: kineStorage, Codec: codec},
	}
	if k.Resource == "nodes" {
		store.BeginCreate = func(ctx context.Context, obj runtime.Object, _ *metav1.CreateOptions) (genericregistry.FinishFunc, error) {
			if err := assignPodCIDR(ctx, kineStorage, obj.(*corev1.Node)); err != nil {
				return nil, err
			}
			return func(context.Context, bool) {}, nil
		}
	}
	return store
}

// assignPodCIDR gives a new Node the lowest free 10.42.N.0/24. The real
// nodeipam controller is not running here; a single writer is assumed.
func assignPodCIDR(ctx context.Context, s *KineStorage, node *corev1.Node) error {
	if node.Spec.PodCIDR != "" {
		return nil
	}
	list := &corev1.NodeList{}
	if err := s.GetList(ctx, "/nodes", storage.ListOptions{Recursive: true, Predicate: storage.Everything}, list); err != nil {
		return err
	}
	used := map[string]bool{}
	for _, n := range list.Items {
		used[n.Spec.PodCIDR] = true
	}
	for i := 0; i < 256; i++ {
		cidr := (&net.IPNet{IP: net.IPv4(10, 42, byte(i), 0), Mask: net.CIDRMask(24, 32)}).String()
		if !used[cidr] {
			node.Spec.PodCIDR = cidr
			node.Spec.PodCIDRs = []string{cidr}
			return nil
		}
	}
	return fmt.Errorf("no free pod CIDR")
}
