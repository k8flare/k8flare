package policy

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "policy", Version: "v1"}, metav1.APIResource{Name: "poddisruptionbudgets", Kind: "PodDisruptionBudget", Namespaced: true})
	min, max := intstr.FromInt32(1), intstr.FromInt32(1)
	obj := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable:   &min,
			MaxUnavailable: &max,
			Selector:       &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
		},
	}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec", "minAvailable and maxUnavailable cannot be both set")
}
