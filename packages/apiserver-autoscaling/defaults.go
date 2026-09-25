package autoscaling

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	autoscalingv1 "k8s.io/kubernetes/pkg/apis/autoscaling/v1"
	autoscalingv2 "k8s.io/kubernetes/pkg/apis/autoscaling/v2"
)

func init() {
	utilruntime.Must(autoscalingv1.RegisterDefaults(scheme.Scheme))
	utilruntime.Must(autoscalingv2.RegisterDefaults(scheme.Scheme))
}
