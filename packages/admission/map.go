package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/plugin/policy/generic"
	"k8s.io/apiserver/pkg/admission/plugin/policy/mutating"
)

var mutatingPolicyPlugin = func() *mutating.Plugin {
	p, err := mutating.NewPlugin(nil)
	if err != nil {
		panic(err)
	}
	return p
}()

var mutatingHooks = policyHookSet[*mutating.Policy, *mutating.PolicyBinding, mutating.PolicyEvaluator]{
	newPolicyAccessor:  mutating.NewMutatingAdmissionPolicyAccessor,
	newBindingAccessor: mutating.NewMutatingAdmissionPolicyBindingAccessor,
	compile:            mutating.CompilePolicy,
}

func (h *Handler) runMAP(ctx context.Context, req admit.Request) (map[string]any, error) {
	if !mutatingPolicyPlugin.Handles(admission.Operation(req.Operation)) || policyExcluded(req) {
		return req.Object, nil
	}
	policies, err := h.store.mutatingPolicies(ctx)
	if err != nil {
		return nil, err
	}
	bindings, err := h.store.mutatingBindings(ctx)
	if err != nil {
		return nil, err
	}
	if len(policies) == 0 || len(bindings) == 0 {
		return req.Object, nil
	}
	f := newStoreInformerFactory(ctx, h.store)
	matcher := h.policyMatcher(f)
	attrs := recordedAttributes(ctx, policyAttributes(req))
	hooks := mutatingHooks.hooks(ctx, h, attrs, generic.NewPolicyMatcher(matcher), pointersTo(policies), pointersTo(bindings))
	if len(hooks) == 0 {
		return req.Object, nil
	}
	h.refreshTypeConverters(ctx)
	err = mutating.NewDispatcher(h.authz, matcher, h.typeConverters).Dispatch(ctx, attrs, policyObjectInterfaces, hooks)
	if f.failure != nil {
		return nil, apierrors.NewInternalError(f.failure)
	}
	if err != nil {
		return nil, err
	}
	return objectMap(attrs.GetObject()), nil
}

func (h *Handler) refreshTypeConverters(ctx context.Context) {
	done, cancel := context.WithCancel(ctx)
	cancel()
	h.typeConverters.Run(done)
}
