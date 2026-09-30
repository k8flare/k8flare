package admission

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodWantsContainers(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{computeClassAnnotation: computeClassContainers}}}
	if !podWantsContainers(pod, nil) {
		t.Fatal("annotation")
	}
	plain := &corev1.Pod{}
	if !podWantsContainers(plain, map[string]string{computeClassAnnotation: computeClassContainers}) {
		t.Fatal("namespace")
	}
	off := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{computeClassAnnotation: "vm"}}}
	if podWantsContainers(off, map[string]string{computeClassAnnotation: computeClassContainers}) {
		t.Fatal("opt-out")
	}
}

func TestMutatePodForComputeClass(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web"}}
	mutatePodForComputeClass(pod)
	if pod.Spec.SchedulerName != containersSchedulerName {
		t.Fatalf("schedulerName = %q", pod.Spec.SchedulerName)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatal("automountServiceAccountToken must default to false")
	}
	if len(pod.Spec.Tolerations) != 1 || pod.Spec.Tolerations[0].Key != containersTaintKey || pod.Spec.Tolerations[0].Effect != corev1.TaintEffectNoSchedule {
		t.Fatalf("tolerations = %v", pod.Spec.Tolerations)
	}
	if pod.Spec.HostNetwork || len(pod.Spec.NodeSelector) != 0 || len(pod.Annotations) != 0 {
		t.Fatalf("NodeVM routing leaked: hostNetwork=%v nodeSelector=%v annotations=%v", pod.Spec.HostNetwork, pod.Spec.NodeSelector, pod.Annotations)
	}
	automount := true
	explicit := &corev1.Pod{Spec: corev1.PodSpec{AutomountServiceAccountToken: &automount, Tolerations: pod.Spec.Tolerations}}
	mutatePodForComputeClass(explicit)
	if !*explicit.Spec.AutomountServiceAccountToken || len(explicit.Spec.Tolerations) != 1 {
		t.Fatalf("explicit fields overwritten: %+v", explicit.Spec)
	}
}
