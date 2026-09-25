package storageconv

import (
	"k8s.io/client-go/kubernetes/scheme"
	autoscalinginstall "k8s.io/kubernetes/pkg/apis/autoscaling/install"
)

func Install() {
	autoscalinginstall.Install(scheme.Scheme)
}
