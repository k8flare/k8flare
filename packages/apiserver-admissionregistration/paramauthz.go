package admissionregistration

import (
	"context"
	"fmt"
	"strings"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
	rbacregistry "k8s.io/kubernetes/pkg/registry/rbac"
)

type paramAuthorizer struct {
	authorizer authorizer.Authorizer
	kine       *kine.Client
	policies   storage.Interface
	mutating   storage.Interface
}

func newParamAuthorizer(a authorizer.Authorizer, client *kine.Client) *paramAuthorizer {
	codec := scheme.Codecs.LegacyCodec(admissionregv1.SchemeGroupVersion)
	return &paramAuthorizer{
		authorizer: a,
		kine:       client,
		policies:   kine.NewStorage(client, codec, func() runtime.Object { return &admissionregv1.ValidatingAdmissionPolicy{} }),
		mutating:   kine.NewStorage(client, codec, func() runtime.Object { return &admissionregv1.MutatingAdmissionPolicy{} }),
	}
}

func (p *paramAuthorizer) authorize(ctx context.Context, obj, old runtime.Object) error {
	switch o := obj.(type) {
	case *admissionregv1.ValidatingAdmissionPolicy:
		var oldKind *admissionregv1.ParamKind
		if oldPolicy, ok := old.(*admissionregv1.ValidatingAdmissionPolicy); ok {
			oldKind = oldPolicy.Spec.ParamKind
		}
		return p.authorizeParamKind(ctx, o.Spec.ParamKind, oldKind)
	case *admissionregv1.MutatingAdmissionPolicy:
		var oldKind *admissionregv1.ParamKind
		if oldPolicy, ok := old.(*admissionregv1.MutatingAdmissionPolicy); ok {
			oldKind = oldPolicy.Spec.ParamKind
		}
		return p.authorizeParamKind(ctx, o.Spec.ParamKind, oldKind)
	case *admissionregv1.ValidatingAdmissionPolicyBinding:
		var oldSpec *admissionregv1.ValidatingAdmissionPolicyBindingSpec
		if oldBinding, ok := old.(*admissionregv1.ValidatingAdmissionPolicyBinding); ok {
			oldSpec = &oldBinding.Spec
		}
		return p.authorizeParamRef(ctx, o.Spec.ParamRef, o.Spec.PolicyName, oldSpec != nil && sameParamRef(oldSpec.ParamRef, o.Spec.ParamRef, oldSpec.PolicyName, o.Spec.PolicyName), p.validatingParamKind)
	case *admissionregv1.MutatingAdmissionPolicyBinding:
		var oldSpec *admissionregv1.MutatingAdmissionPolicyBindingSpec
		if oldBinding, ok := old.(*admissionregv1.MutatingAdmissionPolicyBinding); ok {
			oldSpec = &oldBinding.Spec
		}
		return p.authorizeParamRef(ctx, o.Spec.ParamRef, o.Spec.PolicyName, oldSpec != nil && sameParamRef(oldSpec.ParamRef, o.Spec.ParamRef, oldSpec.PolicyName, o.Spec.PolicyName), p.mutatingParamKind)
	}
	return nil
}

func sameParamRef(old, current *admissionregv1.ParamRef, oldPolicy, currentPolicy string) bool {
	return old != nil && current != nil && *old == *current && oldPolicy == currentPolicy
}

func (p *paramAuthorizer) validatingParamKind(ctx context.Context, name string) (*admissionregv1.ParamKind, error) {
	policy := &admissionregv1.ValidatingAdmissionPolicy{}
	if err := p.policies.Get(ctx, "/validatingadmissionpolicies/"+name, storage.GetOptions{}, policy); err != nil {
		return nil, err
	}
	return policy.Spec.ParamKind, nil
}

func (p *paramAuthorizer) mutatingParamKind(ctx context.Context, name string) (*admissionregv1.ParamKind, error) {
	policy := &admissionregv1.MutatingAdmissionPolicy{}
	if err := p.mutating.Get(ctx, "/mutatingadmissionpolicies/"+name, storage.GetOptions{}, policy); err != nil {
		return nil, err
	}
	return policy.Spec.ParamKind, nil
}

