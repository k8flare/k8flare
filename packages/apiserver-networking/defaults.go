package networking

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	networkingv1 "k8s.io/kubernetes/pkg/apis/networking/v1"
)

func init() {
	utilruntime.Must(networkingv1.RegisterDefaults(scheme.Scheme))
}
