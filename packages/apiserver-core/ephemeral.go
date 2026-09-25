package core

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
)

type ephemeralContainersStrategy struct {
	rest.RESTUpdateStrategy
}

func (s ephemeralContainersStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	newPod, newOK := asPod(obj)
	oldPod, oldOK := asPod(old)
	if newOK && oldOK {
		keepEphemeralContainers(newPod, oldPod)
	}
	if s.RESTUpdateStrategy != nil {
		if newOK && oldOK {
			s.RESTUpdateStrategy.PrepareForUpdate(ctx, newPod, oldPod)
		} else {
			s.RESTUpdateStrategy.PrepareForUpdate(ctx, obj, old)
		}
	}
	if newOK && oldOK && !apiequality.Semantic.DeepEqual(newPod.Spec, oldPod.Spec) && newPod.Generation == oldPod.Generation {
		newPod.Generation = oldPod.Generation + 1
	}
	if newOK && any(newPod) != any(obj) {
		writePod(obj, newPod)
	}
}

func keepEphemeralContainers(newPod, oldPod *corev1.Pod) {
	eph := newPod.Spec.EphemeralContainers
	newPod.Spec = *oldPod.Spec.DeepCopy()
	newPod.Status = *oldPod.Status.DeepCopy()
	newPod.Spec.EphemeralContainers = eph
}

func (ephemeralContainersStrategy) ValidateUpdate(_ context.Context, obj, old runtime.Object) field.ErrorList {
	newPod, newOK := asPod(obj)
	oldPod, oldOK := asPod(old)
	if !newOK || !oldOK {
		return nil
	}
	oldEph := oldPod.Spec.EphemeralContainers
	newEph := newPod.Spec.EphemeralContainers
	if len(newEph) < len(oldEph) {
		return field.ErrorList{field.Forbidden(field.NewPath("spec", "ephemeralContainers"), "may not remove ephemeral containers")}
	}
	for i := range oldEph {
		if !apiequality.Semantic.DeepEqual(oldEph[i], newEph[i]) {
			return field.ErrorList{field.Forbidden(field.NewPath("spec", "ephemeralContainers").Index(i), "may not modify existing ephemeral containers")}
		}
	}
	return nil
}

func asPod(obj runtime.Object) (*corev1.Pod, bool) {
	if pod, ok := obj.(*corev1.Pod); ok {
		return pod, true
	}
	pod := &corev1.Pod{}
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, false
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, pod); err != nil {
		return nil, false
	}
	return pod, true
}

func writePod(dst runtime.Object, src *corev1.Pod) {
	if pod, ok := dst.(*corev1.Pod); ok {
		*pod = *src
		return
	}
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(src)
	if err != nil {
		return
	}
	if setter, ok := dst.(interface {
		SetUnstructuredContent(map[string]interface{})
	}); ok {
		setter.SetUnstructuredContent(raw)
		return
	}
	_ = runtime.DefaultUnstructuredConverter.FromUnstructured(raw, dst)
}
