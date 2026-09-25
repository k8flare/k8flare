package admission

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

func TestAssignContainersNode(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web"}}
	mutatePodForComputeClass(pod)
	if err := assignContainersNode(pod); err != nil {
		t.Fatal(err)
	}
	if pod.Spec.NodeSelector[containersBackendLabel] != computeClassContainers {
		t.Fatalf("backend: %v", pod.Spec.NodeSelector)
	}
	if pod.Spec.NodeSelector[corev1.LabelHostname] == "" || pod.Annotations[nodeVMTierAnnotation] != "small" {
		t.Fatalf("pin: %v %v", pod.Spec.NodeSelector, pod.Annotations)
	}
	if !pod.Spec.HostNetwork {
		t.Fatal("hostNetwork")
	}
	pod.Spec.Containers = []corev1.Container{{
		Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("8Gi")}},
	}}
	pod.Spec.NodeSelector[corev1.LabelHostname] = ""
	if err := assignContainersNode(pod); err == nil {
		t.Fatal("expected oversized reject")
	}
}
