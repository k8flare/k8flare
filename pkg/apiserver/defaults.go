package apiserver

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	coordinationv1 "k8s.io/kubernetes/pkg/apis/coordination/v1"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	discoveryv1 "k8s.io/kubernetes/pkg/apis/discovery/v1"
	storagev1 "k8s.io/kubernetes/pkg/apis/storage/v1"
)

// The real upstream defaulting functions, applied by the codecs on every
// decode. The kubelet relies on them: it refuses to start a container from
// a Pod whose spec.enableServiceLinks was never defaulted.
func init() {
	for _, register := range []func(*runtime.Scheme) error{
		corev1.RegisterDefaults, coordinationv1.RegisterDefaults, discoveryv1.RegisterDefaults, storagev1.RegisterDefaults,
	} {
		if err := register(scheme.Scheme); err != nil {
			panic(err)
		}
	}
}
