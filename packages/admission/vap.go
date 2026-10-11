package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/plugin/policy/generic"
	"k8s.io/apiserver/pkg/admission/plugin/policy/validating"
)

var validatingPolicyPlugin = func() *validating.Plugin {
	p, err := validating.NewPlugin(nil)
	if err != nil {
		panic(err)
	}
	return p
}()

var validatingHooks = policyHookSet[*validating.Policy, *validating.PolicyBinding, validating.PolicyEvaluator]{
	newPolicyAccessor:  validating.NewValidatingAdmissionPolicyAccessor,
	newBindingAccessor: validating.NewValidatingAdmissionPolicyBindingAccessor,
	compile:            validating.CompilePolicy,
}

func (h *Handler) runVAP(ctx context.Context, req admit.Request) error {
	if !validatingPolicyPlugin.Handles(admission.Operation(req.Operation)) || policyExcluded(req) {
		return nil
	}
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
	f := newStoreInformerFactory(ctx, h.store)
	matcher := generic.NewPolicyMatcher(h.policyMatcher(f))
	attrs := recordedAttributes(ctx, policyAttributes(req))
	hooks := validatingHooks.hooks(ctx, h, attrs, matcher, pointersTo(policies), pointersTo(bindings))
	err = validating.NewDispatcher(h.authz, matcher).Dispatch(ctx, attrs, policyObjectInterfaces, hooks)
	if f.failure != nil {
		return apierrors.NewInternalError(f.failure)
	}
	return err
}
