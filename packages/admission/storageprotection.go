package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
)

const (
	pvcProtectionFinalizer = "kubernetes.io/pvc-protection"
	pvProtectionFinalizer  = "kubernetes.io/pv-protection"
	vacProtectionFinalizer = "kubernetes.io/vac-protection"
)

func applyStorageObjectInUseProtection(_ context.Context, _ *store, req *admit.Request) error {
	if req.Subresource != "" || req.Object == nil {
		return nil
	}
	if req.Operation != "" && req.Operation != "CREATE" {
		return nil
	}
	var finalizer string
	switch req.Resource.Resource {
	case "persistentvolumeclaims":
		finalizer = pvcProtectionFinalizer
	case "persistentvolumes":
		finalizer = pvProtectionFinalizer
	case "volumeattributesclasses":
		finalizer = vacProtectionFinalizer
	default:
		return nil
	}
	meta, _ := req.Object["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		req.Object["metadata"] = meta
	}
	raw, _ := meta["finalizers"].([]any)
	for _, item := range raw {
		if s, ok := item.(string); ok && s == finalizer {
			return nil
		}
	}
	meta["finalizers"] = append(raw, finalizer)
	return nil
}
