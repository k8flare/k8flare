package networking

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "networking.k8s.io", Version: "v1"}, metav1.APIResource{Name: "ingressclasses", Kind: "IngressClass"})
	obj := &networkingv1.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: "i"}}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.controller", "Required value")
}
