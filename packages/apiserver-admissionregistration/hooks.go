package admissionregistration

import (
	"context"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

func init() {
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
