package registry

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	coordinationv1 "k8s.io/kubernetes/pkg/apis/coordination/v1"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	discoveryv1 "k8s.io/kubernetes/pkg/apis/discovery/v1"
	nodev1 "k8s.io/kubernetes/pkg/apis/node/v1"
	storagev1 "k8s.io/kubernetes/pkg/apis/storage/v1"
)

// Upstream's own registrations for the served groups: the defaulting
// functions the kubelet depends on, the field label conversions behind
// selectors such as spec.nodeName, and the query-parameter conversions for
// PodLogOptions. These packages hold no internal types themselves.
func init() {
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, coordinationv1.AddToScheme, discoveryv1.AddToScheme, nodev1.AddToScheme, storagev1.AddToScheme,
	} {
		if err := add(scheme.Scheme); err != nil {
			panic(err)
		}
	}
}
