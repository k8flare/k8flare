package core

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
)

type podUpdateStrategy struct {
	rest.RESTUpdateStrategy
}

func (s podUpdateStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	newPod, newOK := obj.(*corev1.Pod)
	oldPod, oldOK := old.(*corev1.Pod)
	nodeNameOnly := newOK && oldOK && nodeNameOnlySpecChange(newPod, oldPod)
	if s.RESTUpdateStrategy != nil {
		s.RESTUpdateStrategy.PrepareForUpdate(ctx, obj, old)
	}
	if nodeNameOnly {
		newPod.Generation = oldPod.Generation
	}
	if newOK && oldOK {
		newPod.Status = oldPod.Status
	}
}

func nodeNameOnlySpecChange(newPod, oldPod *corev1.Pod) bool {
	if newPod.Spec.NodeName == oldPod.Spec.NodeName {
		return false
	}
	a, b := newPod.Spec.DeepCopy(), oldPod.Spec.DeepCopy()
	a.NodeName = ""
	b.NodeName = ""
	return apiequality.Semantic.DeepEqual(a, b)
}

func (s podUpdateStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	var errs field.ErrorList
	if s.RESTUpdateStrategy != nil {
		errs = s.RESTUpdateStrategy.ValidateUpdate(ctx, obj, old)
	}
	return append(errs, validateImmutablePodResources(obj.(*corev1.Pod), old.(*corev1.Pod))...)
}

func validateImmutablePodResources(pod, old *corev1.Pod) field.ErrorList {
	var errs field.ErrorList
	if !sameContainerResources(pod.Spec.Containers, old.Spec.Containers) {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "containers"), "resource requirements are immutable"))
	}
	if !sameContainerResources(pod.Spec.InitContainers, old.Spec.InitContainers) {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "initContainers"), "resource requirements are immutable"))
	}
	return errs
}

func sameContainerResources(new, old []corev1.Container) bool {
	if len(new) != len(old) {
		return false
	}
	for i := range old {
		if !reflect.DeepEqual(new[i].Resources, old[i].Resources) {
			return false
		}
	}
	return true
}
