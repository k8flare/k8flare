package admissionregistration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/k8flare/k8flare/packages/admission"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/utils/ptr"
)

type toServer struct{ base string }

func (t toServer) RoundTrip(req *http.Request) (*http.Response, error) {
	next, err := http.NewRequestWithContext(req.Context(), req.Method, t.base+req.URL.Path, req.Body)
	if err != nil {
		return nil, err
	}
	next.Header = req.Header
	return http.DefaultTransport.RoundTrip(next)
}

func admissionClient(t *testing.T) *http.Client {
	t.Helper()
	srv := httptest.NewServer(admission.NewHandler(admission.Config{Kine: &http.Client{}}))
	t.Cleanup(srv.Close)
	return &http.Client{Transport: toServer{base: srv.URL}}
}

func validatedStore(t *testing.T, name, kind string) *registry.Store {
	t.Helper()
	store := registrytest.Store(t, schema.GroupVersion{Group: "admissionregistration.k8s.io", Version: "v1"}, metav1.APIResource{Name: name, Kind: kind})
	registry.Customizers[name](store, registry.Deps{Admission: admissionClient(t)})
	return store
}

func updateStatus(store *registry.Store, obj, old runtime.Object) error {
	ctx := genericapirequest.WithRequestInfo(registrytest.Context(store), &genericapirequest.RequestInfo{IsResourceRequest: true, Verb: "update", Subresource: "status"})
	strategy := registry.StatusStrategyFor(store)
	strategy.PrepareForUpdate(ctx, obj, old)
	return rest.BeforeUpdate(strategy, ctx, obj, old)
}

func webhook(sideEffects admissionregv1.SideEffectClass) admissionregv1.ValidatingWebhook {
	return admissionregv1.ValidatingWebhook{
		Name:                    "hook.example.com",
		ClientConfig:            admissionregv1.WebhookClientConfig{URL: ptr.To("https://hook.example.com/validate")},
		SideEffects:             &sideEffects,
		AdmissionReviewVersions: []string{"v1"},
	}
}

func webhookConfiguration(hooks ...admissionregv1.ValidatingWebhook) *admissionregv1.ValidatingWebhookConfiguration {
	return &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "hooks", ResourceVersion: "1"},
		Webhooks:   hooks,
	}
}

func TestValidatingWebhookConfigurationRejectsInvalidMatchPolicy(t *testing.T) {
	hook := webhook(admissionregv1.SideEffectClassNone)
	hook.MatchPolicy = ptr.To(admissionregv1.MatchPolicyType("Bogus"))
	err := registrytest.Create(validatedStore(t, "validatingwebhookconfigurations", "ValidatingWebhookConfiguration"), webhookConfiguration(hook))
	registrytest.RequireFieldError(t, err, "webhooks[0].matchPolicy", "Unsupported value")
}

func TestValidatingWebhookConfigurationRejectsSideEffectsSome(t *testing.T) {
	err := registrytest.Create(validatedStore(t, "validatingwebhookconfigurations", "ValidatingWebhookConfiguration"), webhookConfiguration(webhook(admissionregv1.SideEffectClassSome)))
	registrytest.RequireFieldError(t, err, "webhooks[0].sideEffects", "Unsupported value")
}

func TestValidatingWebhookConfigurationRejectsBadMatchCondition(t *testing.T) {
	hook := webhook(admissionregv1.SideEffectClassNone)
	hook.MatchConditions = []admissionregv1.MatchCondition{{Name: "bad", Expression: "... [] bad expression"}}
	err := registrytest.Create(validatedStore(t, "validatingwebhookconfigurations", "ValidatingWebhookConfiguration"), webhookConfiguration(hook))
	registrytest.RequireFieldError(t, err, "webhooks[0].matchConditions[0].expression", "compilation failed")
}

func TestMutatingWebhookConfigurationRejectsMissingAdmissionReviewVersions(t *testing.T) {
	sideEffects := admissionregv1.SideEffectClassNone
	cfg := &admissionregv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "hooks"},
		Webhooks: []admissionregv1.MutatingWebhook{{
			Name:         "hook.example.com",
			ClientConfig: admissionregv1.WebhookClientConfig{URL: ptr.To("https://hook.example.com/mutate")},
			SideEffects:  &sideEffects,
		}},
	}
	err := registrytest.Create(validatedStore(t, "mutatingwebhookconfigurations", "MutatingWebhookConfiguration"), cfg)
	registrytest.RequireFieldError(t, err, "webhooks[0].admissionReviewVersions", "Required value")
}

func TestValidatingWebhookConfigurationUpdateJudgesSideEffectsByTheOldObject(t *testing.T) {
	store := validatedStore(t, "validatingwebhookconfigurations", "ValidatingWebhookConfiguration")

	legacy := webhookConfiguration(webhook(admissionregv1.SideEffectClassSome))
	if err := registrytest.Update(store, legacy.DeepCopy(), legacy); err != nil {
		t.Fatalf("an update that keeps an old sideEffects value must pass: %v", err)
	}

	old := webhookConfiguration(webhook(admissionregv1.SideEffectClassNone))
	err := registrytest.Update(store, webhookConfiguration(webhook(admissionregv1.SideEffectClassSome)), old)
	registrytest.RequireFieldError(t, err, "webhooks[0].sideEffects", "Unsupported value")
}

