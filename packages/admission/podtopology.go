package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
)

func applyPodTopologyLabels(ctx context.Context, s *store, req *admit.Request) error {
	if req.Resource.Resource != "pods" || req.Object == nil {
		return nil
	}
	switch req.Subresource {
	case "binding":
		return applyBindingTopologyLabels(ctx, s, req)
	case "":
		return applyPodSpecTopologyLabels(ctx, s, req)
	default:
		return nil
	}
}

func applyBindingTopologyLabels(ctx context.Context, s *store, req *admit.Request) error {
	if req.Operation != "" && req.Operation != "CREATE" {
		return nil
	}
	target, _ := req.Object["target"].(map[string]any)
	if target == nil {
		return nil
	}
	kind, _ := target["kind"].(string)
	if kind != "Node" {
		return nil
	}
	name, _ := target["name"].(string)
	labels, err := topologyLabels(ctx, s, name)
	if err != nil || len(labels) == 0 {
		return err
	}
	mergeObjectLabels(req.Object, labels)
	return nil
}

func applyPodSpecTopologyLabels(ctx context.Context, s *store, req *admit.Request) error {
	if req.Operation != "" && req.Operation != "CREATE" && req.Operation != "UPDATE" {
		return nil
	}
	spec, _ := req.Object["spec"].(map[string]any)
	if spec == nil {
		return nil
	}
	nodeName, _ := spec["nodeName"].(string)
	if nodeName == "" {
		return nil
	}
	labels, err := topologyLabels(ctx, s, nodeName)
	if err != nil || len(labels) == 0 {
		return err
	}
	mergeObjectLabels(req.Object, labels)
	return nil
}

func topologyLabels(ctx context.Context, s *store, nodeName string) (map[string]string, error) {
	if s == nil || nodeName == "" {
		return nil, nil
	}
	node, ok, err := s.node(ctx, nodeName)
	if err != nil || !ok {
		return nil, err
	}
	out := map[string]string{}
	for _, key := range []string{corev1.LabelTopologyZone, corev1.LabelTopologyRegion} {
		if v := node.Labels[key]; v != "" {
			out[key] = v
		}
	}
	return out, nil
}

func mergeObjectLabels(obj map[string]any, labels map[string]string) {
	meta, _ := obj["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		obj["metadata"] = meta
	}
	existing, _ := meta["labels"].(map[string]any)
	if existing == nil {
		existing = map[string]any{}
		meta["labels"] = existing
	}
	for k, v := range labels {
		existing[k] = v
	}
}
