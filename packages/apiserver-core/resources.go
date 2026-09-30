package core

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
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
