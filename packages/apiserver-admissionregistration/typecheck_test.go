package admissionregistration

import (
	"encoding/json"
	"strings"
	"testing"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTypeCheckPolicyDeployment(t *testing.T) {
	policy := &admissionregv1.ValidatingAdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p"},
		Spec: admissionregv1.ValidatingAdmissionPolicySpec{
			MatchConstraints: &admissionregv1.MatchResources{ResourceRules: []admissionregv1.NamedRuleWithOperations{{
				RuleWithOperations: admissionregv1.RuleWithOperations{
					Operations: []admissionregv1.OperationType{admissionregv1.Create},
					Rule:       admissionregv1.Rule{APIGroups: []string{"apps"}, APIVersions: []string{"v1"}, Resources: []string{"deployments"}},
				},
			}}},
			Validations: []admissionregv1.Validation{{
				Expression:        "object.spec.replicas > '1'",
				MessageExpression: "'wants replicas > 1, got ' + object.spec.replicas",
			}},
		},
	}
	got := typeCheckPolicy(policy)
	if got == nil || len(got.ExpressionWarnings) != 2 {
		t.Fatalf("warnings: %+v", got)
	}
	if got.ExpressionWarnings[0].FieldRef != "spec.validations[0].expression" {
		t.Fatalf("field: %q", got.ExpressionWarnings[0].FieldRef)
	}
	if !strings.Contains(got.ExpressionWarnings[0].Warning, "found no matching overload for '_>_' applied to '(int, string)'") {
		t.Fatalf("expression warning: %q", got.ExpressionWarnings[0].Warning)
	}
	if got.ExpressionWarnings[1].FieldRef != "spec.validations[0].messageExpression" {
		t.Fatalf("field: %q", got.ExpressionWarnings[1].FieldRef)
	}
	if !strings.Contains(got.ExpressionWarnings[1].Warning, "found no matching overload for '_+_' applied to '(string, int)'") {
		t.Fatalf("message warning: %q", got.ExpressionWarnings[1].Warning)
	}

	ok := typeCheckPolicy(&admissionregv1.ValidatingAdmissionPolicy{
		Spec: admissionregv1.ValidatingAdmissionPolicySpec{
			MatchConstraints: policy.Spec.MatchConstraints,
			Validations:      []admissionregv1.Validation{{Expression: "object.spec.replicas > 1"}},
		},
	})
	if ok == nil || len(ok.ExpressionWarnings) != 0 {
		t.Fatalf("correct policy: %+v", ok)
	}
}

func TestTypeCheckPolicyCRDSchema(t *testing.T) {
	schema := schemaNode{
		Type: "object",
		Properties: map[string]schemaNode{
			"spec": {
				Type: "object",
				Properties: map[string]schemaNode{
					"cronSpec": {Type: "string"},
					"image":    {Type: "string"},
					"replicas": {Type: "integer"},
				},
			},
		},
	}
	if w := schemaCheckExpr(schema, "object.spec.replicas > 1"); w != "" {
		t.Fatalf("correct: %q", w)
	}
	got := schemaCheckExpr(schema, "object.spec.replicas > '1'")
	if !strings.Contains(got, "found no matching overload for '_>_' applied to '(int, string)'") {
		t.Fatalf("type confuse: %q", got)
	}
	got = schemaCheckExpr(schema, "object.spec.maxRetries < 10")
	if !strings.Contains(got, "undefined field 'maxRetries'") {
		t.Fatalf("missing field: %q", got)
	}
}

func TestTypeCheckOfficialConfusedCRD(t *testing.T) {
	schema := schemaNode{
		Type: "object",
		Properties: map[string]schemaNode{
			"spec": {
				Type: "object",
				Properties: map[string]schemaNode{
					"cronSpec": {Type: "string"},
					"image":    {Type: "string"},
					"replicas": {Type: "integer"},
				},
			},
		},
	}
	raw := []byte(`{"spec":{"group":"stable.validating-admission-policy-7851.example.com","names":{"plural":"crontabs"},"versions":[{"storage":true,"schema":{"openAPIV3Schema":{"type":"object","properties":{"spec":{"type":"object","properties":{"cronSpec":{"type":"string"},"image":{"type":"string"},"replicas":{"type":"integer"}}}}}}}]}}`)
	var crd crdLite
	if err := json.Unmarshal(raw, &crd); err != nil {
		t.Fatal(err)
	}
	parsed, ok := storageSchema(crd)
	if !ok {
		t.Fatal("storage schema")
	}
	if parsed.Properties["spec"].Properties["replicas"].Type != "integer" {
		t.Fatalf("replicas type: %+v", parsed)
	}
	policy := &admissionregv1.ValidatingAdmissionPolicy{
		Spec: admissionregv1.ValidatingAdmissionPolicySpec{
			MatchConstraints: &admissionregv1.MatchResources{ResourceRules: []admissionregv1.NamedRuleWithOperations{{
				RuleWithOperations: admissionregv1.RuleWithOperations{
					Operations: []admissionregv1.OperationType{admissionregv1.Create},
					Rule:       admissionregv1.Rule{APIGroups: []string{crd.Spec.Group}, APIVersions: []string{"v1"}, Resources: []string{"crontabs"}},
				},
			}}},
			Validations: []admissionregv1.Validation{
				{Expression: "object.spec.replicas > '1'"},
				{Expression: "object.spec.maxRetries < 10"},
			},
		},
	}
	got := collectTypeWarnings(policy, func(expr string) string { return schemaCheckExpr(schema, expr) })
	if got == nil || len(got.ExpressionWarnings) != 2 {
		t.Fatalf("warnings: %+v", got)
	}
	if got.ExpressionWarnings[0].FieldRef != "spec.validations[0].expression" ||
		!strings.Contains(got.ExpressionWarnings[0].Warning, "found no matching overload for '_>_' applied to '(int, string)'") {
		t.Fatalf("first: %+v", got.ExpressionWarnings[0])
	}
	if got.ExpressionWarnings[1].FieldRef != "spec.validations[1].expression" ||
		!strings.Contains(got.ExpressionWarnings[1].Warning, "undefined field 'maxRetries'") {
		t.Fatalf("second: %+v", got.ExpressionWarnings[1])
	}
	okPolicy := collectTypeWarnings(&admissionregv1.ValidatingAdmissionPolicy{
		Spec: admissionregv1.ValidatingAdmissionPolicySpec{
			Validations: []admissionregv1.Validation{{Expression: "object.spec.replicas > 1"}},
		},
	}, func(expr string) string { return schemaCheckExpr(schema, expr) })
	if okPolicy == nil || len(okPolicy.ExpressionWarnings) != 0 {
		t.Fatalf("correct: %+v", okPolicy)
	}
}
