package storage

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/storage"
	"k8s.io/kubernetes/pkg/registry/storage/csidriver"
	"k8s.io/kubernetes/pkg/registry/storage/csinode"
	"k8s.io/kubernetes/pkg/registry/storage/csistoragecapacity"
	"k8s.io/kubernetes/pkg/registry/storage/storageclass"
	"k8s.io/kubernetes/pkg/registry/storage/volumeattachment"
	"k8s.io/kubernetes/pkg/registry/storage/volumeattributesclass"
)

func init() {
	utilruntime.Must(storage.AddToScheme(registry.InternalScheme))
	utilruntime.Must(storagev1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "storage.k8s.io", Resource: resource}
	}
	registry.Upstreams[group("csidrivers")] = registry.Upstream{Strategy: csidriver.Strategy}
	registry.Upstreams[group("csinodes")] = registry.Upstream{Strategy: csinode.Strategy}
	registry.Upstreams[group("csistoragecapacities")] = registry.Upstream{Strategy: csistoragecapacity.Strategy}
	registry.Upstreams[group("storageclasses")] = registry.Upstream{Strategy: storageclass.Strategy}
	registry.Upstreams[group("volumeattachments")] = registry.Upstream{Strategy: volumeattachment.Strategy, Status: volumeattachment.StatusStrategy}
	registry.Upstreams[group("volumeattributesclasses")] = registry.Upstream{Strategy: volumeattributesclass.Strategy}
}
