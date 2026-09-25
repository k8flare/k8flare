package core

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

func TestKeepEphemeralContainersDropsOtherSpec(t *testing.T) {
	old := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Generation: 1},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "pause"}},
			Hostname:   "keep-me",
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	neu := old.DeepCopy()
	neu.Spec.Hostname = "changed"
	neu.Spec.Containers[0].Image = "busybox"
	neu.Status.Phase = corev1.PodSucceeded
	neu.Spec.EphemeralContainers = []corev1.EphemeralContainer{{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "dbg", Image: "busybox"},
	}}
	keepEphemeralContainers(neu, old)
	if neu.Spec.Hostname != "keep-me" || neu.Spec.Containers[0].Image != "pause" {
		t.Fatalf("spec leaked: %#v", neu.Spec)
	}
	if neu.Status.Phase != corev1.PodRunning {
		t.Fatalf("status leaked: %s", neu.Status.Phase)
	}
	if len(neu.Spec.EphemeralContainers) != 1 || neu.Spec.EphemeralContainers[0].Name != "dbg" {
		t.Fatalf("eph %#v", neu.Spec.EphemeralContainers)
	}
}

func TestEphemeralContainersRejectsRemoval(t *testing.T) {
	old := &corev1.Pod{Spec: corev1.PodSpec{EphemeralContainers: []corev1.EphemeralContainer{{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "dbg", Image: "busybox"},
	}}}}
	neu := old.DeepCopy()
	neu.Spec.EphemeralContainers = nil
	if errs := (ephemeralContainersStrategy{}).ValidateUpdate(context.Background(), neu, old); len(errs) == 0 {
		t.Fatal("expected forbid remove")
	}
	neu = old.DeepCopy()
	neu.Spec.EphemeralContainers[0].Image = "other"
	if errs := (ephemeralContainersStrategy{}).ValidateUpdate(context.Background(), neu, old); len(errs) == 0 {
		t.Fatal("expected forbid modify")
	}
	neu = old.DeepCopy()
	neu.Spec.EphemeralContainers = append(neu.Spec.EphemeralContainers, corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "dbg2", Image: "busybox"},
	})
	if errs := (ephemeralContainersStrategy{}).ValidateUpdate(context.Background(), neu, old); len(errs) != 0 {
		t.Fatalf("append should be allowed: %v", errs)
	}
}

func TestEphemeralContainersBumpsGeneration(t *testing.T) {
	old := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Generation: 1},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "pause"}}},
	}
	neu := old.DeepCopy()
	neu.Spec.EphemeralContainers = []corev1.EphemeralContainer{{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "dbg", Image: "busybox"},
	}}
	(ephemeralContainersStrategy{RESTUpdateStrategy: strategyBump{}}).PrepareForUpdate(context.Background(), neu, old)
	if neu.Generation != 2 {
		t.Fatalf("generation=%d want 2", neu.Generation)
	}
	same := old.DeepCopy()
	(ephemeralContainersStrategy{RESTUpdateStrategy: strategyBump{}}).PrepareForUpdate(context.Background(), same, old)
	if same.Generation != 1 {
		t.Fatalf("unchanged generation=%d want 1", same.Generation)
	}
}

type strategyBump struct{ rest.RESTUpdateStrategy }

func (strategyBump) PrepareForUpdate(_ context.Context, obj, old runtime.Object) {
	newPod, oldPod := obj.(*corev1.Pod), old.(*corev1.Pod)
	if !apiequality.Semantic.DeepEqual(newPod.Spec, oldPod.Spec) {
		newPod.Generation = oldPod.Generation + 1
	}
}
