package resource

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "resource.k8s.io", Version: "v1"}, metav1.APIResource{Name: "deviceclasses", Kind: "DeviceClass"})
	obj := &resourcev1.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "d"},
		Spec:       resourcev1.DeviceClassSpec{Selectors: []resourcev1.DeviceSelector{{CEL: &resourcev1.CELDeviceSelector{}}}},
	}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.selectors[0].cel.expression", "compilation failed")
}
