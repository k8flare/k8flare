package admissionregistration

import (
	"strings"
	"testing"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCompileRejectsBadMatchCondition(t *testing.T) {
	cfg := &admissionregv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "bad"},
		Webhooks: []admissionregv1.MutatingWebhook{{
			Name: "hook.example.com",
			MatchConditions: []admissionregv1.MatchCondition{{
				Name:       "invalid-expression-1",
				Expression: "... [] bad expression",
			}},
		}},
	}
	errs := compileAdmissionCEL(cfg)
	if len(errs) == 0 {
		t.Fatal("expected compile error")
	}
	if !strings.Contains(errs.ToAggregate().Error(), "compilation failed") {
		t.Fatalf("%v", errs)
	}
}

func TestCompileAcceptsObjectName(t *testing.T) {
	cfg := &admissionregv1.ValidatingWebhookConfiguration{
		Webhooks: []admissionregv1.ValidatingWebhook{{
			MatchConditions: []admissionregv1.MatchCondition{{
				Name:       "skip-me",
				Expression: "object.metadata.name != 'skip-me'",
			}},
		}},
	}
	if errs := compileAdmissionCEL(cfg); len(errs) != 0 {
		t.Fatal(errs)
	}
}
