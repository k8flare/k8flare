package admission

import (
	"context"
	"encoding/json"
	"fmt"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
)

func applyRuntimeClass(ctx context.Context, s *store, req *admit.Request) error {
	pod, rc, err := runtimeClassObjects(ctx, s, req)
	if err != nil || pod == nil {
		return err
	}
	if err := setRuntimeClassOverhead(pod, rc); err != nil {
		return err
	}
	if err := setRuntimeClassScheduling(pod, rc); err != nil {
		return err
	}
	return writePodObject(req, pod)
}

func validateRuntimeClass(ctx context.Context, s *store, req *admit.Request) error {
	pod, rc, err := runtimeClassObjects(ctx, s, req)
	if err != nil || pod == nil {
		return err
	}
	if rc != nil && rc.Overhead != nil {
		if !apiequality.Semantic.DeepEqual(rc.Overhead.PodFixed, pod.Spec.Overhead) {
			return fmt.Errorf("pod rejected: Pod's Overhead doesn't match RuntimeClass's defined Overhead")
		}
		return nil
	}
	if pod.Spec.Overhead != nil {
		return fmt.Errorf("pod rejected: Pod Overhead set without corresponding RuntimeClass defined Overhead")
	}
	return nil
}

func runtimeClassObjects(ctx context.Context, s *store, req *admit.Request) (*corev1.Pod, *nodev1.RuntimeClass, error) {
	if req.Operation != "CREATE" || req.Resource.Resource != "pods" || req.Subresource != "" || req.Object == nil {
		return nil, nil, nil
	}
	raw, err := json.Marshal(req.Object)
	if err != nil {
		return nil, nil, err
	}
	pod := &corev1.Pod{}
	if err := json.Unmarshal(raw, pod); err != nil {
		return nil, nil, err
	}
	if pod.Spec.RuntimeClassName == nil || *pod.Spec.RuntimeClassName == "" {
		return pod, nil, nil
	}
	name := *pod.Spec.RuntimeClassName
	rc, ok, err := s.runtimeClass(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	if !ok || !rc.DeletionTimestamp.IsZero() {
		return nil, nil, fmt.Errorf("pod rejected: RuntimeClass %q not found", name)
	}
	return pod, &rc, nil
}

func setRuntimeClassOverhead(pod *corev1.Pod, rc *nodev1.RuntimeClass) error {
	if rc == nil || rc.Overhead == nil {
		return nil
	}
	if len(pod.Spec.Overhead) > 0 && !apiequality.Semantic.DeepEqual(rc.Overhead.PodFixed, pod.Spec.Overhead) {
		return fmt.Errorf("pod rejected: Pod's Overhead doesn't match RuntimeClass's defined Overhead")
	}
	pod.Spec.Overhead = rc.Overhead.PodFixed
	return nil
}

func setRuntimeClassScheduling(pod *corev1.Pod, rc *nodev1.RuntimeClass) error {
	if rc == nil || rc.Scheduling == nil {
		return nil
	}
	selector := pod.Spec.NodeSelector
	if selector == nil {
		selector = map[string]string{}
	}
	for key, want := range rc.Scheduling.NodeSelector {
		if have, ok := selector[key]; ok && have != want {
			return fmt.Errorf("conflict: runtimeClass.scheduling.nodeSelector[%s] = %s; pod.spec.nodeSelector[%s] = %s", key, want, key, have)
		}
		selector[key] = want
	}
	pod.Spec.NodeSelector = selector
	pod.Spec.Tolerations = mergeTolerations(pod.Spec.Tolerations, rc.Scheduling.Tolerations)
	return nil
}

func mergeTolerations(pod, extra []corev1.Toleration) []corev1.Toleration {
	out := append([]corev1.Toleration{}, pod...)
	for _, t := range extra {
		found := false
		for _, e := range out {
			if apiequality.Semantic.DeepEqual(e, t) {
				found = true
				break
			}
		}
		if !found {
			out = append(out, t)
		}
	}
	return out
}

func writePodObject(req *admit.Request, pod *corev1.Pod) error {
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
