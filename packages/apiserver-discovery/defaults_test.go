package discovery

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDefaultsEndpointPort(t *testing.T) {
	slice := &discoveryv1.EndpointSlice{Ports: []discoveryv1.EndpointPort{{}}}
	scheme.Scheme.Default(slice)
	if slice.Ports[0].Protocol == nil || *slice.Ports[0].Protocol != corev1.ProtocolTCP {
		t.Fatalf("protocol=%v", slice.Ports[0].Protocol)
	}
	if slice.Ports[0].Name == nil {
		t.Fatal("name was not defaulted")
	}
}
