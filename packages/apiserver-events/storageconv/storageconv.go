package storageconv

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	coreapi "k8s.io/kubernetes/pkg/apis/core"
	coreapiv1 "k8s.io/kubernetes/pkg/apis/core/v1"
	eventsapi "k8s.io/kubernetes/pkg/apis/events"
)

func Install() {
	runtime.Must(coreapi.AddToScheme(scheme.Scheme))
	runtime.Must(coreapiv1.AddToScheme(scheme.Scheme))
	runtime.Must(eventsapi.AddToScheme(scheme.Scheme))
}
