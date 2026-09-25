//go:build !js

package openapi

import (
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func init() {
	runtime.Must(eventsv1.AddToScheme(scheme.Scheme))
}
