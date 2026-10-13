package admission

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/registry/admissionregistration/mutatingwebhookconfiguration"
	"k8s.io/kubernetes/pkg/registry/admissionregistration/validatingadmissionpolicy"
	"k8s.io/kubernetes/pkg/registry/admissionregistration/validatingwebhookconfiguration"
	"k8s.io/utils/ptr"
)

type registrationCase struct {
	name     string
	resource string
	obj      runtime.Object
	old      runtime.Object
	status   bool
	accepted bool
}

func registrationHook(sideEffects admissionregv1.SideEffectClass) (admissionregv1.WebhookClientConfig, *admissionregv1.SideEffectClass) {
	return admissionregv1.WebhookClientConfig{URL: ptr.To("https://hook.example.com/hook")}, &sideEffects
}

func validatingConfiguration(sideEffects admissionregv1.SideEffectClass, mutate func(*admissionregv1.ValidatingWebhook)) *admissionregv1.ValidatingWebhookConfiguration {
	client, effects := registrationHook(sideEffects)
	hook := admissionregv1.ValidatingWebhook{Name: "hook.example.com", ClientConfig: client, SideEffects: effects, AdmissionReviewVersions: []string{"v1"}}
	if mutate != nil {
		mutate(&hook)
	}
	return &admissionregv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "hooks"}, Webhooks: []admissionregv1.ValidatingWebhook{hook}}
}

func mutatingConfiguration(mutate func(*admissionregv1.MutatingWebhook)) *admissionregv1.MutatingWebhookConfiguration {
	client, effects := registrationHook(admissionregv1.SideEffectClassNone)
	hook := admissionregv1.MutatingWebhook{Name: "hook.example.com", ClientConfig: client, SideEffects: effects, AdmissionReviewVersions: []string{"v1"}}
	if mutate != nil {
		mutate(&hook)
	}
	return &admissionregv1.MutatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "hooks"}, Webhooks: []admissionregv1.MutatingWebhook{hook}}
}

func registrationPolicy(mutate func(*admissionregv1.ValidatingAdmissionPolicy)) *admissionregv1.ValidatingAdmissionPolicy {
	p := &admissionregv1.ValidatingAdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy.example.com"},
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
				}}},
			Validations: []admissionregv1.Validation{{Expression: "object.metadata.name != ''"}},
		},
	}
	if mutate != nil {
		mutate(p)
	}
	return p
}

