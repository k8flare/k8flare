package core

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodUpdateKeepsStatus(t *testing.T) {
	old := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.8"}}
	next := old.DeepCopy()
	next.Status.Phase = corev1.PodPending
	next.Status.PodIP = ""
	podUpdateStrategy{}.PrepareForUpdate(context.Background(), next, old)
	if next.Status.Phase != corev1.PodRunning || next.Status.PodIP != "10.0.0.8" {
		t.Fatalf("status=%+v", next.Status)
	}
}

func TestPodCreateMarksSchedulingGated(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "gated"}, Spec: corev1.PodSpec{
		SchedulingGates: []corev1.PodSchedulingGate{{Name: "example.com/hold"}},
		Containers:      []corev1.Container{{Name: "c", Image: "img"}},
	}}
	podCreateStrategy{}.PrepareForCreate(context.Background(), pod)
	if pod.Status.Phase != corev1.PodPending {
		t.Fatalf("phase=%s", pod.Status.Phase)
	}
	if len(pod.Status.Conditions) != 1 || pod.Status.Conditions[0].Type != corev1.PodScheduled || pod.Status.Conditions[0].Status != corev1.ConditionFalse || pod.Status.Conditions[0].Reason != corev1.PodReasonSchedulingGated {
		t.Fatalf("conditions=%v", pod.Status.Conditions)
	}
	open := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}}}
	podCreateStrategy{}.PrepareForCreate(context.Background(), open)
	if len(open.Status.Conditions) != 0 {
		t.Fatalf("conditions=%v", open.Status.Conditions)
	}
}
