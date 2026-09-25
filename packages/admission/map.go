package admission

import (
	"context"
	"fmt"

	celgo "github.com/google/cel-go/cel"
	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/apiserver/pkg/admission"
	plugincel "k8s.io/apiserver/pkg/admission/plugin/cel"
	"k8s.io/apiserver/pkg/admission/plugin/policy/mutating/patch"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/cel/environment"
	"k8s.io/client-go/kubernetes/scheme"
)

func (h *Handler) runMAP(ctx context.Context, req admit.Request) (map[string]any, error) {
	if exemptAdmissionConfig(req) || req.Object == nil {
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
	byName := map[string]admissionregv1.MutatingAdmissionPolicy{}
	for _, p := range policies {
		byName[p.Name] = p
	}
	obj := req.Object
	nsLabels := h.namespaceLabels(ctx, req.Namespace)
	var ns *corev1.Namespace
	if stored, ok, err := h.store.namespace(ctx, req.Namespace); err == nil && ok {
		ns = &stored
	}
	for _, b := range bindings {
		policy, ok := byName[b.Spec.PolicyName]
		if !ok {
			continue
		}
		if policy.Spec.MatchConstraints == nil || !matchVAP(req, policy.Spec.MatchConstraints, nsLabels) {
			continue
		}
		if b.Spec.MatchResources != nil && !matchVAP(req, b.Spec.MatchResources, nsLabels) {
			continue
		}
		ok, err := matchConditions(req, policy.Spec.MatchConditions)
		if err != nil {
			if ignoreFailure(policy.Spec.FailurePolicy) {
				continue
			}
			return nil, err
		}
		if !ok {
			continue
		}
		next, err := applyMAP(ctx, req, obj, policy, ns)
		if err != nil {
			if ignoreFailure(policy.Spec.FailurePolicy) {
				continue
			}
			return nil, err
		}
		obj = next
		req.Object = obj
	}
	return obj, nil
}

func applyMAP(ctx context.Context, req admit.Request, current map[string]any, policy admissionregv1.MutatingAdmissionPolicy, ns *corev1.Namespace) (map[string]any, error) {
	patchers, err := compileMAP(policy)
	if err != nil {
		return nil, err
	}
	obj := unstructuredFrom(req, current)
	var old runtime.Object
	if req.OldObject != nil {
		old = unstructuredFrom(req, req.OldObject)
	}
	attrs := admission.NewAttributesRecord(
		obj, old, req.Kind, req.Namespace, req.Name, req.Resource, req.Subresource,
		admission.Operation(req.Operation), nil, req.DryRun,
		&user.DefaultInfo{Name: req.User.Username, UID: req.User.UID, Groups: req.User.Groups, Extra: req.User.Extra},
	)
	versioned := &admission.VersionedAttributes{
		Attributes:         attrs,
		VersionedOldObject: old,
		VersionedObject:    obj,
		VersionedKind:      req.Kind,
	}
	patchReq := patch.Request{
		MatchedResource:     req.Resource,
		VersionedAttributes: versioned,
		ObjectInterfaces:    admission.NewObjectInterfacesFromScheme(scheme.Scheme),
		Namespace:           ns,
		TypeConverter:       managedfields.NewDeducedTypeConverter(),
	}
	patched := runtime.Object(obj)
	for _, p := range patchers {
		next, err := p.Patch(ctx, patchReq, celconfig.RuntimeCELCostBudget)
		if err != nil {
			return nil, fmt.Errorf("MutatingAdmissionPolicy %s: %w", policy.Name, err)
		}
		patched = next
		versioned.VersionedObject = next
		versioned.Dirty = true
	}
	return objectMap(patched)
}

func compileMAP(policy admissionregv1.MutatingAdmissionPolicy) ([]patch.Patcher, error) {
	opts := plugincel.OptionalVariableDeclarations{HasParams: policy.Spec.ParamKind != nil, HasAuthorizer: true, HasPatchTypes: true}
	compiler, err := plugincel.NewCompositedCompiler(environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion()))
	if err != nil {
		return nil, err
	}
	if len(policy.Spec.Variables) > 0 {
		vars := make([]plugincel.NamedExpressionAccessor, len(policy.Spec.Variables))
		for i, v := range policy.Spec.Variables {
			vars[i] = mapVariable{name: v.Name, expr: v.Expression}
		}
		compiler.CompileAndStoreVariables(vars, plugincel.OptionalVariableDeclarations{HasParams: policy.Spec.ParamKind != nil, HasAuthorizer: true}, environment.StoredExpressions)
	}
	var patchers []patch.Patcher
	for _, m := range policy.Spec.Mutations {
		switch m.PatchType {
		case admissionregv1.PatchTypeJSONPatch:
			if m.JSONPatch == nil {
				continue
			}
			compiled := compiler.CompileMutatingEvaluator(&patch.JSONPatchCondition{Expression: m.JSONPatch.Expression}, opts, environment.StoredExpressions)
			if errs := compiled.CompilationErrors(); len(errs) > 0 {
				return nil, fmt.Errorf("MutatingAdmissionPolicy %s: %v", policy.Name, errs)
			}
			patchers = append(patchers, patch.NewJSONPatcher(compiled))
		case admissionregv1.PatchTypeApplyConfiguration:
			if m.ApplyConfiguration == nil {
				continue
			}
			compiled := compiler.CompileMutatingEvaluator(&patch.ApplyConfigurationCondition{Expression: m.ApplyConfiguration.Expression}, opts, environment.StoredExpressions)
			if errs := compiled.CompilationErrors(); len(errs) > 0 {
				return nil, fmt.Errorf("MutatingAdmissionPolicy %s: %v", policy.Name, errs)
			}
			patchers = append(patchers, patch.NewApplyConfigurationPatcher(compiled))
		}
	}
	return patchers, nil
}

func unstructuredFrom(req admit.Request, obj map[string]any) *unstructured.Unstructured {
	copied := runtime.DeepCopyJSON(obj)
	u := &unstructured.Unstructured{Object: copied}
	if u.GetAPIVersion() == "" {
		u.SetGroupVersionKind(req.Kind)
	}
	if u.GetKind() == "" {
		u.SetKind(req.Kind.Kind)
	}
	if u.GroupVersionKind().Empty() {
		u.SetGroupVersionKind(schema.GroupVersionKind{Group: req.Kind.Group, Version: req.Kind.Version, Kind: req.Kind.Kind})
	}
	return u
}

type mapVariable struct {
	name string
	expr string
}

func (v mapVariable) GetName() string                 { return v.name }
func (v mapVariable) GetExpression() string           { return v.expr }
func (v mapVariable) ReturnTypes() []*celgo.Type      { return []*celgo.Type{celgo.AnyType, celgo.DynType} }

func objectMap(obj runtime.Object) (map[string]any, error) {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return u.Object, nil
	}
	return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
}
