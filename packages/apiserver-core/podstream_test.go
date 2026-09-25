package core

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKubeletStreamParams(t *testing.T) {
	q := kubeletStreamParams(&corev1.PodExecOptions{Stdout: true, Stderr: true, Command: []string{"echo", "ok"}})
	if q.Get("output") != "1" || q.Get("error") != "1" || q.Get("input") != "" {
		t.Fatalf("flags %v", q)
	}
	if got := q["command"]; len(got) != 2 || got[0] != "echo" || got[1] != "ok" {
		t.Fatalf("command %v", got)
	}
}

func TestStreamContainerDefaults(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "only"}}},
	}
	got, err := streamContainer("exec", pod, &corev1.PodExecOptions{})
	if err != nil || got != "only" {
		t.Fatalf("got %q %v", got, err)
	}
	got, err = streamContainer("exec", pod, &corev1.PodExecOptions{Container: "only"})
	if err != nil || got != "only" {
		t.Fatalf("named %q %v", got, err)
	}
}
