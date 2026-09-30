package scheduling

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	schedulingv1 "k8s.io/api/scheduling/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/scheduling"
	"k8s.io/kubernetes/pkg/registry/scheduling/priorityclass"
)

func init() {
	utilruntime.Must(scheduling.AddToScheme(registry.InternalScheme))
	utilruntime.Must(schedulingv1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "scheduling.k8s.io", Resource: resource}
	}
	registry.Upstreams[group("priorityclasses")] = registry.Upstream{Strategy: priorityclass.Strategy}
}
