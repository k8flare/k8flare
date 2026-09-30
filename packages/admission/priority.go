package admission

import (
	"context"
	"encoding/json"
	"fmt"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	schedhelpers "k8s.io/kubernetes/pkg/apis/scheduling/v1"
)

func applyPriority(ctx context.Context, s *store, req *admit.Request) error {
	if req.Resource.Resource != "pods" || req.Subresource != "" || req.Object == nil {
		return nil
	}
	if req.Operation != "CREATE" && req.Operation != "UPDATE" {
		return nil
	}
	pod, err := decodePod(req.Object)
	if err != nil {
		return err
	}
	if req.Operation == "UPDATE" {
		old, err := decodePod(req.OldObject)
		if err != nil {
			return err
		}
		if pod.Spec.Priority == nil && old != nil && old.Spec.Priority != nil {
			pod.Spec.Priority = old.Spec.Priority
		}
		if pod.Spec.PreemptionPolicy == nil && old != nil && old.Spec.PreemptionPolicy != nil {
			pod.Spec.PreemptionPolicy = old.Spec.PreemptionPolicy
		}
		return writePodObject(req, pod)
	}
	name, value, policy, err := resolvePriority(ctx, s, pod.Spec.PriorityClassName)
	if err != nil {
		return err
	}
	pod.Spec.PriorityClassName = name
	if pod.Spec.Priority != nil && *pod.Spec.Priority != value {
		return fmt.Errorf("the integer value of priority (%d) must not be provided in pod spec; priority admission controller computed %d from the given PriorityClass name", *pod.Spec.Priority, value)
	}
	pod.Spec.Priority = &value
	if policy != nil {
		if pod.Spec.PreemptionPolicy != nil && *pod.Spec.PreemptionPolicy != *policy {
			return fmt.Errorf("the string value of PreemptionPolicy (%s) must not be provided in pod spec; priority admission controller computed %s from the given PriorityClass name", *pod.Spec.PreemptionPolicy, *policy)
		}
		pod.Spec.PreemptionPolicy = policy
	}
	return writePodObject(req, pod)
}

func resolvePriority(ctx context.Context, s *store, className string) (string, int32, *corev1.PreemptionPolicy, error) {
	if className == "" {
		return defaultPriority(ctx, s)
	}
	if s == nil {
		return "", 0, nil, fmt.Errorf("no PriorityClass with name %s was found", className)
	}
	pc, ok, err := s.priorityClass(ctx, className)
	if err != nil {
		return "", 0, nil, err
	}
	if !ok {
		pc, ok = systemPriorityClass(className)
	}
	if !ok {
		return "", 0, nil, fmt.Errorf("no PriorityClass with name %s was found", className)
	}
	return className, pc.Value, pc.PreemptionPolicy, nil
}

func systemPriorityClass(name string) (schedulingv1.PriorityClass, bool) {
	for _, pc := range schedhelpers.SystemPriorityClasses() {
		if pc.Name == name {
			return *pc, true
		}
	}
	return schedulingv1.PriorityClass{}, false
}

func defaultPriority(ctx context.Context, s *store) (string, int32, *corev1.PreemptionPolicy, error) {
	preempt := corev1.PreemptLowerPriority
	if s == nil {
		return "", 0, &preempt, nil
	}
	list, err := s.priorityClasses(ctx)
	if err != nil {
		return "", 0, nil, err
	}
	var def *schedulingv1.PriorityClass
	for i := range list {
		pc := &list[i]
		if !pc.GlobalDefault {
			continue
		}
		if def == nil || def.Value > pc.Value {
			def = pc
		}
	}
	if def != nil {
		return def.Name, def.Value, def.PreemptionPolicy, nil
	}
	return "", 0, &preempt, nil
}

func decodePod(obj map[string]any) (*corev1.Pod, error) {
	if obj == nil {
		return &corev1.Pod{}, nil
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	pod := &corev1.Pod{}
	if err := json.Unmarshal(raw, pod); err != nil {
		return nil, err
	}
	return pod, nil
}
