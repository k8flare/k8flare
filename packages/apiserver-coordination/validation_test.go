package coordination

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "coordination.k8s.io", Version: "v1"}, metav1.APIResource{Name: "leases", Kind: "Lease", Namespaced: true})
	obj := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: "l", Namespace: "default"},
		Spec:       coordinationv1.LeaseSpec{LeaseDurationSeconds: ptr.To[int32](-1)},
	}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.leaseDurationSeconds", "must be greater than 0")
}
