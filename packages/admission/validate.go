package admission

import (
	"encoding/json"
	"fmt"
	"net/http"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/admissionregistration"
	_ "k8s.io/kubernetes/pkg/apis/admissionregistration/install"
	"k8s.io/kubernetes/pkg/apis/admissionregistration/validation"
)

type registrationValidator struct {
	kind   string
	create func(obj runtime.Object) field.ErrorList
	update func(obj, old runtime.Object) field.ErrorList
	status func(obj, old runtime.Object) field.ErrorList
}

var registrationValidators = map[string]registrationValidator{
	"validatingwebhookconfigurations": {
		kind: "ValidatingWebhookConfiguration",
		create: func(obj runtime.Object) field.ErrorList {
			return validation.ValidateValidatingWebhookConfiguration(obj.(*admissionregistration.ValidatingWebhookConfiguration))
		},
		update: func(obj, old runtime.Object) field.ErrorList {
			return validation.ValidateValidatingWebhookConfigurationUpdate(obj.(*admissionregistration.ValidatingWebhookConfiguration), old.(*admissionregistration.ValidatingWebhookConfiguration))
		},
	},
	"mutatingwebhookconfigurations": {
		kind: "MutatingWebhookConfiguration",
		create: func(obj runtime.Object) field.ErrorList {
			return validation.ValidateMutatingWebhookConfiguration(obj.(*admissionregistration.MutatingWebhookConfiguration))
		},
		update: func(obj, old runtime.Object) field.ErrorList {
			return validation.ValidateMutatingWebhookConfigurationUpdate(obj.(*admissionregistration.MutatingWebhookConfiguration), old.(*admissionregistration.MutatingWebhookConfiguration))
		},
	},
	"validatingadmissionpolicies": {
		kind: "ValidatingAdmissionPolicy",
		create: func(obj runtime.Object) field.ErrorList {
			return validation.ValidateValidatingAdmissionPolicy(obj.(*admissionregistration.ValidatingAdmissionPolicy))
		},
		update: func(obj, old runtime.Object) field.ErrorList {
			return validation.ValidateValidatingAdmissionPolicyUpdate(obj.(*admissionregistration.ValidatingAdmissionPolicy), old.(*admissionregistration.ValidatingAdmissionPolicy))
		},
		status: func(obj, old runtime.Object) field.ErrorList {
			return validation.ValidateValidatingAdmissionPolicyStatusUpdate(obj.(*admissionregistration.ValidatingAdmissionPolicy), old.(*admissionregistration.ValidatingAdmissionPolicy))
		},
	},
	"validatingadmissionpolicybindings": {
		kind: "ValidatingAdmissionPolicyBinding",
		create: func(obj runtime.Object) field.ErrorList {
			return validation.ValidateValidatingAdmissionPolicyBinding(obj.(*admissionregistration.ValidatingAdmissionPolicyBinding))
		},
		update: func(obj, old runtime.Object) field.ErrorList {
			return validation.ValidateValidatingAdmissionPolicyBindingUpdate(obj.(*admissionregistration.ValidatingAdmissionPolicyBinding), old.(*admissionregistration.ValidatingAdmissionPolicyBinding))
		},
	},
	"mutatingadmissionpolicies": {
		kind: "MutatingAdmissionPolicy",
		create: func(obj runtime.Object) field.ErrorList {
			return validation.ValidateMutatingAdmissionPolicy(obj.(*admissionregistration.MutatingAdmissionPolicy))
		},
		update: func(obj, old runtime.Object) field.ErrorList {
			return validation.ValidateMutatingAdmissionPolicyUpdate(obj.(*admissionregistration.MutatingAdmissionPolicy), old.(*admissionregistration.MutatingAdmissionPolicy))
		},
	},
	"mutatingadmissionpolicybindings": {
		kind: "MutatingAdmissionPolicyBinding",
		create: func(obj runtime.Object) field.ErrorList {
			return validation.ValidateMutatingAdmissionPolicyBinding(obj.(*admissionregistration.MutatingAdmissionPolicyBinding))
		},
		update: func(obj, old runtime.Object) field.ErrorList {
			return validation.ValidateMutatingAdmissionPolicyBindingUpdate(obj.(*admissionregistration.MutatingAdmissionPolicyBinding), old.(*admissionregistration.MutatingAdmissionPolicyBinding))
		},
	},
}

func validateRegistration(req admit.ValidateRequest) (field.ErrorList, error) {
	v, ok := registrationValidators[req.Resource]
	if !ok {
		return nil, fmt.Errorf("no validation for %q", req.Resource)
	}
	gvk := schema.GroupVersionKind{Group: admissionregistration.GroupName, Version: "v1", Kind: v.kind}
	obj, err := decodeRequestObject(req.Object, gvk)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("no object to validate")
	}
	if !req.Update {
		return v.create(obj), nil
	}
	old, err := decodeRequestObject(req.OldObject, gvk)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, fmt.Errorf("no old object to validate against")
	}
	if req.Subresource == "status" {
		if v.status == nil {
			return nil, fmt.Errorf("%s has no status validation", req.Resource)
		}
		return v.status(obj, old), nil
	}
	return v.update(obj, old), nil
}

func (h *Handler) validate(w http.ResponseWriter, r *http.Request) {
	var req admit.ValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	errs, err := validateRegistration(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(admit.ValidateResponse{Errors: errs})
}
