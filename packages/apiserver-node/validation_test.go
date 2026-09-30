package node

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	nodev1 "k8s.io/api/node/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "node.k8s.io", Version: "v1"}, metav1.APIResource{Name: "runtimeclasses", Kind: "RuntimeClass"})
	obj := &nodev1.RuntimeClass{ObjectMeta: metav1.ObjectMeta{Name: "r"}}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "handler", "Required value")
}
