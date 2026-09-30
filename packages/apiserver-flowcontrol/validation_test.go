package flowcontrol

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "flowcontrol.apiserver.k8s.io", Version: "v1"}, metav1.APIResource{Name: "flowschemas", Kind: "FlowSchema"})
	obj := &flowcontrolv1.FlowSchema{ObjectMeta: metav1.ObjectMeta{Name: "f"}}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.priorityLevelConfiguration.name", "Required value")
}
