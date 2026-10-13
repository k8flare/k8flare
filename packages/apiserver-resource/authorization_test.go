package resource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

type toServer struct{ base string }

func (t toServer) RoundTrip(req *http.Request) (*http.Response, error) {
	next, err := http.NewRequestWithContext(req.Context(), req.Method, t.base+req.URL.RequestURI(), req.Body)
	if err != nil {
		return nil, err
	}
	return http.DefaultTransport.RoundTrip(next)
}

func emptyKine(t *testing.T) *kine.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kvs": []any{}})
	}))
	t.Cleanup(srv.Close)
	return &kine.Client{HTTP: &http.Client{Transport: toServer{base: srv.URL}}}
}

func claimStatusContext(who user.Info) context.Context {
	ctx := genericapirequest.WithNamespace(context.Background(), "default")
	ctx = genericapirequest.WithUser(ctx, who)
	return genericapirequest.WithRequestInfo(ctx, &genericapirequest.RequestInfo{
		IsResourceRequest: true,
		Verb:              "update",
		APIGroup:          "resource.k8s.io",
		APIVersion:        "v1",
		Resource:          "resourceclaims",
		Subresource:       "status",
		Namespace:         "default",
		Name:              "c",
	})
}

func claimWithoutAllocation() *resourcev1.ResourceClaim {
	return &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "default", ResourceVersion: "1"},
		Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{
			Requests: []resourcev1.DeviceRequest{{Name: "gpu", Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "class", AllocationMode: resourcev1.DeviceAllocationModeExactCount, Count: 1}}},
		}},
	}
}

func claimWithAllocation() *resourcev1.ResourceClaim {
	claim := claimWithoutAllocation()
	claim.Status.Allocation = &resourcev1.AllocationResult{
		Devices: resourcev1.DeviceAllocationResult{
			Results: []resourcev1.DeviceRequestAllocationResult{{Request: "gpu", Driver: "driver.example.com", Pool: "pool", Device: "dev"}},
		},
	}
	return claim
}

func updateClaimStatus(t *testing.T, who user.Info) error {
	t.Helper()
	client := emptyKine(t)
	store, err := registry.NewStore(client, schema.GroupVersion{Group: "resource.k8s.io", Version: "v1"}, metav1.APIResource{Name: "resourceclaims", Kind: "ResourceClaim", Namespaced: true})
	if err != nil {
		t.Fatal(err)
	}
	registry.Customizers["resourceclaims"](store, registry.Deps{Kine: client})
	strategy := registry.StatusStrategyFor(store)
	ctx := claimStatusContext(who)
	obj := claimWithAllocation()
	old := claimWithoutAllocation()
	strategy.PrepareForUpdate(ctx, obj, old)
	return rest.BeforeUpdate(strategy, ctx, obj, old)
}

func TestClaimAllocationNeedsBindingPermission(t *testing.T) {
	err := updateClaimStatus(t, &user.DefaultInfo{Name: "alice", Groups: []string{user.AllAuthenticated}})
	registrytest.RequireFieldError(t, err, "status.allocation", "resourceclaims/binding")
}

func TestClaimAllocationAllowedWithBindingPermission(t *testing.T) {
	if err := updateClaimStatus(t, &user.DefaultInfo{Name: "system:kube-scheduler", Groups: []string{user.AllAuthenticated}}); err != nil {
		t.Fatal(err)
	}
}
