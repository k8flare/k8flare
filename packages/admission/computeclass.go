package admission

import (
	"context"
	"encoding/json"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
)

const (
	computeClassAnnotation  = "k8flare.com/compute"
	computeClassContainers  = "containers"
	containersTaintKey      = "k8flare.com/pod-on-containers"
	containersSchedulerName = "k8flare-containers"
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
	pod.Spec.SchedulerName = containersSchedulerName
	if pod.Spec.AutomountServiceAccountToken == nil {
		automount := false
		pod.Spec.AutomountServiceAccountToken = &automount
	}
	for _, t := range pod.Spec.Tolerations {
		if t.Key == containersTaintKey {
			return
		}
	}
	pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{
		Key:      containersTaintKey,
		Operator: corev1.TolerationOpEqual,
		Value:    "true",
		Effect:   corev1.TaintEffectNoSchedule,
	})
}
