package main

import (
	"context"
	"fmt"

	"github.com/emicklei/go-restful/v3"
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
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/endpoints"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/kubernetes/scheme"
)

var codecs = serializer.NewCodecFactory(scheme.Scheme)

type strategy struct {
	runtime.ObjectTyper
	names.NameGenerator
	namespaced bool
}

func (s strategy) NamespaceScoped() bool                                { return s.namespaced }
func (strategy) PrepareForCreate(context.Context, runtime.Object)          {}
func (strategy) Validate(context.Context, runtime.Object) field.ErrorList { return nil }
func (strategy) WarningsOnCreate(context.Context, runtime.Object) []string { return nil }
func (strategy) Canonicalize(runtime.Object)                              {}
func (strategy) AllowCreateOnUpdate() bool                                { return false }
func (strategy) PrepareForUpdate(context.Context, runtime.Object, runtime.Object) {}
func (strategy) ValidateUpdate(context.Context, runtime.Object, runtime.Object) field.ErrorList {
	return nil
}
func (strategy) WarningsOnUpdate(context.Context, runtime.Object, runtime.Object) []string {
	return nil
}
func (strategy) AllowUnconditionalUpdate() bool { return true }

type stubStorage struct{ storage.Interface }

type kind struct {
	gv          schema.GroupVersion
	resource    string
	singular    string
	namespaced  bool
	newFunc     func() runtime.Object
	newListFunc func() runtime.Object
}

func store(k kind) *genericregistry.Store {
	strat := strategy{ObjectTyper: scheme.Scheme, NameGenerator: names.SimpleNameGenerator, namespaced: k.namespaced}
	prefix := "/" + k.resource
	gr := k.gv.WithResource(k.resource).GroupResource()
	return &genericregistry.Store{
		NewFunc:                   k.newFunc,
		NewListFunc:               k.newListFunc,
		DefaultQualifiedResource:  gr,
		SingularQualifiedResource: k.gv.WithResource(k.singular).GroupResource(),
		CreateStrategy:            strat,
		UpdateStrategy:            strat,
		DeleteStrategy:            strat,
		TableConvertor:            rest.NewDefaultTableConvertor(gr),
		ObjectNameFunc: func(obj runtime.Object) (string, error) {
			a, err := meta.Accessor(obj)
			if err != nil {
				return "", err
			}
			return a.GetName(), nil
		},
		KeyRootFunc: func(ctx context.Context) string {
			if k.namespaced {
				if ns, ok := genericapirequest.NamespaceFrom(ctx); ok && ns != "" {
					return prefix + "/" + ns
				}
			}
			return prefix
		},
		KeyFunc: func(ctx context.Context, name string) (string, error) {
			if k.namespaced {
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
				return a.GetLabels(), fields.Set{"metadata.name": a.GetName()}, nil
			}}
		},
		Storage: genericregistry.DryRunnableStorage{Storage: stubStorage{}, Codec: codecs.LegacyCodec(k.gv)},
	}
}

var kinds = []kind{
	{corev1.SchemeGroupVersion, "nodes", "node", false, func() runtime.Object { return &corev1.Node{} }, func() runtime.Object { return &corev1.NodeList{} }},
	{corev1.SchemeGroupVersion, "pods", "pod", true, func() runtime.Object { return &corev1.Pod{} }, func() runtime.Object { return &corev1.PodList{} }},
	{corev1.SchemeGroupVersion, "services", "service", true, func() runtime.Object { return &corev1.Service{} }, func() runtime.Object { return &corev1.ServiceList{} }},
	{corev1.SchemeGroupVersion, "events", "event", true, func() runtime.Object { return &corev1.Event{} }, func() runtime.Object { return &corev1.EventList{} }},
	{corev1.SchemeGroupVersion, "namespaces", "namespace", false, func() runtime.Object { return &corev1.Namespace{} }, func() runtime.Object { return &corev1.NamespaceList{} }},
	{corev1.SchemeGroupVersion, "configmaps", "configmap", true, func() runtime.Object { return &corev1.ConfigMap{} }, func() runtime.Object { return &corev1.ConfigMapList{} }},
	{corev1.SchemeGroupVersion, "secrets", "secret", true, func() runtime.Object { return &corev1.Secret{} }, func() runtime.Object { return &corev1.SecretList{} }},
	{coordinationv1.SchemeGroupVersion, "leases", "lease", true, func() runtime.Object { return &coordinationv1.Lease{} }, func() runtime.Object { return &coordinationv1.LeaseList{} }},
	{discoveryv1.SchemeGroupVersion, "endpointslices", "endpointslice", true, func() runtime.Object { return &discoveryv1.EndpointSlice{} }, func() runtime.Object { return &discoveryv1.EndpointSliceList{} }},
	{nodev1.SchemeGroupVersion, "runtimeclasses", "runtimeclass", false, func() runtime.Object { return &nodev1.RuntimeClass{} }, func() runtime.Object { return &nodev1.RuntimeClassList{} }},
	{storagev1.SchemeGroupVersion, "csidrivers", "csidriver", false, func() runtime.Object { return &storagev1.CSIDriver{} }, func() runtime.Object { return &storagev1.CSIDriverList{} }},
}

func install(container *restful.Container, gv schema.GroupVersion, stores map[string]rest.Storage) error {
	root := "/apis"
	if gv.Group == "" {
		root = "/api"
	}
	served := map[string][]string{}
	for r := range stores {
		served[r] = []string{gv.String()}
	}
	group := &endpoints.APIGroupVersion{
		Storage:                     stores,
		Root:                        root,
		GroupVersion:                gv,
		MetaGroupVersion:            &metav1.SchemeGroupVersion,
		AllServedVersionsByResource: served,
		Creater:                     scheme.Scheme,
		Convertor:                   scheme.Scheme,
		Typer:                       scheme.Scheme,
		Defaulter:                   scheme.Scheme,
		ConvertabilityChecker:       scheme.Scheme,
		UnsafeConvertor:             runtime.UnsafeObjectConvertor(scheme.Scheme),
		Namer:                       runtime.Namer(meta.NewAccessor()),
		Serializer:                  codecs,
		ParameterCodec:              runtime.NewParameterCodec(scheme.Scheme),
		EquivalentResourceRegistry:  runtime.NewEquivalentResourceRegistry(),
		TypeConverter:               managedfields.NewDeducedTypeConverter(),
		Admit:                       admission.NewChainHandler(),
	}
	_, _, err := group.InstallREST(container)
	return err
}

func main() {
	byGV := map[schema.GroupVersion]map[string]rest.Storage{}
	for _, k := range kinds {
		if byGV[k.gv] == nil {
			byGV[k.gv] = map[string]rest.Storage{}
		}
		byGV[k.gv][k.resource] = store(k)
	}
	container := restful.NewContainer()
	for gv, stores := range byGV {
		if err := install(container, gv, stores); err != nil {
			panic(err)
		}
	}
	n := 0
	for _, ws := range container.RegisteredWebServices() {
		n += len(ws.Routes())
	}
	fmt.Println("routes:", n)
}
