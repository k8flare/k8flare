package core

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

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
}
