package flowcontrol

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	flowcontrolv1 "k8s.io/kubernetes/pkg/apis/flowcontrol/v1"
)

func init() {
	utilruntime.Must(flowcontrolv1.RegisterDefaults(scheme.Scheme))
}
