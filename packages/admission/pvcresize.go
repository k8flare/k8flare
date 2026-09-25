package admission

import (
	"context"
	"fmt"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func applyPersistentVolumeClaimResize(ctx context.Context, s *store, req *admit.Request) error {
	if req.Resource.Resource != "persistentvolumeclaims" || req.Subresource != "" {
		return nil
	}
	if req.Operation != "" && req.Operation != "UPDATE" {
		return nil
	}
	if req.Object == nil || req.OldObject == nil {
		return nil
	}
	oldSize, okOld := pvcStorageRequest(req.OldObject)
	newSize, okNew := pvcStorageRequest(req.Object)
	if !okOld || !okNew || newSize.Cmp(oldSize) <= 0 {
		return nil
	}
	if pvcPhase(req.OldObject) != string(corev1.ClaimBound) {
		return fmt.Errorf("Only bound persistent volume claims can be expanded")
	}
	if !allowPVCResize(ctx, s, req.Object, req.OldObject) {
		return fmt.Errorf("only dynamically provisioned pvc can be resized and the storageclass that provisions the pvc must support resize")
	}
	return nil
}

func allowPVCResize(ctx context.Context, s *store, pvc, old map[string]any) bool {
	class := pvcStorageClassName(pvc)
	if class == "" || class != pvcStorageClassName(old) {
		return false
	}
	classes, err := s.storageClasses(ctx)
	if err != nil {
		return false
	}
	for _, sc := range classes {
		if sc.Name == class && sc.AllowVolumeExpansion != nil {
			return *sc.AllowVolumeExpansion
		}
	}
	return false
}

func pvcStorageClassName(obj map[string]any) string {
	spec, _ := obj["spec"].(map[string]any)
	name, _ := spec["storageClassName"].(string)
	return name
}

func pvcPhase(obj map[string]any) string {
	status, _ := obj["status"].(map[string]any)
	phase, _ := status["phase"].(string)
	return phase
}

func pvcStorageRequest(obj map[string]any) (resource.Quantity, bool) {
	spec, _ := obj["spec"].(map[string]any)
	resources, _ := spec["resources"].(map[string]any)
	requests, _ := resources["requests"].(map[string]any)
	raw, ok := requests["storage"]
	if !ok {
		return resource.Quantity{}, false
	}
	switch v := raw.(type) {
	case string:
		q, err := resource.ParseQuantity(v)
		return q, err == nil
	default:
		return resource.Quantity{}, false
	}
}
