package core

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

func TestValidateImmutablePodResourcesRejectsRequestChange(t *testing.T) {
	old := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{
		Name: "c",
		Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("500m"),
		}},
	}}}}
	pod := old.DeepCopy()
	pod.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("100m")
	if errs := validateImmutablePodResources(pod, old); len(errs) == 0 {
		t.Fatal("expected immutable resources")
	}
}

func TestNodeNameOnlySpecChangeDoesNotBumpGeneration(t *testing.T) {
	old := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Generation: 1}, Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyAlways}}
	next := old.DeepCopy()
	next.Spec.NodeName = "n1"
	podUpdateStrategy{RESTUpdateStrategy: nopUpdateStrategy{}}.PrepareForUpdate(context.Background(), next, old)
	if next.Generation != 1 {
		t.Fatalf("generation=%d", next.Generation)
	}
}

func TestSpecChangeOtherThanNodeNameBumpsGeneration(t *testing.T) {
	old := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Generation: 1}, Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyAlways}}
	next := old.DeepCopy()
	next.Spec.RestartPolicy = corev1.RestartPolicyNever
	podUpdateStrategy{RESTUpdateStrategy: bumpOnSpec{}}.PrepareForUpdate(context.Background(), next, old)
	if next.Generation != 2 {
		t.Fatalf("generation=%d", next.Generation)
	}
}

type nopUpdateStrategy struct{ rest.RESTUpdateStrategy }

func (nopUpdateStrategy) PrepareForUpdate(context.Context, runtime.Object, runtime.Object) {}

type bumpOnSpec struct{ rest.RESTUpdateStrategy }

func (bumpOnSpec) PrepareForUpdate(_ context.Context, obj, old runtime.Object) {
	obj.(*corev1.Pod).Generation = old.(*corev1.Pod).Generation + 1
}

func TestValidateImmutablePodResourcesAllowsUnchanged(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{
		Name: "c",
		Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("500m"),
		}},
	}}}}
	if errs := validateImmutablePodResources(pod.DeepCopy(), pod); len(errs) != 0 {
		t.Fatalf("unchanged: %v", errs)
	}
}
