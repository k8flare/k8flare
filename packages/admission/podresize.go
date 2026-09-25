package admission

import (
	"context"
	"fmt"
	"strings"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/component-helpers/resource"
)

func validatePodResize(ctx context.Context, s *store, req *admit.Request) error {
	if req.Resource.Resource != "pods" || req.Subresource != "resize" || req.Object == nil {
		return nil
	}
	if req.Operation != "" && req.Operation != "UPDATE" {
		return nil
	}
	pod, err := decodePod(req.Object)
	if err != nil {
		return err
	}
	if pod.Spec.NodeName == "" {
		return nil
	}
	old, err := decodePod(req.OldObject)
	if err != nil {
		return err
	}
	if old != nil && old.Generation == pod.Generation {
		return nil
	}
	if s == nil {
		return fmt.Errorf("node %q not found", pod.Spec.NodeName)
	}
	node, ok, err := s.node(ctx, pod.Spec.NodeName)
	if err != nil {
		return fmt.Errorf("failed to get node %q: %w", pod.Spec.NodeName, err)
	}
	if !ok {
		return fmt.Errorf("node %q not found", pod.Spec.NodeName)
	}
	if err := resizeLinuxNode(&node); err != nil {
		return err
	}
	return resizeWithinAllocatable(pod, node.Status.Allocatable)
}

func resizeLinuxNode(node *corev1.Node) error {
	val, ok := node.Labels[corev1.LabelOSStable]
	if !ok || val == "linux" {
		return nil
	}
	return fmt.Errorf("pod resize is only supported on linux nodes, node %q is %q", node.Name, val)
}

func resizeWithinAllocatable(pod *corev1.Pod, allocatable corev1.ResourceList) error {
	requests := resource.PodRequests(pod, resource.PodResourcesOptions{})
	cpuRequests := requests[corev1.ResourceCPU]
	memRequests := requests[corev1.ResourceMemory]
	var msg []string
	if cpu := allocatable.Cpu(); cpuRequests.Cmp(*cpu) > 0 {
		msg = append(msg, fmt.Sprintf("cpu, requested: %d, allocatable: %d", cpuRequests.MilliValue(), cpu.MilliValue()))
	}
	if mem := allocatable.Memory(); memRequests.Cmp(*mem) > 0 {
		msg = append(msg, fmt.Sprintf("memory, requested: %d, allocatable: %d", memRequests.Value(), mem.Value()))
	}
	if len(msg) == 0 {
		return nil
	}
	return fmt.Errorf("node didn't have enough allocatable resources: %s", strings.Join(msg, "; "))
}
