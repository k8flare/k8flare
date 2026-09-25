package core

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
)

type resizeStrategy struct {
	rest.RESTUpdateStrategy
}

func (s resizeStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	newPod, newOK := obj.(*corev1.Pod)
	oldPod, oldOK := old.(*corev1.Pod)
	if !newOK || !oldOK {
		return
	}
	keepResizeFields(newPod, oldPod)
	s.RESTUpdateStrategy.PrepareForUpdate(ctx, obj, old)
}

func keepResizeFields(newPod, oldPod *corev1.Pod) {
	if !sameContainerOrder(newPod.Spec.Containers, oldPod.Spec.Containers) || !sameContainerOrder(newPod.Spec.InitContainers, oldPod.Spec.InitContainers) {
		return
	}
	podResources := newPod.Spec.Resources
	containers := mergeContainerResources(newPod.Spec.Containers, oldPod.Spec.Containers)
	initContainers := mergeContainerResources(newPod.Spec.InitContainers, oldPod.Spec.InitContainers)
	newPod.Spec = *oldPod.Spec.DeepCopy()
	newPod.Status = *oldPod.Status.DeepCopy()
	newPod.Spec.Resources = podResources
	newPod.Spec.Containers = containers
	newPod.Spec.InitContainers = initContainers
}

func sameContainerOrder(new, old []corev1.Container) bool {
	if len(new) != len(old) {
		return false
	}
	for i := range old {
		if new[i].Name != old[i].Name {
			return false
		}
	}
	return true
}

func mergeContainerResources(new, old []corev1.Container) []corev1.Container {
	out := make([]corev1.Container, len(old))
	copy(out, old)
	for i := range out {
		out[i].Resources = new[i].Resources
		out[i].ResizePolicy = new[i].ResizePolicy
	}
	return out
}

func (resizeStrategy) ValidateUpdate(_ context.Context, obj, old runtime.Object) field.ErrorList {
	newPod, newOK := obj.(*corev1.Pod)
	oldPod, oldOK := old.(*corev1.Pod)
	if !newOK || !oldOK {
		return field.ErrorList{field.InternalError(field.NewPath("spec"), fmt.Errorf("not a Pod"))}
	}
	if !sameContainerOrder(newPod.Spec.Containers, oldPod.Spec.Containers) {
		return field.ErrorList{field.Forbidden(field.NewPath("spec", "containers"), "may not add, remove, or reorder containers")}
	}
	if !sameContainerOrder(newPod.Spec.InitContainers, oldPod.Spec.InitContainers) {
		return field.ErrorList{field.Forbidden(field.NewPath("spec", "initContainers"), "may not add, remove, or reorder init containers")}
	}
	return nil
}
