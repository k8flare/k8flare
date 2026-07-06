package apiserver

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Pure unit tests for the compute-class admission mutation; no wrangler
// dev instance involved (TestMain only reaps devCmd if a test started one).

func TestPodWantsContainers(t *testing.T) {
	containersNS := map[string]string{ComputeClassAnnotation: ComputeClassContainers}
	cases := []struct {
		name     string
		podAnn   map[string]string
		nsLabels map[string]string
		want     bool
	}{
		{"pod annotation opts in from plain namespace", map[string]string{ComputeClassAnnotation: ComputeClassContainers}, nil, true},
		{"namespace label routes unannotated pod", nil, containersNS, true},
		{"pod annotation opts out of containers namespace", map[string]string{ComputeClassAnnotation: "default"}, containersNS, false},
		{"no annotation, plain namespace", nil, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: tc.podAnn}}
			if got := PodWantsContainers(pod, tc.nsLabels); got != tc.want {
				t.Errorf("PodWantsContainers = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMutatePodForComputeClass(t *testing.T) {
	pod := &corev1.Pod{}
	MutatePodForComputeClass(pod)

	if pod.Spec.SchedulerName != ContainersSchedulerName {
		t.Errorf("schedulerName = %q, want %q", pod.Spec.SchedulerName, ContainersSchedulerName)
	}
	if pod.Spec.NodeSelector[containersBackendLabel] != ComputeClassContainers {
		t.Errorf("nodeSelector[%s] = %q, want %q", containersBackendLabel, pod.Spec.NodeSelector[containersBackendLabel], ComputeClassContainers)
	}
	if !pod.Spec.HostNetwork {
		t.Error("hostNetwork not injected")
	}
	if pod.Spec.DNSPolicy != "" {
		t.Errorf("dnsPolicy mutated to %q, want untouched", pod.Spec.DNSPolicy)
	}

	// Idempotent: a second pass must not duplicate the toleration.
	MutatePodForComputeClass(pod)
	count := 0
	for _, tol := range pod.Spec.Tolerations {
		if tol.Key == containersTaintKey {
			count++
		}
	}
	if count != 1 {
		t.Errorf("toleration injected %d times, want 1", count)
	}

	// An explicit user scheduler choice is respected.
	custom := &corev1.Pod{Spec: corev1.PodSpec{SchedulerName: "my-scheduler"}}
	MutatePodForComputeClass(custom)
	if custom.Spec.SchedulerName != "my-scheduler" {
		t.Errorf("explicit schedulerName overwritten: %q", custom.Spec.SchedulerName)
	}
	if !custom.Spec.HostNetwork {
		t.Error("hostNetwork not injected on explicit-scheduler pod")
	}
}
