package apiserver

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

	// schedulerName is left untouched: these Pods are bound by the real
	// kube-scheduler via the standard default-scheduler path, not a
	// dedicated binder (see the func's own doc comment).
	if pod.Spec.SchedulerName != "" {
		t.Errorf("schedulerName = %q, want untouched (empty)", pod.Spec.SchedulerName)
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

	// An explicit user scheduler choice is respected (this repo never
	// sets schedulerName itself, but shouldn't clobber a user's own).
	custom := &corev1.Pod{Spec: corev1.PodSpec{SchedulerName: "my-scheduler"}}
	MutatePodForComputeClass(custom)
	if custom.Spec.SchedulerName != "my-scheduler" {
		t.Errorf("explicit schedulerName overwritten: %q", custom.Spec.SchedulerName)
	}
	if !custom.Spec.HostNetwork {
		t.Error("hostNetwork not injected on explicit-scheduler pod")
	}
}

func TestAssignContainersNode(t *testing.T) {
	newPod := func(requests corev1.ResourceList) *corev1.Pod {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: requests}}},
			},
		}
		MutatePodForComputeClass(pod)
		return pod
	}

	cases := []struct {
		name     string
		requests corev1.ResourceList
		wantTier string
		wantErr  bool
	}{
		{"no requests defaults to small", nil, "small", false},
		{"fits small", corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("128Mi")}, "small", false},
		{"fits medium", corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("512Mi")}, "medium", false},
		{"fits large", corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("4Gi")}, "large", false},
		{"exceeds largest tier", corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4"), corev1.ResourceMemory: resource.MustParse("16Gi")}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pod := newPod(tc.requests)
			err := AssignContainersNode(pod)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := pod.Annotations[NodeVMTierAnnotation]; got != tc.wantTier {
				t.Errorf("tier annotation = %q, want %q", got, tc.wantTier)
			}
			hostname := pod.Spec.NodeSelector[corev1.LabelHostname]
			if !strings.HasPrefix(hostname, "cf-") {
				t.Errorf("nodeSelector[%s] = %q, want a cf- prefixed name", corev1.LabelHostname, hostname)
			}
		})
	}

	// A pod that already carries a hostname pin (e.g. a second admission
	// pass) is left alone -- the real scheduler must always see the same
	// pin across retries of the same Pod object.
	pod := newPod(nil)
	if err := AssignContainersNode(pod); err != nil {
		t.Fatalf("first call: %v", err)
	}
	hostname := pod.Spec.NodeSelector[corev1.LabelHostname]
	if err := AssignContainersNode(pod); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if pod.Spec.NodeSelector[corev1.LabelHostname] != hostname {
		t.Errorf("hostname pin changed across calls: %q -> %q", hostname, pod.Spec.NodeSelector[corev1.LabelHostname])
	}
}
