package resource

import (
	"context"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	rbacv1 "k8s.io/api/rbac/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
)

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

func claimStore(t *testing.T) *registry.Store {
	t.Helper()
	cs := registrytest.NewCountingStore(t)
	codec := scheme.Codecs.LegacyCodec(rbacv1.SchemeGroupVersion)
	seed := func(key string, obj runtime.Object) {
		s := kine.NewStorage(cs.Client(), codec, func() runtime.Object { return obj.DeepCopyObject() })
		if err := s.Create(context.Background(), key, obj, obj.DeepCopyObject(), 0); err != nil {
			t.Fatal(err)
		}
	}
	seed("/clusterroles/driver-writer", &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "driver-writer"},
		Rules: []rbacv1.PolicyRule{{
			Verbs:         []string{resourcev1.VerbPrefixArbitraryNode + "update"},
			APIGroups:     []string{"resource.k8s.io"},
			Resources:     []string{"resourceclaims/driver"},
			ResourceNames: []string{"driver.example.com"},
		}},
	})
	seed("/clusterrolebindings/driver-writer", &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "driver-writer"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "driver-writer"},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: "driver-plugin"}},
	})
	store, err := registry.NewStore(cs.Client(), schema.GroupVersion{Group: "resource.k8s.io", Version: "v1"}, metav1.APIResource{Name: "resourceclaims", Kind: "ResourceClaim", Namespaced: true})
	if err != nil {
		t.Fatal(err)
	}
	registry.Customizers["resourceclaims"](store, registry.Deps{Kine: cs.Client()})
	return store
}

func updateClaimStatus(t *testing.T, who user.Info, obj, old *resourcev1.ResourceClaim) error {
	t.Helper()
	store := claimStore(t)
	strategy := registry.StatusStrategyFor(store)
	ctx := claimStatusContext(who)
	strategy.PrepareForUpdate(ctx, obj, old)
	return rest.BeforeUpdate(strategy, ctx, obj, old)
}

func claimWithDeviceStatus() *resourcev1.ResourceClaim {
	claim := claimWithAllocation()
	claim.Status.Devices = []resourcev1.AllocatedDeviceStatus{{Driver: "driver.example.com", Pool: "pool", Device: "dev"}}
	return claim
}

func TestClaimAllocationNeedsBindingPermission(t *testing.T) {
	err := updateClaimStatus(t, &user.DefaultInfo{Name: "alice", Groups: []string{user.AllAuthenticated}}, claimWithAllocation(), claimWithoutAllocation())
	registrytest.RequireFieldError(t, err, "status.allocation", "resourceclaims/binding")
}

func TestClaimAllocationAllowedWithBindingPermission(t *testing.T) {
	who := &user.DefaultInfo{Name: "system:kube-scheduler", Groups: []string{user.AllAuthenticated}}
	if err := updateClaimStatus(t, who, claimWithAllocation(), claimWithoutAllocation()); err != nil {
		t.Fatal(err)
	}
}

func TestClaimDeviceStatusNeedsDriverPermission(t *testing.T) {
	err := updateClaimStatus(t, &user.DefaultInfo{Name: "alice", Groups: []string{user.AllAuthenticated}}, claimWithDeviceStatus(), claimWithAllocation())
	registrytest.RequireFieldError(t, err, "status.devices", "resourceclaims/driver")
}

func TestClaimDeviceStatusAllowedWithDriverPermission(t *testing.T) {
	who := &user.DefaultInfo{Name: "driver-plugin", Groups: []string{user.AllAuthenticated}}
	if err := updateClaimStatus(t, who, claimWithDeviceStatus(), claimWithAllocation()); err != nil {
		t.Fatal(err)
	}
}
