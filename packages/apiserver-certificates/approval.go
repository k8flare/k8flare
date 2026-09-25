package certificates

import (
	"context"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
	internalcert "k8s.io/kubernetes/pkg/apis/certificates"
	certvalidation "k8s.io/kubernetes/pkg/apis/certificates/validation"
)

func init() {
	utilruntime.Must(internalcert.AddToScheme(scheme.Scheme))
	registry.Subresources["certificatesigningrequests/approval"] = func(stores map[string]*registry.Store, _ registry.Deps) rest.Storage {
		parent := stores["certificatesigningrequests"]
		return registry.NewUpdateOnlyREST(parent, approvalStrategy{parent.UpdateStrategy})
	}
}

type approvalStrategy struct {
	rest.RESTUpdateStrategy
}

func (approvalStrategy) PrepareForUpdate(_ context.Context, obj, old runtime.Object) {
	newCSR, ok1 := obj.(*certificatesv1.CertificateSigningRequest)
	oldCSR, ok2 := old.(*certificatesv1.CertificateSigningRequest)
	if !ok1 || !ok2 {
		return
	}
	populateConditionTimestamps(newCSR, oldCSR)
	conditions := newCSR.Status.Conditions
	newCSR.Spec = oldCSR.Spec
	newCSR.Status = oldCSR.Status
	newCSR.Status.Conditions = conditions
}

func (approvalStrategy) ValidateUpdate(_ context.Context, obj, old runtime.Object) field.ErrorList {
	newInt, err := toInternalCSR(obj)
	if err != nil {
		return field.ErrorList{field.InternalError(nil, err)}
	}
	oldInt, err := toInternalCSR(old)
	if err != nil {
		return field.ErrorList{field.InternalError(nil, err)}
	}
	return certvalidation.ValidateCertificateSigningRequestApprovalUpdate(newInt, oldInt)
}

func toInternalCSR(obj runtime.Object) (*internalcert.CertificateSigningRequest, error) {
	if in, ok := obj.(*internalcert.CertificateSigningRequest); ok {
		return in, nil
	}
	out := &internalcert.CertificateSigningRequest{}
	if err := scheme.Scheme.Convert(obj, out, nil); err != nil {
		return nil, err
	}
	return out, nil
}

func populateConditionTimestamps(newCSR, oldCSR *certificatesv1.CertificateSigningRequest) {
	now := metav1.Now()
	for i := range newCSR.Status.Conditions {
		if newCSR.Status.Conditions[i].LastUpdateTime.IsZero() {
			newCSR.Status.Conditions[i].LastUpdateTime = now
		}
		if newCSR.Status.Conditions[i].LastTransitionTime.IsZero() {
			lastTransition := now
			for _, oldCondition := range oldCSR.Status.Conditions {
				if oldCondition.Type == newCSR.Status.Conditions[i].Type &&
					oldCondition.Status == newCSR.Status.Conditions[i].Status &&
					!oldCondition.LastTransitionTime.IsZero() {
					lastTransition = oldCondition.LastTransitionTime
					break
				}
			}
			newCSR.Status.Conditions[i].LastTransitionTime = lastTransition
		}
	}
}
