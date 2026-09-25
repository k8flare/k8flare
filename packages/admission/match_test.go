package admission

import (
	"fmt"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestEvalVariables(t *testing.T) {
	vars := celVars(admit.Request{Object: map[string]any{"spec": map[string]any{"replicas": float64(3)}}})
	got := evalVariables([]admissionregv1.Variable{
		{Name: "replicas", Expression: "object.spec.replicas"},
		{Name: "oddReplicas", Expression: "variables.replicas % 2 == 1"},
	}, vars)
	if got["replicas"] != int64(3) {
		t.Fatalf("replicas: %v (%T)", got["replicas"], got["replicas"])
	}
	if got["oddReplicas"] != true {
		t.Fatalf("oddReplicas: %v", got["oddReplicas"])
	}
	ok, err := evalBool("variables.oddReplicas", vars)
	if err != nil || !ok {
		t.Fatalf("variables.oddReplicas: ok=%v err=%v", ok, err)
	}
}

func TestExemptAdmissionConfig(t *testing.T) {
	if !exemptAdmissionConfig(admit.Request{Kind: schema.GroupVersionKind{Group: "admissionregistration.k8s.io", Kind: "MutatingWebhookConfiguration"}}) {
		t.Fatal("mutating webhook config should be exempt")
	}
	if exemptAdmissionConfig(admit.Request{Kind: schema.GroupVersionKind{Group: "", Kind: "ConfigMap"}}) {
		t.Fatal("configmap should not be exempt")
	}
}

func TestDenyMessageUsesReason(t *testing.T) {
	if got := denyMessage(&metav1.Status{Reason: "the configmap contains unwanted key and value"}); got != "the configmap contains unwanted key and value" {
		t.Fatalf("reason: %q", got)
	}
	if got := denyMessage(&metav1.Status{Message: "msg", Reason: "reason"}); got != "msg" {
		t.Fatalf("message wins: %q", got)
	}
	if got := denyMessage(nil); got != "without explanation" {
		t.Fatalf("nil: %q", got)
	}
}

func TestVAPDenyIsInvalid(t *testing.T) {
	out := denyResponse(invalidError{msg: "ValidatingAdmissionPolicy example denied the request"})
	if out.Allowed || out.Reason != "Invalid" {
		t.Fatalf("%+v", out)
	}
}

func TestFailClosed(t *testing.T) {
	ignore := admissionregv1.Ignore
	if err := failClosed(&ignore, fmt.Errorf("x509")); err != nil {
		t.Fatal(err)
	}
	fail := admissionregv1.Fail
	err := failClosed(&fail, fmt.Errorf("x509"))
	if _, ok := err.(internalError); !ok {
		t.Fatalf("got %T %v", err, err)
	}
	out := denyResponse(err)
	if out.Reason != "InternalError" {
		t.Fatalf("reason: %q", out.Reason)
	}
}

func TestMatchRules(t *testing.T) {
	all := admissionregv1.OperationAll
	req := admit.Request{
		Operation: "CREATE",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Namespace: "default",
	}
	if !matchRules(req, []admissionregv1.RuleWithOperations{{
		Operations: []admissionregv1.OperationType{all},
		Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
	}}, nil) {
		t.Fatal("expected match")
	}
	if matchRules(req, []admissionregv1.RuleWithOperations{{
		Operations: []admissionregv1.OperationType{"UPDATE"},
		Rule:       admissionregv1.Rule{APIGroups: []string{"*"}, APIVersions: []string{"*"}, Resources: []string{"*"}},
	}}, nil) {
		t.Fatal("operation should not match")
	}
	status := req
	status.Subresource = "status"
	if matchRules(status, []admissionregv1.RuleWithOperations{{
		Operations: []admissionregv1.OperationType{all},
		Rule:       admissionregv1.Rule{APIGroups: []string{"*"}, APIVersions: []string{"*"}, Resources: []string{"pods"}},
	}}, nil) {
		t.Fatal("pods should not match pods/status")
	}
	if !matchRules(status, []admissionregv1.RuleWithOperations{{
		Operations: []admissionregv1.OperationType{all},
		Rule:       admissionregv1.Rule{APIGroups: []string{"*"}, APIVersions: []string{"*"}, Resources: []string{"pods/status"}},
	}}, nil) {
		t.Fatal("pods/status should match")
	}
}

func TestMatchCRDObjectSelector(t *testing.T) {
	req := admit.Request{
		Operation: "CREATE",
		Resource:  schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"},
		Kind:      schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"},
		Object: map[string]any{
			"metadata": map[string]any{"name": "deny.example.com", "labels": map[string]any{"webhook-e2e-test": "webhook-disallow", "unique": "true"}},
		},
	}
	rules := []admissionregv1.RuleWithOperations{{
		Operations: []admissionregv1.OperationType{admissionregv1.Create},
		Rule:       admissionregv1.Rule{APIGroups: []string{"apiextensions.k8s.io"}, APIVersions: []string{"*"}, Resources: []string{"customresourcedefinitions"}},
	}}
	sel := &metav1.LabelSelector{MatchLabels: map[string]string{"unique": "true"}}
	if !matchWebhook(req, rules, nil, sel, nil, nil) {
		t.Fatal("cluster-scoped CRD should match objectSelector")
	}
	req.Object["metadata"].(map[string]any)["labels"] = map[string]any{"webhook-e2e-test": "webhook-disallow"}
	if matchWebhook(req, rules, nil, sel, nil, nil) {
		t.Fatal("CRD without selector label should not match")
	}
}

func TestMatchConditions(t *testing.T) {
	req := admit.Request{
		Operation: "CREATE",
		Name:      "web",
		Object:    map[string]any{"metadata": map[string]any{"name": "web"}},
	}
	ok, err := matchConditions(req, []admissionregv1.MatchCondition{{Name: "name", Expression: `object.metadata.name == "web"`}})
	if err != nil || !ok {
		t.Fatalf("want match, got ok=%v err=%v", ok, err)
	}
	ok, err = matchConditions(req, []admissionregv1.MatchCondition{{Name: "name", Expression: `object.metadata.name == "other"`}})
	if err != nil || ok {
		t.Fatalf("want skip, got ok=%v err=%v", ok, err)
	}
}

func TestMatchConnectPodAttach(t *testing.T) {
	req := admit.Request{
		Operation:   "CONNECT",
		Namespace:   "webhook",
		Resource:    schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Subresource: "attach",
		Kind:        schema.GroupVersionKind{Version: "v1", Kind: "PodAttachOptions"},
	}
	rules := []admissionregv1.RuleWithOperations{{
		Operations: []admissionregv1.OperationType{admissionregv1.Connect},
		Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods/attach"}},
	}}
	sel := &metav1.LabelSelector{MatchLabels: map[string]string{"unique": "true"}}
	if !matchWebhook(req, rules, sel, nil, map[string]string{"unique": "true"}, nil) {
		t.Fatal("CONNECT pods/attach should match")
	}
	if matchWebhook(req, rules, sel, nil, map[string]string{}, nil) {
		t.Fatal("unlabeled namespace should not match")
	}
}
