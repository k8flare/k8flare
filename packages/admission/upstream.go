package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	_ "k8s.io/kubernetes/pkg/apis/core/install"
)

var legacyObjectInterfaces = admission.NewObjectInterfacesFromScheme(legacyscheme.Scheme)

func toInternalObject(obj runtime.Object) (runtime.Object, error) {
	if obj == nil {
		return nil, nil
	}
	gvks, _, err := legacyscheme.Scheme.ObjectKinds(obj)
	if err != nil {
		return nil, err
	}
	return legacyscheme.Scheme.ConvertToVersion(obj, schema.GroupVersion{Group: gvks[0].Group, Version: runtime.APIVersionInternal})
}

func toVersionedObject(obj runtime.Object, gv schema.GroupVersion) (runtime.Object, error) {
	if obj == nil {
		return nil, nil
	}
	out, err := legacyscheme.Scheme.ConvertToVersion(obj.DeepCopyObject(), gv)
	if err != nil {
		return nil, err
	}
	out.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{})
	return out, nil
}

func decodeRequestObject(m map[string]any, gvk schema.GroupVersionKind) (runtime.Object, error) {
	if m == nil {
		return nil, nil
	}
	if !legacyscheme.Scheme.Recognizes(gvk) {
		return nil, nil
	}
	versioned, err := legacyscheme.Scheme.New(gvk)
	if err != nil {
		return nil, err
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(m, versioned); err != nil {
		return nil, err
	}
	return toInternalObject(versioned)
}

func operationOptions(op admission.Operation, dryRun bool) runtime.Object {
	var dryRunList []string
	if dryRun {
		dryRunList = []string{metav1.DryRunAll}
	}
	switch op {
	case admission.Create:
		return &metav1.CreateOptions{DryRun: dryRunList}
	case admission.Update:
		return &metav1.UpdateOptions{DryRun: dryRunList}
	case admission.Delete:
		return &metav1.DeleteOptions{DryRun: dryRunList}
	case admission.Connect:
		return nil
	default:
		return nil
	}
}

func userInfoFromRequest(u admit.User) user.Info {
	return &user.DefaultInfo{
		Name:   u.Username,
		UID:    u.UID,
		Groups: u.Groups,
		Extra:  u.Extra,
	}
}

func requestAttributes(req *admit.Request, obj, oldObj runtime.Object) admission.Attributes {
	op := admission.Operation(req.Operation)
	if op == "" {
		op = admission.Create
	}
	return admission.NewAttributesRecord(
		obj,
		oldObj,
		req.Kind,
		req.Namespace,
		req.Name,
		req.Resource,
		req.Subresource,
		op,
		operationOptions(op, req.DryRun),
		req.DryRun,
		userInfoFromRequest(req.User),
	)
}

func runUpstreamPlugin(ctx context.Context, plugin admission.Interface, req *admit.Request) error {
	op := admission.Operation(req.Operation)
	if op == "" {
		op = admission.Create
	}
	if !plugin.Handles(op) {
		return nil
	}

	obj, err := decodeRequestObject(req.Object, req.Kind)
	if err != nil {
		return err
	}
	oldObj, err := decodeRequestObject(req.OldObject, req.Kind)
	if err != nil {
		return err
	}

	attrs := requestAttributes(req, obj, oldObj)

	switch req.Phase {
	case "", "admit":
		mutator, ok := plugin.(admission.MutationInterface)
		if !ok {
			return nil
		}
		if err := mutator.Admit(ctx, attrs, legacyObjectInterfaces); err != nil {
			return err
		}
		if attrs.GetObject() != nil && req.Object != nil {
			versioned, err := toVersionedObject(attrs.GetObject(), req.Kind.GroupVersion())
			if err != nil {
				return err
			}
			m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(versioned)
			if err != nil {
				return err
			}
			req.Object = m
		}
		return nil

	case "validate":
		validator, ok := plugin.(admission.ValidationInterface)
		if !ok {
			return nil
		}
		if err := validator.Validate(ctx, attrs, legacyObjectInterfaces); err != nil {
			return err
		}
		return nil

	default:
		return nil
	}
}
