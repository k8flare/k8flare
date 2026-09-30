package resource

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/resource"
	"k8s.io/kubernetes/pkg/registry/resource/deviceclass"
	"k8s.io/kubernetes/pkg/registry/resource/resourceslice"
)

func init() {
	utilruntime.Must(resource.AddToScheme(registry.InternalScheme))
	utilruntime.Must(resourcev1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "resource.k8s.io", Resource: resource}
	}
	registry.Upstreams[group("deviceclasses")] = registry.Upstream{Strategy: deviceclass.Strategy}
	registry.Upstreams[group("resourceslices")] = registry.Upstream{Strategy: resourceslice.Strategy}
}
