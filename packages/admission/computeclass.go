package admission

import (
	"context"
	"encoding/json"
	"fmt"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/storage/names"
)

const (
	computeClassAnnotation  = "k8flare.com/compute"
	computeClassContainers  = "containers"
	containersBackendLabel  = "k8flare.com/backend"
	containersTaintKey      = "k8flare.com/pod-on-containers"
	nodeVMTierAnnotation    = "k8flare.com/nodevm-tier"
)

func applyComputeClass(ctx context.Context, s *store, req *admit.Request) error {
	if req.Operation != "CREATE" || req.Resource.Resource != "pods" || req.Subresource != "" || req.Object == nil {
		return nil
	}
	raw, err := json.Marshal(req.Object)
	if err != nil {
		return err
	}
	pod := &corev1.Pod{}
	if err := json.Unmarshal(raw, pod); err != nil {
		return err
	}
	nsLabels := map[string]string{}
	if req.Namespace != "" {
		ns, ok, err := s.namespace(ctx, req.Namespace)
		if err != nil {
			return err
		}
		if ok {
			nsLabels = ns.Labels
		}
	}
	if !podWantsContainers(pod, nsLabels) {
		return nil
	}
	mutatePodForComputeClass(pod)
	if err := assignContainersNode(pod); err != nil {
		return err
	}
	out, err := json.Marshal(pod)
	if err != nil {
		return err
	}
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		return err
	}
	req.Object = obj
	return nil
}

func podWantsContainers(pod *corev1.Pod, nsLabels map[string]string) bool {
	if v, ok := pod.Annotations[computeClassAnnotation]; ok {
		return v == computeClassContainers
	}
	return nsLabels[computeClassAnnotation] == computeClassContainers
}

func mutatePodForComputeClass(pod *corev1.Pod) {
	if pod.Spec.NodeSelector == nil {
		pod.Spec.NodeSelector = map[string]string{}
	}
	pod.Spec.NodeSelector[containersBackendLabel] = computeClassContainers
	for _, t := range pod.Spec.Tolerations {
		if t.Key == containersTaintKey {
			pod.Spec.HostNetwork = true
			return
		}
	}
	pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{
		Key:      containersTaintKey,
		Operator: corev1.TolerationOpEqual,
		Value:    "true",
		Effect:   corev1.TaintEffectNoSchedule,
	})
	pod.Spec.HostNetwork = true
}

type sizeTier struct {
	name        string
	milliCPU    int64
	memoryBytes int64
}

var sizeTiers = []sizeTier{
	{name: "small", milliCPU: 63, memoryBytes: 256 * 1024 * 1024},
	{name: "medium", milliCPU: 250, memoryBytes: 1 * 1024 * 1024 * 1024},
	{name: "large", milliCPU: 500, memoryBytes: 4 * 1024 * 1024 * 1024},
}

func podResourceFootprint(pod *corev1.Pod) (milliCPU int64, memoryBytes int64) {
	for _, c := range pod.Spec.Containers {
		reqCPU := c.Resources.Requests.Cpu().MilliValue()
		limCPU := c.Resources.Limits.Cpu().MilliValue()
		if limCPU > reqCPU {
			reqCPU = limCPU
		}
		reqMem := c.Resources.Requests.Memory().Value()
		limMem := c.Resources.Limits.Memory().Value()
		if limMem > reqMem {
			reqMem = limMem
		}
		milliCPU += reqCPU
		memoryBytes += reqMem
	}
	return milliCPU, memoryBytes
}

func resolveSizeTier(milliCPU, memoryBytes int64) string {
	for _, t := range sizeTiers {
		if milliCPU <= t.milliCPU && memoryBytes <= t.memoryBytes {
			return t.name
		}
	}
	return ""
}

func assignContainersNode(pod *corev1.Pod) error {
	if pod.Spec.NodeSelector[corev1.LabelHostname] != "" {
		return nil
	}
	milliCPU, memoryBytes := podResourceFootprint(pod)
	tier := resolveSizeTier(milliCPU, memoryBytes)
	if tier == "" {
		largest := sizeTiers[len(sizeTiers)-1]
		return fmt.Errorf("pod resources (%dm CPU, %d bytes memory) exceed the largest containers size tier %q", milliCPU, memoryBytes, largest.name)
	}
	pod.Spec.NodeSelector[corev1.LabelHostname] = names.SimpleNameGenerator.GenerateName("cf-" + pod.Name + "-")
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[nodeVMTierAnnotation] = tier
	return nil
}
