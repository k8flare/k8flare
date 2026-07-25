package apiserver_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// TestKCMDynamicWorkerControlPlane is the LOCAL stand-in for the parts of
// e2e-conformance.yml's dw variants that don't need a Linux kubelet: it
// runs wrangler dev with the real KCM/GC/sched dynamic workers ENABLED
// (no KCM_DISABLED kill switch, unlike every other test in this package)
// and drives the poke-pump control plane end to end with real client-go.
//
// This is the regression gate for the 2026-07-25 prod-only bug class --
// WatchHub's reserved-close-code exception storm, the namespace-lifecycle
// admission gap, and the events sweep race were all invisible to `make
// test` because it runs with controllers disabled. What a Mac cannot
// check remains out of scope here: actual Pod scheduling/binding needs a
// Node (Linux kubelet), which stays the conformance CI's job.
//
// Opt-in via K8FLARE_KCM_TEST=1 (`make test-kcm`): it boots its own
// wrangler dev on a fresh port with a throwaway state dir, and the first
// poke compiles three ~40MB WASM modules inside workerd, so it is far
// slower than the rest of the suite.
func TestKCMDynamicWorkerControlPlane(t *testing.T) {
	if os.Getenv("K8FLARE_KCM_TEST") != "1" {
		t.Skip("KCM dynamic-worker smoke is opt-in: set K8FLARE_KCM_TEST=1 (make test-kcm)")
	}

	port := findFreePort(t)
	projectRoot := findProjectRoot(t)

	// Same flag rationale as setupWranglerDev (apiserver_test.go), minus
	// KCM_DISABLED, plus an isolated state dir so this run can't inherit
	// or clobber the main suite's persisted DO state.
	cmd := exec.Command("npx", "wrangler", "dev",
		"-c", "workers/k8flare/wrangler.jsonc",
		"--enable-containers=false",
		"--local",
		"--port", fmt.Sprintf("%d", port),
		"--persist-to", t.TempDir(),
		"--log-level", "error",
	)
	cmd.Dir = projectRoot
	devNull, _ := os.Open(os.DevNull)
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start wrangler dev: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		_, _ = cmd.Process.Wait()
	})

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			time.Sleep(2 * time.Second)
			break
		}
		time.Sleep(time.Second)
	}

	client, err := kubernetes.NewForConfig(&rest.Config{
		Host:        fmt.Sprintf("http://127.0.0.1:%d", port),
		BearerToken: "k8flare-dev-token",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	ctx := context.Background()

	const ns = "kcmdw-smoke"
	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	replicas := int32(2)
	if _, err := client.AppsV1().Deployments(ns).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "nginx", Image: "nginx:1.27"}},
				},
			},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create deployment: %v", err)
	}

	// The real KCM (deployment + replicaset controllers, loaded as a
	// dynamic worker by this write's poke) must produce 2 Pods. First
	// poke includes the in-workerd WASM compile, so the budget is
	// generous; a green run typically finishes in well under a minute.
	waitFor(t, 5*time.Minute, "KCM created 2 pods", func() bool {
		pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		return err == nil && len(pods.Items) == 2
	})

	// Namespace deletion must leave nothing behind, with the LIVE
	// controllers racing the sweep (admission + post-delete events
	// sweep regression: commits 9dfbb35 / a012a01).
	if err := client.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete namespace: %v", err)
	}
	waitFor(t, 2*time.Minute, "namespace fully deleted", func() bool {
		_, err := client.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		return err != nil
	})
	// Give the controllers a post-delete window to attempt any straggler
	// writes, then require zero leftovers.
	time.Sleep(30 * time.Second)
	pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list pods: %v", err)
	}
	if len(pods.Items) != 0 {
		t.Errorf("expected 0 pods cluster-wide after namespace delete, got %d", len(pods.Items))
	}
	events, err := client.CoreV1().Events(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	for _, ev := range events.Items {
		if ev.Namespace == ns {
			t.Errorf("orphan event survived namespace delete: %s/%s", ev.Namespace, ev.Name)
		}
	}
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("timed out after %s waiting for: %s", timeout, what)
}
