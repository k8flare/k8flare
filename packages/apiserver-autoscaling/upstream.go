package autoscaling

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/autoscaling"
	"k8s.io/kubernetes/pkg/registry/autoscaling/horizontalpodautoscaler"
)

func init() {
	utilruntime.Must(autoscaling.AddToScheme(registry.InternalScheme))
	utilruntime.Must(autoscalingv1.AddToScheme(registry.InternalScheme))
	utilruntime.Must(autoscalingv2.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "autoscaling", Resource: resource}
	}
	registry.Upstreams[group("horizontalpodautoscalers")] = registry.Upstream{Strategy: horizontalpodautoscaler.Strategy, Status: horizontalpodautoscaler.StatusStrategy}
}
