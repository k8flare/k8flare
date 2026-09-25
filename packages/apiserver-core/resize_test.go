package core

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKeepResizeFieldsDropsOtherSpec(t *testing.T) {
	old := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p"},
		Spec: corev1.PodSpec{
			Hostname: "keep-me",
			Containers: []corev1.Container{{
				Name:  "app",
				Image: "pause",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	neu := old.DeepCopy()
	neu.Spec.Hostname = "changed"
	neu.Spec.Containers[0].Image = "busybox"
	neu.Status.Phase = corev1.PodSucceeded
	neu.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("100m")
	keepResizeFields(neu, old)
	if neu.Spec.Hostname != "keep-me" || neu.Spec.Containers[0].Image != "pause" {
		t.Fatalf("spec leaked: %#v", neu.Spec)
	}
	if neu.Status.Phase != corev1.PodRunning {
		t.Fatalf("status leaked: %s", neu.Status.Phase)
	}
	if got := neu.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]; got.Cmp(resource.MustParse("100m")) != 0 {
		t.Fatalf("cpu %s", got.String())
	}
}

func TestResizeRejectsContainerReorder(t *testing.T) {
	old := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "a"}, {Name: "b"}}}}
	neu := old.DeepCopy()
	neu.Spec.Containers[0].Name = "b"
	neu.Spec.Containers[1].Name = "a"
	if errs := (resizeStrategy{}).ValidateUpdate(context.Background(), neu, old); len(errs) == 0 {
		t.Fatal("expected forbid reorder")
	}
	neu = old.DeepCopy()
	if errs := (resizeStrategy{}).ValidateUpdate(context.Background(), neu, old); len(errs) != 0 {
		t.Fatalf("same order: %v", errs)
	}
}
