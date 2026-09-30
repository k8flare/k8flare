package storage

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "storage.k8s.io", Version: "v1"}, metav1.APIResource{Name: "storageclasses", Kind: "StorageClass"})
	class := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "s"}}
	scheme.Scheme.Default(class)
	obj := class
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "provisioner", "Required value")
}
