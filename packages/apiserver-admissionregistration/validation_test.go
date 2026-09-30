package admissionregistration

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gv = schema.GroupVersion{Group: "admissionregistration.k8s.io", Version: "v1"}

func TestWebhookConfigurationRejectsShortWebhookName(t *testing.T) {
	store := registrytest.Store(t, gv, metav1.APIResource{Name: "validatingwebhookconfigurations", Kind: "ValidatingWebhookConfiguration"})
	obj := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "w"},
		Webhooks:   []admissionregv1.ValidatingWebhook{{Name: "short"}},
	}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "webhooks[0].name", "should be a domain with at least three segments")
}

func TestPolicyBindingRejectsMissingPolicyName(t *testing.T) {
	store := registrytest.Store(t, gv, metav1.APIResource{Name: "validatingadmissionpolicybindings", Kind: "ValidatingAdmissionPolicyBinding"})
	obj := &admissionregv1.ValidatingAdmissionPolicyBinding{ObjectMeta: metav1.ObjectMeta{Name: "b"}}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.policyName", "")
}

func TestMutatingPolicyBindingRejectsMissingPolicyName(t *testing.T) {
	store := registrytest.Store(t, gv, metav1.APIResource{Name: "mutatingadmissionpolicybindings", Kind: "MutatingAdmissionPolicyBinding"})
	obj := &admissionregv1.MutatingAdmissionPolicyBinding{ObjectMeta: metav1.ObjectMeta{Name: "b"}}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.policyName", "")
}

func TestMutatingPolicyRejectsEmptyMutations(t *testing.T) {
	store := registrytest.Store(t, gv, metav1.APIResource{Name: "mutatingadmissionpolicies", Kind: "MutatingAdmissionPolicy"})
	obj := &admissionregv1.MutatingAdmissionPolicy{ObjectMeta: metav1.ObjectMeta{Name: "p"}}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.mutations", "")
}

func TestPolicyRejectsEmptyValidations(t *testing.T) {
	store := registrytest.Store(t, gv, metav1.APIResource{Name: "validatingadmissionpolicies", Kind: "ValidatingAdmissionPolicy"})
	obj := &admissionregv1.ValidatingAdmissionPolicy{ObjectMeta: metav1.ObjectMeta{Name: "p"}}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.validations", "validations or auditAnnotations must contain at least one item")
}

func TestWebhookConfigurationRejectsBadMatchCondition(t *testing.T) {
	store := registrytest.Store(t, gv, metav1.APIResource{Name: "mutatingwebhookconfigurations", Kind: "MutatingWebhookConfiguration"})
	obj := &admissionregv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "bad"},
		Webhooks: []admissionregv1.MutatingWebhook{{
			Name:                    "hook.example.com",
			ClientConfig:            admissionregv1.WebhookClientConfig{URL: ptr("https://example.com/hook")},
			SideEffects:             ptr(admissionregv1.SideEffectClassNone),
			AdmissionReviewVersions: []string{"v1"},
			MatchConditions: []admissionregv1.MatchCondition{{
				Name:       "invalid-expression-1",
				Expression: "... [] bad expression",
			}},
		}},
	}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "webhooks[0].matchConditions[0].expression", "compilation failed")
}

func TestWebhookConfigurationAcceptsObjectName(t *testing.T) {
	store := registrytest.Store(t, gv, metav1.APIResource{Name: "validatingwebhookconfigurations", Kind: "ValidatingWebhookConfiguration"})
	obj := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "ok"},
		Webhooks: []admissionregv1.ValidatingWebhook{{
			Name:                    "hook.example.com",
			ClientConfig:            admissionregv1.WebhookClientConfig{URL: ptr("https://example.com/hook")},
			SideEffects:             ptr(admissionregv1.SideEffectClassNone),
			AdmissionReviewVersions: []string{"v1"},
			MatchConditions: []admissionregv1.MatchCondition{{
				Name:       "skip-me",
				Expression: "object.metadata.name != 'skip-me'",
			}},
		}},
	}
	if err := registrytest.Create(store, obj); err != nil {
		t.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }
