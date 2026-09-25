package admission

import (
	"context"
	"fmt"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
)

func (h *Handler) runVAP(ctx context.Context, req admit.Request) error {
	policies, err := h.store.policies(ctx)
	if err != nil {
		return err
	}
	bindings, err := h.store.bindings(ctx)
	if err != nil {
		return err
	}
	if len(policies) == 0 || len(bindings) == 0 {
		return nil
	}
	byName := map[string]admissionregv1.ValidatingAdmissionPolicy{}
	for _, p := range policies {
		byName[p.Name] = p
	}
	vars := celVars(req)
	if req.Namespace != "" {
		if ns, ok, err := h.store.namespace(ctx, req.Namespace); err == nil && ok {
			vars["namespaceObject"] = map[string]any{
				"metadata": map[string]any{"name": ns.Name, "namespace": ns.Namespace, "labels": ns.Labels, "annotations": ns.Annotations},
			}
		}
	}
	for _, b := range bindings {
		policy, ok := byName[b.Spec.PolicyName]
		if !ok {
			continue
		}
		nsLabels := h.namespaceLabels(ctx, req.Namespace)
		if policy.Spec.MatchConstraints == nil || !matchVAP(req, policy.Spec.MatchConstraints, nsLabels) {
			continue
		}
		if b.Spec.MatchResources != nil && !matchVAP(req, b.Spec.MatchResources, nsLabels) {
			continue
		}
		actions := b.Spec.ValidationActions
		if len(actions) == 0 {
			actions = []admissionregv1.ValidationAction{admissionregv1.Deny}
		}
		vars["variables"] = evalVariables(policy.Spec.Variables, vars)
		for i, v := range policy.Spec.Validations {
			ok, err := evalBool(v.Expression, vars)
			if err != nil {
				if denyAction(actions) {
					return invalidError{msg: fmt.Sprintf("ValidatingAdmissionPolicy %s validation[%d]: %s", policy.Name, i, err.Error())}
				}
				continue
			}
			if ok {
				continue
			}
			if !denyAction(actions) {
				continue
			}
			msg := v.Message
			if v.MessageExpression != "" {
				if s, err := evalString(v.MessageExpression, vars); err == nil && s != "" {
					msg = s
				}
			}
			if msg == "" {
				msg = fmt.Sprintf("ValidatingAdmissionPolicy %s denied the request", policy.Name)
			}
			return invalidError{msg: msg}
		}
	}
	return nil
}

func matchVAP(req admit.Request, m *admissionregv1.MatchResources, nsLabels map[string]string) bool {
	if m == nil {
		return true
	}
	rules := make([]admissionregv1.RuleWithOperations, 0, len(m.ResourceRules))
	for _, r := range m.ResourceRules {
		rules = append(rules, admissionregv1.RuleWithOperations{Operations: r.Operations, Rule: r.Rule})
	}
	if len(rules) > 0 && !matchRules(req, rules, m.MatchPolicy) {
		return false
	}
	if req.Kind.Kind == "Namespace" {
		nsLabels = objectLabels(req.Object, req.OldObject)
	}
	if !matchLabelSelector(m.NamespaceSelector, nsLabels) {
		return false
	}
	if !matchLabelSelector(m.ObjectSelector, objectLabels(req.Object, req.OldObject)) {
		return false
	}
	return true
}

type invalidError struct{ msg string }

func (e invalidError) Error() string { return e.msg }

func denyAction(actions []admissionregv1.ValidationAction) bool {
	for _, a := range actions {
		if a == admissionregv1.Deny {
			return true
		}
	}
	return false
}
