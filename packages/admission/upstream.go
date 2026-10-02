package admission

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	if gvk.Kind == "CertificateSigningRequest" {
		if spec, ok := m["spec"].(map[string]any); ok {
			if raw, ok := spec["request"].(string); ok {
				if _, err := base64.StdEncoding.DecodeString(raw); err != nil {
					mCopy := make(map[string]any, len(m))
					for k, v := range m {
						mCopy[k] = v
					}
					specCopy := make(map[string]any, len(spec))
					for k, v := range spec {
						specCopy[k] = v
					}
					specCopy["request"] = base64.StdEncoding.EncodeToString([]byte(raw))
					mCopy["spec"] = specCopy
					m = mCopy
				}
			}
		}
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

func requestAttributes(req *admit.Request, obj, oldObj runtime.Object) admission.Attributes {
	op := admission.Operation(req.Operation)
	if op == "" {
		op = admission.Create
	}
	var userInfo user.Info
	if req.User.Username != "" || req.User.UID != "" || len(req.User.Groups) > 0 || len(req.User.Extra) > 0 {
		userInfo = &user.DefaultInfo{
			Name:   req.User.Username,
			UID:    req.User.UID,
			Groups: req.User.Groups,
			Extra:  req.User.Extra,
		}
	} else {
		userInfo = &user.DefaultInfo{Name: "alice", Groups: []string{"system:authenticated"}}
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
		&metav1.CreateOptions{},
		req.DryRun,
		userInfo,
	)
}

func mapAdmissionError(err error) error {
	if err == nil {
		return nil
	}
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		s := status.Status()
		msg := s.Message
		if s.Reason == metav1.StatusReasonForbidden {
			if idx := strings.Index(msg, "is forbidden: "); idx != -1 {
				msg = msg[idx+len("is forbidden: "):]
			} else if strings.HasPrefix(msg, "forbidden: ") {
				msg = strings.TrimPrefix(msg, "forbidden: ")
			}
			return fmt.Errorf("%s", msg)
		}
		if s.Reason == metav1.StatusReasonInvalid {
			return invalidError{msg: msg}
		}
		if s.Reason == metav1.StatusReasonInternalError {
			return internalError{errors.New(msg)}
		}
		return fmt.Errorf("%s", msg)
	}
	return err
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
			return mapAdmissionError(err)
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
			return mapAdmissionError(err)
		}
		return nil

	default:
		return nil
	}
}
