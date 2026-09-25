package core

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	corevalidation "k8s.io/kubernetes/pkg/apis/core/validation"
)

func (s podCreateStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	var errs field.ErrorList
	if s.RESTCreateStrategy != nil {
		errs = s.RESTCreateStrategy.Validate(ctx, obj)
	}
	return append(errs, validatePodSysctls(obj.(*corev1.Pod))...)
}

func validatePodSysctls(pod *corev1.Pod) field.ErrorList {
	if pod.Spec.SecurityContext == nil {
		return nil
	}
	var errs field.ErrorList
	fld := field.NewPath("spec", "securityContext", "sysctls")
	seen := map[string]struct{}{}
	for i, s := range pod.Spec.SecurityContext.Sysctls {
		if s.Name == "" {
			errs = append(errs, field.Required(fld.Index(i).Child("name"), ""))
			continue
		}
		if !corevalidation.IsValidSysctlName(s.Name) {
			errs = append(errs, field.Invalid(fld.Index(i).Child("name"), s.Name, "must have at most 253 characters and match regex [a-z0-9]([-_a-z0-9]*[a-z0-9])?([\\./][a-z0-9]([-_a-z0-9]*[a-z0-9])?)*"))
		}
		if _, ok := seen[s.Name]; ok {
			errs = append(errs, field.Duplicate(fld.Index(i).Child("name"), s.Name))
		}
		seen[s.Name] = struct{}{}
	}
	return errs
}
