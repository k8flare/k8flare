package admissionregistration

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "admissionregistration.k8s.io", Version: "v1"}, metav1.APIResource{Name: "mutatingwebhookconfigurations", Kind: "MutatingWebhookConfiguration"})
	obj := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "m"},
		Webhooks:   []admissionregistrationv1.MutatingWebhook{{Name: "x"}},
	}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "webhooks[0].name", "should be a domain with at least three segments")
}
