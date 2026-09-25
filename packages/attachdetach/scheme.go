package attachdetach

import (
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func init() {
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, storagev1.AddToScheme} {
		utilruntime.Must(add(scheme.Scheme))
	}
}
