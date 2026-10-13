package admissionregistration

import (
	"context"
	"net/http"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

var validatedResources = []string{
	"mutatingwebhookconfigurations",
	"validatingwebhookconfigurations",
	"validatingadmissionpolicies",
	"validatingadmissionpolicybindings",
	"mutatingadmissionpolicies",
	"mutatingadmissionpolicybindings",
}

func init() {
	for _, name := range validatedResources {
		resource := name
		registry.Customizers[resource] = func(store *registry.Store, deps registry.Deps) {
			store.CreateStrategy = validateRemotely{RESTCreateStrategy: store.CreateStrategy, resource: resource, client: deps.Admission}
			store.UpdateStrategy = validateUpdateRemotely{RESTUpdateStrategy: store.UpdateStrategy, resource: resource, client: deps.Admission}
			if resource == "validatingadmissionpolicies" {
				store.CreateStrategy = vapCreate{RESTCreateStrategy: store.CreateStrategy, kine: deps.Kine}
				store.UpdateStrategy = vapUpdate{RESTUpdateStrategy: store.UpdateStrategy, kine: deps.Kine}
			}
		}
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

type validateRemotely struct {
	rest.RESTCreateStrategy
	resource string
	client   *http.Client
}

type validateUpdateRemotely struct {
	rest.RESTUpdateStrategy
	resource string
	client   *http.Client
}

func (s validateRemotely) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	return validateOnAdmissionWorker(ctx, s.client, admit.ValidateRequest{Resource: s.resource}, obj, nil)
}

func (s validateUpdateRemotely) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	req := admit.ValidateRequest{Resource: s.resource, Update: true}
	if info, ok := genericapirequest.RequestInfoFrom(ctx); ok {
		req.Subresource = info.Subresource
	}
	return validateOnAdmissionWorker(ctx, s.client, req, obj, old)
}

func validateOnAdmissionWorker(ctx context.Context, client *http.Client, req admit.ValidateRequest, obj, old runtime.Object) field.ErrorList {
	if client == nil {
		return nil
	}
	var err error
	if req.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(obj); err != nil {
		return field.ErrorList{field.InternalError(nil, err)}
	}
	if old != nil {
		if req.OldObject, err = runtime.DefaultUnstructuredConverter.ToUnstructured(old); err != nil {
			return field.ErrorList{field.InternalError(nil, err)}
		}
	}
	return admit.Validate(ctx, client, req)
}