func (p *paramAuthorizer) authorizeParamKind(ctx context.Context, paramKind, oldParamKind *admissionregv1.ParamKind) error {
	if paramKind == nil {
		return nil
	}
	if oldParamKind != nil && *oldParamKind == *paramKind {
		return nil
	}
	if p.authorizer == nil || rbacregistry.EscalationAllowed(ctx) {
		return nil
	}
	user, ok := genericapirequest.UserFrom(ctx)
	if !ok {
		return fmt.Errorf("cannot identify user to authorize read access to paramKind resources")
	}
	resource, apiGroup, apiVersion := "*", "*", "*"
	if gv, err := schema.ParseGroupVersion(paramKind.APIVersion); err == nil {
		apiGroup = gv.Group
		apiVersion = gv.Version
		if gvr, err := p.resolve(ctx, gv.WithKind(paramKind.Kind)); err == nil {
			resource = gvr.Resource
		}
	}
	d, _, err := p.authorizer.Authorize(ctx, authorizer.AttributesRecord{
		User:            user,
		Verb:            "get",
		ResourceRequest: true,
		Name:            "*",
		Namespace:       "*",
		APIGroup:        apiGroup,
		APIVersion:      apiVersion,
		Resource:        resource,
	})
	if err != nil {
		return err
	}
	if d != authorizer.DecisionAllow {
		return fmt.Errorf(`user %v must have "get" permission on all objects of the referenced paramKind (kind=%s, apiVersion=%s)`, user, paramKind.Kind, paramKind.APIVersion)
	}
	return nil
}

func (p *paramAuthorizer) authorizeParamRef(ctx context.Context, paramRef *admissionregv1.ParamRef, policyName string, unchanged bool, paramKindOf func(context.Context, string) (*admissionregv1.ParamKind, error)) error {
	if paramRef == nil || unchanged {
		return nil
	}
	if p.authorizer == nil || rbacregistry.EscalationAllowed(ctx) {
		return nil
	}
	user, ok := genericapirequest.UserFrom(ctx)
	if !ok {
		return fmt.Errorf("cannot identify user to authorize read access to paramRef object")
	}
	resource, apiGroup, apiVersion := "*", "*", "*"
	var policyErr, gvParseErr, gvrResolveErr error
	paramKind, policyErr := paramKindOf(ctx, policyName)
	if policyErr == nil && paramKind != nil {
		var gv schema.GroupVersion
		gv, gvParseErr = schema.ParseGroupVersion(paramKind.APIVersion)
		if gvParseErr == nil {
			apiGroup = gv.Group
			apiVersion = gv.Version
			var gvr schema.GroupVersionResource
			gvr, gvrResolveErr = p.resolve(ctx, gv.WithKind(paramKind.Kind))
			if gvrResolveErr == nil {
				resource = gvr.Resource
			}
		}
	}
	verb := "get"
	if len(paramRef.Name) == 0 {
		verb = "list"
	}
	d, _, err := p.authorizer.Authorize(ctx, authorizer.AttributesRecord{
		User:            user,
		Verb:            verb,
		ResourceRequest: true,
		Name:            paramRef.Name,
		Namespace:       paramRef.Namespace,
		APIGroup:        apiGroup,
		APIVersion:      apiVersion,
		Resource:        resource,
	})
	if err != nil {
		return fmt.Errorf(`failed to authorize request: %w`, err)
	}
	if d == authorizer.DecisionAllow {
		return nil
	}
	switch {
	case policyErr != nil:
		return fmt.Errorf(`unable to get policy %s to determine minimum required permissions and user %v does not have "%v" permission for all groups, versions and resources`, policyName, user, verb)
	case gvParseErr != nil:
		return fmt.Errorf(`unable to parse paramKind %v to determine minimum required permissions and user %v does not have "%v" permission for all groups, versions and resources`, paramKind, user, verb)
	case gvrResolveErr != nil:
		return fmt.Errorf(`unable to resolve paramKind %v to determine minimum required permissions and user %v does not have "%v" permission for all groups, versions and resources`, paramKind, user, verb)
	}
	return fmt.Errorf(`user %v does not have "%v" permission on the object referenced by paramRef`, user, verb)
}

func (p *paramAuthorizer) resolve(ctx context.Context, gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	for _, served := range registry.Served {
		if served.GV != gvk.GroupVersion() {
			continue
		}
		for _, res := range served.Resources {
			if res.Kind == gvk.Kind && !strings.Contains(res.Name, "/") {
				return gvk.GroupVersion().WithResource(res.Name), nil
			}
		}
	}
	crds, err := listCRDs(ctx, p.kine)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	for _, crd := range crds {
		if crd.Spec.Group != gvk.Group || crd.Spec.Names.Kind != gvk.Kind {
			continue
		}
		for _, v := range crd.Spec.Versions {
			if v.Name == gvk.Version && v.Served {
				return gvk.GroupVersion().WithResource(crd.Spec.Names.Plural), nil
			}
		}
	}
	return schema.GroupVersionResource{}, &meta.NoKindMatchError{GroupKind: gvk.GroupKind(), SearchedVersions: []string{gvk.Version}}
}