func upstreamRegistrationErrors(t *testing.T, c registrationCase) field.ErrorList {
	t.Helper()
	obj, err := toInternalObject(c.obj)
	if err != nil {
		t.Fatal(err)
	}
	var old runtime.Object
	if c.old != nil {
		if old, err = toInternalObject(c.old); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	switch c.resource {
	case "validatingwebhookconfigurations":
		if old == nil {
			return validatingwebhookconfiguration.Strategy.Validate(ctx, obj)
		}
		return validatingwebhookconfiguration.Strategy.ValidateUpdate(ctx, obj, old)
	case "mutatingwebhookconfigurations":
		if old == nil {
			return mutatingwebhookconfiguration.Strategy.Validate(ctx, obj)
		}
		return mutatingwebhookconfiguration.Strategy.ValidateUpdate(ctx, obj, old)
	}
	strategy := validatingadmissionpolicy.NewStrategy(nil, nil)
	switch {
	case old == nil:
		return strategy.Validate(ctx, obj)
	case c.status:
		return validatingadmissionpolicy.NewStatusStrategy(strategy).ValidateUpdate(ctx, obj, old)
	default:
		return strategy.ValidateUpdate(ctx, obj, old)
	}
}

func summarize(errs field.ErrorList) []string {
	var out []string
	for _, e := range errs {
		out = append(out, string(e.Type)+" "+e.Field+" "+e.Detail)
	}
	return out
}

func TestRegistrationValidationMatchesUpstreamStrategies(t *testing.T) {
	invalidCEL := func(p *admissionregv1.ValidatingAdmissionPolicy) {
		p.Spec.Validations[0].Expression = "object.metadata.name !="
	}
	cases := []registrationCase{
		{name: "valid webhook configuration", accepted: true, resource: "validatingwebhookconfigurations", obj: validatingConfiguration(admissionregv1.SideEffectClassNone, nil)},
		{name: "bad matchPolicy", resource: "validatingwebhookconfigurations", obj: validatingConfiguration(admissionregv1.SideEffectClassNone, func(h *admissionregv1.ValidatingWebhook) {
			h.MatchPolicy = ptr.To(admissionregv1.MatchPolicyType("Bogus"))
		})},
		{name: "bad match condition", resource: "validatingwebhookconfigurations", obj: validatingConfiguration(admissionregv1.SideEffectClassNone, func(h *admissionregv1.ValidatingWebhook) {
			h.MatchConditions = []admissionregv1.MatchCondition{{Name: "bad", Expression: "... [] bad"}}
		})},
		{name: "sideEffects Some on create", resource: "validatingwebhookconfigurations", obj: validatingConfiguration(admissionregv1.SideEffectClassSome, nil)},
		{name: "sideEffects Some kept on update", accepted: true, resource: "validatingwebhookconfigurations", obj: validatingConfiguration(admissionregv1.SideEffectClassSome, nil), old: validatingConfiguration(admissionregv1.SideEffectClassSome, nil)},
		{name: "sideEffects Some introduced on update", resource: "validatingwebhookconfigurations", obj: validatingConfiguration(admissionregv1.SideEffectClassSome, nil), old: validatingConfiguration(admissionregv1.SideEffectClassNone, nil)},
		{name: "missing admissionReviewVersions", resource: "mutatingwebhookconfigurations", obj: mutatingConfiguration(func(h *admissionregv1.MutatingWebhook) { h.AdmissionReviewVersions = nil })},
		{name: "bad reinvocationPolicy", resource: "mutatingwebhookconfigurations", obj: mutatingConfiguration(func(h *admissionregv1.MutatingWebhook) {
			h.ReinvocationPolicy = ptr.To(admissionregv1.ReinvocationPolicyType("Bogus"))
		})},
		{name: "valid policy", accepted: true, resource: "validatingadmissionpolicies", obj: registrationPolicy(nil)},
		{name: "invalid CEL on create", resource: "validatingadmissionpolicies", obj: registrationPolicy(invalidCEL)},
		{name: "invalid CEL on update", resource: "validatingadmissionpolicies", obj: registrationPolicy(invalidCEL), old: registrationPolicy(nil)},
		{name: "invalid CEL kept on update", resource: "validatingadmissionpolicies", obj: registrationPolicy(invalidCEL), old: registrationPolicy(invalidCEL)},
		{name: "status with empty warning", resource: "validatingadmissionpolicies", status: true, old: registrationPolicy(nil), obj: registrationPolicy(func(p *admissionregv1.ValidatingAdmissionPolicy) {
			p.Status.TypeChecking = &admissionregv1.TypeChecking{ExpressionWarnings: []admissionregv1.ExpressionWarning{{FieldRef: "spec.validations[0].expression"}}}
		})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(c.obj)
			if err != nil {
				t.Fatal(err)
			}
			req := admit.ValidateRequest{Resource: c.resource, Object: obj}
			if c.old != nil {
				req.Update = true
				if req.OldObject, err = runtime.DefaultUnstructuredConverter.ToUnstructured(c.old); err != nil {
					t.Fatal(err)
				}
			}
			if c.status {
				req.Subresource = "status"
			}
			got, err := validateRegistration(req)
			if err != nil {
				t.Fatal(err)
			}
			upstream := upstreamRegistrationErrors(t, c)
			if c.accepted != (len(upstream) == 0) {
				t.Fatalf("upstream errors = %v, want accepted = %v", summarize(upstream), c.accepted)
			}
			if diff := cmp.Diff(summarize(upstream), summarize(got)); diff != "" {
				t.Fatalf("worker and upstream strategy disagree (-upstream +worker):\n%s", diff)
			}
		})
	}
}
