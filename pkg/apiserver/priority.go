package apiserver

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// defaultPriorityWhenNoDefaultClassExists mirrors upstream's
// k8s.io/kubernetes/pkg/apis/scheduling.DefaultPriorityWhenNoDefaultClassExists
// (0) -- not imported directly to avoid pulling in the internal (non-versioned)
// scheduling API package for a single constant.
const defaultPriorityWhenNoDefaultClassExists int32 = 0

// ResolvePodPriority resolves a Pod's spec.priorityClassName into a real
// spec.priority at Pod-create admission time, following real upstream's
// plugin/pkg/admission/priority/admission.go (Plugin.admitPod's Create path,
// k8s.io/kubernetes@v1.36.2-k3s1) -- this is the apiserver's job, not the
// scheduler's; the real, unmodified kube-scheduler binary (cmd/scheduler)
// already preempts correctly on whatever spec.priority ends up set (see
// docs/general-purpose-k8s-plan.md's Phase 5 PriorityClass item).
//
// Deliberate deviation from upstream: a Pod that sets spec.priority directly
// and leaves priorityClassName empty is left completely untouched here.
// Real upstream would instead resolve the (empty) class name to the cluster
// default and then reject the Pod with 403 Forbidden unless spec.priority
// happens to equal that default's value -- but this apiserver already has
// real, working, verified direct-spec.priority preemption (README.md's
// Scheduling table), and preserving that untouched path takes priority over
// exactly matching upstream's stricter behavior here.
func ResolvePodPriority(ctx context.Context, priorityClassStore *ResourceStore, pod *corev1.Pod) error {
	if pod.Spec.PriorityClassName == "" {
		if pod.Spec.Priority != nil {
			// Deviation described above: direct spec.priority, no class name.
			return nil
		}
		name, priority, preemptionPolicy, err := defaultPriority(ctx, priorityClassStore)
		if err != nil {
			return err
		}
		pod.Spec.PriorityClassName = name
		pod.Spec.Priority = &priority
		if preemptionPolicy != nil && pod.Spec.PreemptionPolicy == nil {
			pod.Spec.PreemptionPolicy = preemptionPolicy
		}
		return nil
	}

	pc, err := resolvePriorityClass(ctx, priorityClassStore, pod.Spec.PriorityClassName)
	if err != nil {
		return err
	}
	if pod.Spec.Priority != nil && *pod.Spec.Priority != pc.Value {
		return fmt.Errorf("the integer value of priority (%d) must not be provided in pod spec; priority admission computed %d from PriorityClass %q", *pod.Spec.Priority, pc.Value, pod.Spec.PriorityClassName)
	}
	priority := pc.Value
	pod.Spec.Priority = &priority
	if pc.PreemptionPolicy != nil {
		if pod.Spec.PreemptionPolicy != nil && *pod.Spec.PreemptionPolicy != *pc.PreemptionPolicy {
			return fmt.Errorf("the string value of preemptionPolicy (%s) must not be provided in pod spec; priority admission computed %s from PriorityClass %q", *pod.Spec.PreemptionPolicy, *pc.PreemptionPolicy, pod.Spec.PriorityClassName)
		}
		policy := *pc.PreemptionPolicy
		pod.Spec.PreemptionPolicy = &policy
	}
	return nil
}

// resolvePriorityClass looks up name in priorityClassStore, matching
// upstream's Plugin.resolvePriorityClass: a missing PriorityClass is a 403
// Forbidden (the class name genuinely doesn't resolve), not a 500 -- a
// client naming a class that doesn't exist is a client error.
func resolvePriorityClass(ctx context.Context, priorityClassStore *ResourceStore, name string) (*schedulingv1.PriorityClass, error) {
	obj, err := priorityClassStore.Get(ctx, "", name)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Status.Reason == metav1.StatusReasonNotFound {
			return nil, fmt.Errorf("no PriorityClass with name %v was found", name)
		}
		return nil, fmt.Errorf("failed to resolve PriorityClass with name %s: %w", name, err)
	}
	pc, ok := obj.(*schedulingv1.PriorityClass)
	if !ok {
		return nil, fmt.Errorf("resource was marked with kind PriorityClass but was unable to be converted")
	}
	return pc, nil
}

// defaultPriority returns the cluster's default priority: the value of the
// PriorityClass with GlobalDefault set (lowest Value wins if more than one
// somehow claims GlobalDefault, matching upstream's tie-break for that race),
// or defaultPriorityWhenNoDefaultClassExists if none exists.
func defaultPriority(ctx context.Context, priorityClassStore *ResourceStore) (string, int32, *corev1.PreemptionPolicy, error) {
	listObj, err := priorityClassStore.List(ctx, "", "", "")
	if err != nil {
		return "", 0, nil, fmt.Errorf("error occurred while retrieving default priority class: %w", err)
	}
	list, ok := listObj.(*schedulingv1.PriorityClassList)
	if !ok {
		return "", 0, nil, fmt.Errorf("error occurred while retrieving default priority class: unexpected list type")
	}

	var defaultPC *schedulingv1.PriorityClass
	for i := range list.Items {
		pc := &list.Items[i]
		if !pc.GlobalDefault {
			continue
		}
		if defaultPC == nil || defaultPC.Value > pc.Value {
			defaultPC = pc
		}
	}
	if defaultPC != nil {
		return defaultPC.Name, defaultPC.Value, defaultPC.PreemptionPolicy, nil
	}
	preemptLowerPriority := corev1.PreemptLowerPriority
	return "", defaultPriorityWhenNoDefaultClassExists, &preemptLowerPriority, nil
}