func policy() *admissionregv1.ValidatingAdmissionPolicy {
	return &admissionregv1.ValidatingAdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy.example.com", ResourceVersion: "1"},
		Spec: admissionregv1.ValidatingAdmissionPolicySpec{
			FailurePolicy: ptr.To(admissionregv1.Fail),
			MatchConstraints: &admissionregv1.MatchResources{
				MatchPolicy:       ptr.To(admissionregv1.Equivalent),
				NamespaceSelector: &metav1.LabelSelector{},
				ObjectSelector:    &metav1.LabelSelector{},
				ResourceRules: []admissionregv1.NamedRuleWithOperations{{
					RuleWithOperations: admissionregv1.RuleWithOperations{
						Operations: []admissionregv1.OperationType{admissionregv1.Create},
						Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
					},
				}},
			},
			Validations: []admissionregv1.Validation{{Expression: "object.metadata.name != ''"}},
		},
	}
}

func TestValidatingAdmissionPolicyAcceptsValidPolicyOnCreateAndUpdate(t *testing.T) {
	store := validatedStore(t, "validatingadmissionpolicies", "ValidatingAdmissionPolicy")
	if err := registrytest.Create(store, policy()); err != nil {
		t.Fatal(err)
	}
	if err := registrytest.Update(store, policy(), policy()); err != nil {
		t.Fatal(err)
	}
}

func TestValidatingAdmissionPolicyRejectsInvalidCEL(t *testing.T) {
	obj := policy()
	obj.Spec.Validations[0].Expression = "object.metadata.name !="
	err := registrytest.Create(validatedStore(t, "validatingadmissionpolicies", "ValidatingAdmissionPolicy"), obj)
	registrytest.RequireFieldError(t, err, "spec.validations[0].expression", "compilation failed")
}

func TestValidatingAdmissionPolicyUpdateRejectsInvalidCEL(t *testing.T) {
	obj := policy()
	obj.Spec.Validations[0].Expression = "object.metadata.name !="
	err := registrytest.Update(validatedStore(t, "validatingadmissionpolicies", "ValidatingAdmissionPolicy"), obj, policy())
	registrytest.RequireFieldError(t, err, "spec.validations[0].expression", "compilation failed")
}

func TestValidatingAdmissionPolicyStatusUpdateUsesTheStatusRules(t *testing.T) {
	store := validatedStore(t, "validatingadmissionpolicies", "ValidatingAdmissionPolicy")
	obj := policy()
	obj.Status.TypeChecking = &admissionregv1.TypeChecking{ExpressionWarnings: []admissionregv1.ExpressionWarning{{FieldRef: "spec.validations[0].expression"}}}
	err := updateStatus(store, obj, policy())
	registrytest.RequireFieldError(t, err, "status.typeChecking.expressionWarnings[0].warning", "Required value")
}

func TestValidatingAdmissionPolicyStatusUpdateIgnoresSpecChanges(t *testing.T) {
	store := validatedStore(t, "validatingadmissionpolicies", "ValidatingAdmissionPolicy")
	obj := policy()
	obj.Spec.Validations[0].Expression = "object.metadata.name !="
	if err := updateStatus(store, obj, policy()); err != nil {
		t.Fatal(err)
	}
}

func TestValidatingAdmissionPolicyBindingRejectsEmptyPolicyName(t *testing.T) {
	obj := &admissionregv1.ValidatingAdmissionPolicyBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "binding.example.com"},
		Spec:       admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny}},
	}
	err := registrytest.Create(validatedStore(t, "validatingadmissionpolicybindings", "ValidatingAdmissionPolicyBinding"), obj)
	registrytest.RequireFieldError(t, err, "spec.policyName", "Required value")
}

func TestValidatingAdmissionPolicyBindingUpdateRejectsUnknownAction(t *testing.T) {
	old := &admissionregv1.ValidatingAdmissionPolicyBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "binding.example.com", ResourceVersion: "1"},
		Spec: admissionregv1.ValidatingAdmissionPolicyBindingSpec{
			PolicyName:        "policy.example.com",
			ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny},
		},
	}
	obj := old.DeepCopy()
	obj.Spec.ValidationActions = []admissionregv1.ValidationAction{"Bogus"}
	err := registrytest.Update(validatedStore(t, "validatingadmissionpolicybindings", "ValidatingAdmissionPolicyBinding"), obj, old)
	registrytest.RequireFieldError(t, err, "spec.validationActions[0]", "Unsupported value")
}

func TestMutatingAdmissionPolicyRejectsInvalidReinvocationPolicy(t *testing.T) {
	obj := &admissionregv1.MutatingAdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "mutate.example.com"},
		Spec: admissionregv1.MutatingAdmissionPolicySpec{
			ReinvocationPolicy: "Bogus",
			FailurePolicy:      ptr.To(admissionregv1.Fail),
			MatchConstraints: &admissionregv1.MatchResources{
				ResourceRules: []admissionregv1.NamedRuleWithOperations{{
					RuleWithOperations: admissionregv1.RuleWithOperations{
						Operations: []admissionregv1.OperationType{admissionregv1.Create},
						Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
					},
				}},
			},
			Mutations: []admissionregv1.Mutation{{
				PatchType:          admissionregv1.PatchTypeApplyConfiguration,
				ApplyConfiguration: &admissionregv1.ApplyConfiguration{Expression: "Object{}"},
			}},
		},
	}
	err := registrytest.Create(validatedStore(t, "mutatingadmissionpolicies", "MutatingAdmissionPolicy"), obj)
	registrytest.RequireFieldError(t, err, "spec.reinvocationPolicy", "Unsupported value")
}

func TestMutatingAdmissionPolicyBindingRejectsEmptyPolicyName(t *testing.T) {
	obj := &admissionregv1.MutatingAdmissionPolicyBinding{ObjectMeta: metav1.ObjectMeta{Name: "binding.example.com"}}
	err := registrytest.Create(validatedStore(t, "mutatingadmissionpolicybindings", "MutatingAdmissionPolicyBinding"), obj)
	registrytest.RequireFieldError(t, err, "spec.policyName", "Required value")
}
