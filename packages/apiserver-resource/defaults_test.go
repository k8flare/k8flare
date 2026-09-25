package resource

import (
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDefaultsDeviceRequestCount(t *testing.T) {
	claim := &resourcev1.ResourceClaim{Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{
		Requests: []resourcev1.DeviceRequest{{
			Name:    "gpu",
			Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "example.com"},
		}},
	}}}
	scheme.Scheme.Default(claim)
	got := claim.Spec.Devices.Requests[0].Exactly
	if got.AllocationMode != resourcev1.DeviceAllocationModeExactCount || got.Count != 1 {
		t.Fatalf("mode=%s count=%d", got.AllocationMode, got.Count)
	}
}
