package registry

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPrepareForCreateSetsGeneration(t *testing.T) {
	s := strategy{}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Generation: 100}}
	s.PrepareForCreate(context.Background(), pod)
	if pod.Generation != 1 {
		t.Fatalf("generation=%d want 1", pod.Generation)
	}
}

func TestPrepareForUpdateBumpsOnSpecChange(t *testing.T) {
	s := strategy{}
	old := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Generation: 1},
		Spec:       corev1.PodSpec{RestartPolicy: corev1.RestartPolicyAlways},
	}
	next := old.DeepCopy()
	next.Spec.RestartPolicy = corev1.RestartPolicyNever
	s.PrepareForUpdate(context.Background(), next, old)
	if next.Generation != 2 {
		t.Fatalf("generation=%d want 2", next.Generation)
	}

	labelOnly := old.DeepCopy()
	labelOnly.Labels = map[string]string{"k": "v"}
	s.PrepareForUpdate(context.Background(), labelOnly, old)
	if labelOnly.Generation != 1 {
		t.Fatalf("label update generation=%d want 1", labelOnly.Generation)
	}
}

func TestPrepareForUpdateSemanticSpecDoesNotBump(t *testing.T) {
	s := strategy{}
	old := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Generation: 1},
		Spec:       corev1.PodSpec{Tolerations: []corev1.Toleration{}},
	}
	next := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Generation: 1},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{}},
	}
	s.PrepareForUpdate(context.Background(), next, old)
	if next.Generation != 1 {
		t.Fatalf("semantic spec generation=%d want 1", next.Generation)
	}
}
