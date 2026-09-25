package core

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
)

func init() {
	utilruntime.Must(corev1.RegisterDefaults(scheme.Scheme))
}
