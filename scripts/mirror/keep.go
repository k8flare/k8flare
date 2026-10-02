package main

type keptGroup struct {
	Name            string
	Versions        []string
	InformerFactory bool
	NarrowInformer  bool
}

var keptAPIs = []keptGroup{
	{Name: "admissionregistration", Versions: []string{"v1"}, InformerFactory: true, NarrowInformer: true},
	{Name: "apps", Versions: []string{"v1"}, InformerFactory: true, NarrowInformer: true},
	{Name: "autoscaling", Versions: []string{"v1", "v2"}, InformerFactory: true},
	{Name: "authorization", Versions: []string{"v1"}},
	{Name: "batch", Versions: []string{"v1"}, InformerFactory: true, NarrowInformer: true},
	{Name: "certificates", Versions: []string{"v1"}},
	{Name: "coordination", Versions: []string{"v1"}, InformerFactory: true, NarrowInformer: true},
	{Name: "core", Versions: []string{"v1"}, InformerFactory: true},
	{Name: "discovery", Versions: []string{"v1"}, InformerFactory: true, NarrowInformer: true},
	{Name: "events", Versions: []string{"v1"}},
	{Name: "networking", Versions: []string{"v1"}, InformerFactory: true, NarrowInformer: true},
	{Name: "policy", Versions: []string{"v1"}, InformerFactory: true, NarrowInformer: true},
	{Name: "rbac", Versions: []string{"v1"}},
	{Name: "resource", Versions: []string{"v1", "v1beta2"}, InformerFactory: true, NarrowInformer: true},
	{Name: "scheduling", Versions: []string{"v1", "v1alpha2"}, InformerFactory: true, NarrowInformer: true},
	{Name: "storage", Versions: []string{"v1"}, InformerFactory: true, NarrowInformer: true},
}

const genericInformerStub = `package informers

import (
	fmt "fmt"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	cache "k8s.io/client-go/tools/cache"
)

type GenericInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() cache.GenericLister
}

func (f *sharedInformerFactory) ForResource(resource schema.GroupVersionResource) (GenericInformer, error) {
	return nil, fmt.Errorf("informers: ForResource is not available in this build")
}
`
