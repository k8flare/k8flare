package discovery

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "discovery.k8s.io", Version: "v1"}, metav1.APIResource{Name: "endpointslices", Kind: "EndpointSlice", Namespaced: true})
	obj := &discoveryv1.EndpointSlice{
		ObjectMeta:  metav1.ObjectMeta{Name: "e", Namespace: "default"},
		AddressType: discoveryv1.AddressType("Bogus"),
	}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "addressType", "Unsupported value")
}
