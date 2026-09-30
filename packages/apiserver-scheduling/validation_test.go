package scheduling

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "scheduling.k8s.io", Version: "v1"}, metav1.APIResource{Name: "priorityclasses", Kind: "PriorityClass"})
	obj := &schedulingv1.PriorityClass{ObjectMeta: metav1.ObjectMeta{Name: "p"}, Value: 2000000001}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "value", "maximum allowed value")
}
