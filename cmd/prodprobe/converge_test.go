package main

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func pod(name string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels:    map[string]string{"app": "k8flare-prodprobe"},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
}

func TestWaitForRunningPod(t *testing.T) {
	cs := fake.NewSimpleClientset(pod("p1", corev1.PodRunning))
	name, err := waitForRunningPod(context.Background(), cs, "default", "app=k8flare-prodprobe", time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "p1" {
		t.Fatalf("got pod %q", name)
	}
}

func TestWaitForRunningPodTimesOutWithPodState(t *testing.T) {
	pending := pod("p1", corev1.PodPending)
	pending.Status.Conditions = []corev1.PodCondition{{
		Type:    corev1.PodScheduled,
		Status:  corev1.ConditionFalse,
		Reason:  "Unschedulable",
		Message: "no nodes available",
	}}
	cs := fake.NewSimpleClientset(pending)
	_, err := waitForRunningPod(context.Background(), cs, "default", "app=k8flare-prodprobe", 100*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Fatal("want a timeout error, got nil")
	}
	for _, want := range []string{"reached Running", "p1=Pending", "Unschedulable", "no nodes available"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

func TestWaitForPodsGone(t *testing.T) {
	cs := fake.NewSimpleClientset()
	if err := waitForPodsGone(context.Background(), cs, "default", "app=k8flare-prodprobe", time.Second, 10*time.Millisecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cs = fake.NewSimpleClientset(pod("p1", corev1.PodRunning))
	err := waitForPodsGone(context.Background(), cs, "default", "app=k8flare-prodprobe", 100*time.Millisecond, 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "still exist") {
		t.Fatalf("want a still-exist error, got %v", err)
	}
}

func TestProbeDeploymentCarriesComputeClass(t *testing.T) {
	d := probeDeployment("probe", "registry.k8s.io/pause:3.10", "containers")
	if got := d.Spec.Template.Annotations[computeClassAnnotation]; got != "containers" {
		t.Fatalf("compute annotation was %q", got)
	}
	if d.Spec.Selector.MatchLabels["app"] != "probe" {
		t.Fatalf("selector was %+v", d.Spec.Selector)
	}

	d = probeDeployment("probe", "img", "")
	if _, ok := d.Spec.Template.Annotations[computeClassAnnotation]; ok {
		t.Fatal("empty -compute must not annotate the pod template")
	}
}

func TestAssertParkedRefusesWithNodesAttached(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "byo-1"}})
	cfg := config{parking: true, quietWindow: time.Minute, scriptName: "k8flare"}
	err := assertParked(context.Background(), cfg, cs, time.Now(), time.Now(), func(string, ...any) {})
	if err == nil || !strings.Contains(err.Error(), "byo-1") {
		t.Fatalf("want a refusal naming the attached node, got %v", err)
	}
}

func TestEvaluateParking(t *testing.T) {
	active := invocations{WorkerRequests: 120, DORequests: 90}

	if err := evaluateParking(active, invocations{}, "k8flare", time.Minute); err != nil {
		t.Fatalf("a quiet window after a busy one must pass: %v", err)
	}

	err := evaluateParking(invocations{}, invocations{}, "k8flare", time.Minute)
	if err == nil || !strings.Contains(err.Error(), "instrument check") {
		t.Fatalf("an active window measured as zero must be an instrument error, got %v", err)
	}

	err = evaluateParking(active, invocations{DORequests: 7}, "k8flare", time.Minute)
	if err == nil || !strings.Contains(err.Error(), "did not park") {
		t.Fatalf("DO traffic in the quiet window must fail, got %v", err)
	}

	err = evaluateParking(active, invocations{WorkerRequests: 1}, "k8flare", time.Minute)
	if err == nil || !strings.Contains(err.Error(), "did not park") {
		t.Fatalf("worker traffic in the quiet window must fail, got %v", err)
	}
}

func TestParseFlagsRequiresCredentialsForParking(t *testing.T) {
	t.Setenv("K8FLARE_PROBE_URL", "")
	t.Setenv("K8FLARE_PROBE_TOKEN", "")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "")
	t.Setenv("CLOUDFLARE_API_TOKEN", "")

	_, err := parseFlags([]string{"-url", "https://example.test", "-token", "t"})
	if err == nil || !strings.Contains(err.Error(), "-parking needs") {
		t.Fatalf("parking must refuse to run without analytics credentials, got %v", err)
	}

	cfg, err := parseFlags([]string{"-url", "https://example.test/", "-token", "t", "-parking=false"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.url != "https://example.test" {
		t.Fatalf("trailing slash not trimmed: %q", cfg.url)
	}
}
