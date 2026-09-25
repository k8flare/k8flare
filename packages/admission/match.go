package admission

import (
	"fmt"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func exemptAdmissionConfig(req admit.Request) bool {
	if req.Kind.Group != "admissionregistration.k8s.io" {
		return false
	}
	switch req.Kind.Kind {
	case "ValidatingWebhookConfiguration", "MutatingWebhookConfiguration",
		"ValidatingAdmissionPolicy", "ValidatingAdmissionPolicyBinding",
		"MutatingAdmissionPolicy", "MutatingAdmissionPolicyBinding":
		return true
	}
	return false
}

func matchWebhook(req admit.Request, rules []admissionregv1.RuleWithOperations, nsSelector, objSelector *metav1.LabelSelector, nsLabels map[string]string, matchPolicy *admissionregv1.MatchPolicyType) bool {
	if !matchRules(req, rules, matchPolicy) {
		return false
	}
	if req.Kind.Kind != "Namespace" && req.Namespace == "" {
		return matchLabelSelector(objSelector, objectLabels(req.Object, req.OldObject))
	}
	if req.Kind.Kind == "Namespace" {
		nsLabels = objectLabels(req.Object, req.OldObject)
	}
	if !matchLabelSelector(nsSelector, nsLabels) {
		return false
	}
	if !matchLabelSelector(objSelector, objectLabels(req.Object, req.OldObject)) {
		return false
	}
	return true
}

func matchRules(req admit.Request, rules []admissionregv1.RuleWithOperations, _ *admissionregv1.MatchPolicyType) bool {
	for _, rule := range rules {
		if !matchOperation(req.Operation, rule.Operations) {
			continue
		}
		if !matchOne(req.Resource.Group, rule.APIGroups) {
			continue
		}
		if !matchOne(req.Resource.Version, rule.APIVersions) {
			continue
		}
		if !matchResource(req.Resource.Resource, req.Subresource, rule.Resources) {
			continue
		}
		if !matchScope(req, rule.Scope) {
			continue
		}
		return true
	}
	return false
}

func matchOperation(op string, ops []admissionregv1.OperationType) bool {
	for _, candidate := range ops {
		if candidate == admissionregv1.OperationAll || string(candidate) == op {
			return true
		}
	}
	return false
}

func matchOne(got string, want []string) bool {
	for _, w := range want {
		if w == "*" || w == got {
			return true
		}
	}
	return false
}

func matchResource(resource, sub string, resources []string) bool {
	full := resource
	if sub != "" {
		full = resource + "/" + sub
	}
	for _, r := range resources {
		switch {
		case r == "*":
			if sub == "" {
				return true
			}
		case r == "*/*":
			return true
		case r == full:
			return true
		case sub != "" && r == "*/"+sub:
			return true
		case sub == "" && r == resource:
			return true
		}
	}
	return false
}

func matchScope(req admit.Request, scope *admissionregv1.ScopeType) bool {
	if scope == nil || *scope == admissionregv1.AllScopes {
		return true
	}
	namespaced := req.Namespace != ""
	if *scope == admissionregv1.NamespacedScope {
		return namespaced
	}
	return !namespaced
}

func matchLabelSelector(sel *metav1.LabelSelector, lbls map[string]string) bool {
	if sel == nil {
		return true
	}
	parsed, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return false
	}
	if lbls == nil {
		lbls = map[string]string{}
	}
	return parsed.Matches(labels.Set(lbls))
}

func objectLabels(obj, old map[string]any) map[string]string {
	src := obj
	if src == nil {
		src = old
	}
	if src == nil {
		return nil
	}
	meta, _ := src["metadata"].(map[string]any)
	if meta == nil {
		return nil
	}
	return stringMap(meta["labels"])
}

func stringMap(v any) map[string]string {
	raw, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for k, val := range raw {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out
}

func matchConditions(req admit.Request, conds []admissionregv1.MatchCondition) (bool, error) {
	if len(conds) == 0 {
		return true, nil
	}
	vars := celVars(req)
	for _, cond := range conds {
		ok, err := evalBool(cond.Expression, vars)
		if err != nil {
			return false, fmt.Errorf("matchConditions[%s]: %w", cond.Name, err)
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
