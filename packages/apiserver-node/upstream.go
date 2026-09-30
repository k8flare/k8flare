package node

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	nodev1 "k8s.io/api/node/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/node"
	"k8s.io/kubernetes/pkg/registry/node/runtimeclass"
)

func init() {
	utilruntime.Must(node.AddToScheme(registry.InternalScheme))
	utilruntime.Must(nodev1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "node.k8s.io", Resource: resource}
	}
	registry.Upstreams[group("runtimeclasses")] = registry.Upstream{Strategy: runtimeclass.Strategy}
}
