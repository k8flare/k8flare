package resource

import (
	"net/http"
	"net/http/httptest"
	"testing"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
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

func TestResourceClaimRejectsEmptyDeviceClassName(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "resource.k8s.io", Version: "v1"}, metav1.APIResource{Name: "resourceclaims", Kind: "ResourceClaim", Namespaced: true})
	obj := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "default"},
		Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{
			Requests: []resourcev1.DeviceRequest{{Name: "gpu", Exactly: &resourcev1.ExactDeviceRequest{}}},
		}},
	}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.devices.requests[0].exactly.deviceClassName", "")
}

func TestResourceClaimTemplateRejectsEmptyDeviceClassName(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "resource.k8s.io", Version: "v1"}, metav1.APIResource{Name: "resourceclaimtemplates", Kind: "ResourceClaimTemplate", Namespaced: true})
	obj := &resourcev1.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "default"},
		Spec: resourcev1.ResourceClaimTemplateSpec{Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{
			Requests: []resourcev1.DeviceRequest{{Name: "gpu", Exactly: &resourcev1.ExactDeviceRequest{}}},
		}}},
	}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.spec.devices.requests[0].exactly.deviceClassName", "")
}

func TestClaimStatusWritesCarryRequestInfo(t *testing.T) {
	var resource, subresource string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		info, ok := genericapirequest.RequestInfoFrom(r.Context())
		if ok {
			resource, subresource = info.Resource, info.Subresource
		}
	})
	handler := registry.Middleware[len(registry.Middleware)-1](nil)(next)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/apis/resource.k8s.io/v1/namespaces/default/resourceclaims/c/status", nil))
	if resource != "resourceclaims" || subresource != "status" {
		t.Fatalf("resource=%q subresource=%q", resource, subresource)
	}
}
