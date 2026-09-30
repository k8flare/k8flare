package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
)

const defaultTolerationSeconds = int64(300)

func applyDefaultTolerationSeconds(_ context.Context, _ *store, req *admit.Request) error {
	if req.Operation != "" && req.Operation != "CREATE" && req.Operation != "UPDATE" {
		return nil
	}
	if req.Resource.Resource != "pods" || req.Subresource != "" || req.Object == nil {
		return nil
	}
	spec, _ := req.Object["spec"].(map[string]any)
	if spec == nil {
		return nil
	}
	tolerations, _ := spec["tolerations"].([]any)
	hasNotReady := false
	hasUnreachable := false
	for _, raw := range tolerations {
		t, _ := raw.(map[string]any)
		if t == nil {
			continue
		}
		key, _ := t["key"].(string)
		effect, _ := t["effect"].(string)
		if (key == corev1.TaintNodeNotReady || key == "") && (effect == string(corev1.TaintEffectNoExecute) || effect == "") {
			hasNotReady = true
		}
		if (key == corev1.TaintNodeUnreachable || key == "") && (effect == string(corev1.TaintEffectNoExecute) || effect == "") {
			hasUnreachable = true
		}
	}
	if !hasNotReady {
		tolerations = append(tolerations, defaultNodeToleration(corev1.TaintNodeNotReady))
	}
	if !hasUnreachable {
		tolerations = append(tolerations, defaultNodeToleration(corev1.TaintNodeUnreachable))
	}
	spec["tolerations"] = tolerations
	return nil
}

func defaultNodeToleration(key string) map[string]any {
	return map[string]any{
		"key":               key,
		"operator":          string(corev1.TolerationOpExists),
		"effect":            string(corev1.TaintEffectNoExecute),
		"tolerationSeconds": defaultTolerationSeconds,
	}
}
