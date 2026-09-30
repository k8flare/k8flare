package coordination

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/coordination"
	"k8s.io/kubernetes/pkg/registry/coordination/lease"
)

func init() {
	utilruntime.Must(coordination.AddToScheme(registry.InternalScheme))
	utilruntime.Must(coordinationv1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "coordination.k8s.io", Resource: resource}
	}
	registry.Upstreams[group("leases")] = registry.Upstream{Strategy: lease.Strategy}
}
