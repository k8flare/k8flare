package admissionregistration

import (
	"context"
	"fmt"

	"github.com/google/cel-go/cel"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
)

func init() {
	for _, name := range []string{"mutatingwebhookconfigurations", "validatingwebhookconfigurations"} {
		res := name
		registry.Customizers[res] = func(store *registry.Store, _ registry.Deps) {
			store.CreateStrategy = createCEL{store.CreateStrategy}
			store.UpdateStrategy = updateCEL{store.UpdateStrategy}
		}
	}
	registry.Customizers["validatingadmissionpolicies"] = func(store *registry.Store, deps registry.Deps) {
		store.CreateStrategy = vapCreate{RESTCreateStrategy: store.CreateStrategy, kine: deps.Kine}
		store.UpdateStrategy = vapUpdate{RESTUpdateStrategy: store.UpdateStrategy, kine: deps.Kine}
	}
}

type vapCreate struct {
	rest.RESTCreateStrategy
	kine *kine.Client
}

type vapUpdate struct {
	rest.RESTUpdateStrategy
	kine *kine.Client
}

func (s vapCreate) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	if s.RESTCreateStrategy != nil {
		s.RESTCreateStrategy.PrepareForCreate(ctx, obj)
	}
	if p, ok := obj.(*admissionregv1.ValidatingAdmissionPolicy); ok {
		p.Status.TypeChecking = typeCheckPolicyWith(ctx, p, s.kine)
	}
}

func (s vapUpdate) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	if s.RESTUpdateStrategy != nil {
		s.RESTUpdateStrategy.PrepareForUpdate(ctx, obj, old)
	}
	if p, ok := obj.(*admissionregv1.ValidatingAdmissionPolicy); ok {
		p.Status.TypeChecking = typeCheckPolicyWith(ctx, p, s.kine)
	}
}

type createCEL struct{ rest.RESTCreateStrategy }

type updateCEL struct{ rest.RESTUpdateStrategy }

func (createCEL) Validate(_ context.Context, obj runtime.Object) field.ErrorList {
	return compileAdmissionCEL(obj)
}

func (updateCEL) ValidateUpdate(_ context.Context, obj, _ runtime.Object) field.ErrorList {
	return compileAdmissionCEL(obj)
}

func compileAdmissionCEL(obj runtime.Object) field.ErrorList {
	var errs field.ErrorList
	switch o := obj.(type) {
	case *admissionregv1.MutatingWebhookConfiguration:
		for i, hook := range o.Webhooks {
			errs = append(errs, compileMatchConditions(field.NewPath("webhooks").Index(i).Child("matchConditions"), hook.MatchConditions)...)
		}
	case *admissionregv1.ValidatingWebhookConfiguration:
		for i, hook := range o.Webhooks {
			errs = append(errs, compileMatchConditions(field.NewPath("webhooks").Index(i).Child("matchConditions"), hook.MatchConditions)...)
		}
	}
	return errs
}

func compileMatchConditions(path *field.Path, conds []admissionregv1.MatchCondition) field.ErrorList {
	var errs field.ErrorList
	for i, c := range conds {
		if err := compileCEL(c.Expression); err != nil {
			errs = append(errs, field.Invalid(path.Index(i).Child("expression"), c.Expression, err.Error()))
		}
	}
	return errs
}

func compileCEL(expr string) error {
	env, err := cel.NewEnv(
		cel.Variable("object", cel.DynType),
		cel.Variable("oldObject", cel.DynType),
		cel.Variable("request", cel.DynType),
		cel.Variable("namespaceObject", cel.DynType),
		cel.Variable("authorizer", cel.DynType),
		cel.Variable("params", cel.DynType),
	)
	if err != nil {
		return err
	}
	_, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return fmt.Errorf("compilation failed: %w", iss.Err())
	}
	return nil
}
