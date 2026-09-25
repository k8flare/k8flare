package core

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/kubernetes/pkg/apis/core/v1/helper/qos"
)

type podCreateStrategy struct {
	rest.RESTCreateStrategy
}

func (s podCreateStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	if s.RESTCreateStrategy != nil {
		s.RESTCreateStrategy.PrepareForCreate(ctx, obj)
	}
	pod := obj.(*corev1.Pod)
	pod.Status = corev1.PodStatus{Phase: corev1.PodPending, QOSClass: qos.GetPodQOS(pod)}
	if len(pod.Spec.SchedulingGates) > 0 {
		pod.Status.Conditions = []corev1.PodCondition{{
			Type:               corev1.PodScheduled,
			Status:             corev1.ConditionFalse,
			Reason:             corev1.PodReasonSchedulingGated,
			Message:            "Scheduling is blocked due to non-empty scheduling gates",
			LastTransitionTime: metav1.Now(),
		}}
	}
}

type podStatusStrategy struct {
	rest.RESTUpdateStrategy
}

func (s podStatusStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	s.RESTUpdateStrategy.PrepareForUpdate(ctx, obj, old)
	pod, oldPod := obj.(*corev1.Pod), old.(*corev1.Pod)
	if oldPod.Spec.NodeName == "" {
		return
	}
	for i := range pod.Status.Conditions {
		c := &pod.Status.Conditions[i]
		if c.Type == corev1.PodScheduled && c.Status != corev1.ConditionTrue {
			println("pods/status: kept PodScheduled=True for", pod.Namespace+"/"+pod.Name, "reason="+c.Reason)
			pod.Status.Conditions = append(pod.Status.Conditions[:i], pod.Status.Conditions[i+1:]...)
			break
		}
	}
	setPodScheduled(pod)
	if pod.Status.QOSClass == "" {
		pod.Status.QOSClass = oldPod.Status.QOSClass
	}
	preservePodObservedGeneration(pod, oldPod)
}

func preservePodObservedGeneration(pod, oldPod *corev1.Pod) {
	if pod.Status.ObservedGeneration == 0 {
		pod.Status.ObservedGeneration = oldPod.Status.ObservedGeneration
	}
	oldGens := map[corev1.PodConditionType][]int64{}
	for _, c := range oldPod.Status.Conditions {
		oldGens[c.Type] = append(oldGens[c.Type], c.ObservedGeneration)
	}
	for i, c := range pod.Status.Conditions {
		var oldGen int64
		if gens := oldGens[c.Type]; len(gens) > 0 {
			oldGen = gens[0]
			oldGens[c.Type] = gens[1:]
		}
		if c.ObservedGeneration == 0 {
			pod.Status.Conditions[i].ObservedGeneration = oldGen
		}
	}
}
