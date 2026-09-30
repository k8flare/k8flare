package discovery

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/discovery"
	"k8s.io/kubernetes/pkg/registry/discovery/endpointslice"
)

func init() {
	utilruntime.Must(discovery.AddToScheme(registry.InternalScheme))
	utilruntime.Must(discoveryv1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "discovery.k8s.io", Resource: resource}
	}
	registry.Upstreams[group("endpointslices")] = registry.Upstream{Strategy: endpointslice.Strategy}
}
