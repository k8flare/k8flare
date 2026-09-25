package scheduling

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	schedulingv1 "k8s.io/kubernetes/pkg/apis/scheduling/v1"
)

func init() {
	utilruntime.Must(schedulingv1.RegisterDefaults(scheme.Scheme))
}
