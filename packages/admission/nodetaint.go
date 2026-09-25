package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
)

func applyTaintNodesByCondition(_ context.Context, _ *store, req *admit.Request) error {
	if req.Resource.Resource != "nodes" || req.Subresource != "" || req.Object == nil {
		return nil
	}
	if req.Operation != "" && req.Operation != "CREATE" {
		return nil
	}
	spec, _ := req.Object["spec"].(map[string]any)
	if spec == nil {
		spec = map[string]any{}
		req.Object["spec"] = spec
	}
	taints, _ := spec["taints"].([]any)
	for _, raw := range taints {
		t, _ := raw.(map[string]any)
		if t == nil {
			continue
		}
		key, _ := t["key"].(string)
		effect, _ := t["effect"].(string)
		if key == corev1.TaintNodeNotReady && effect == string(corev1.TaintEffectNoSchedule) {
			return nil
		}
	}
	spec["taints"] = append(taints, map[string]any{
		"key":    corev1.TaintNodeNotReady,
		"effect": string(corev1.TaintEffectNoSchedule),
	})
	return nil
}
