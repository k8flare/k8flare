package apiserver_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

// TestGCMultiOwnerForegroundProbe is the local stand-in for the upstream
// conformance test "[sig-api-machinery] Garbage collector should not
// delete dependents that have both valid owner and owner that's waiting
// for dependents to be deleted", which fails intermittently in
// e2e-conformance's dw variants with `failed to delete rc
// simpletest-rc-to-be-deleted, err: context deadline exceeded`
// (garbage_collector.go:795 -- 90s of polling and the rc is still
// there). See docs/platform-verification.md S36.
//
// K8FLARE_GCMO_VARIANT picks the CI shape: kcm-dw (kcm + gc resident,
// the failing one), sched-dw (sched + gc), host (gc alone, the required
// variant that passes). K8FLARE_GCMO_RUNS repeats the measurement in
// fresh namespaces against the same instance, since what is under test
// is how the platform behaves long after load.
func TestGCMultiOwnerForegroundProbe(t *testing.T) {
	if os.Getenv("K8FLARE_GCMO_PROBE") != "1" {
		t.Skip("probe")
	}
	replicas := envInt("K8FLARE_GCMO_REPLICAS", 25)
	warmup := time.Duration(envInt("K8FLARE_GCMO_WARMUP_SECONDS", 240)) * time.Second
	runs := envInt("K8FLARE_GCMO_RUNS", 5)
	budget := time.Duration(envInt("K8FLARE_GCMO_BUDGET_SECONDS", 90)) * time.Second
	grace := time.Duration(envInt("K8FLARE_GCMO_GRACE_SECONDS", 0)) * time.Second

	port := findFreePort(t)
	projectRoot := findProjectRoot(t)
	logPath := os.Getenv("K8FLARE_GCMO_LOG")
	if logPath == "" {
		logPath = "/tmp/gcmo-dev.log"
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"wrangler", "dev",
		"-c", "packages/k8flare-worker/wrangler.jsonc",
		"--enable-containers=false",
		"--local",
		"--port", fmt.Sprintf("%d", port),
		"--persist-to", t.TempDir(),
		"--log-level", "info",
	}
	variant := os.Getenv("K8FLARE_GCMO_VARIANT")
	if variant == "" {
		variant = "kcm-dw"
	}
	switch variant {
	case "kcm-dw":
		args = append(args, "--var", "SCHED_DISABLED:1", "--var", "CM_DISABLED:0")
	case "sched-dw":
		args = append(args, "--var", "SCHED_DISABLED:0", "--var", "CM_DISABLED:1")
	case "host":
		args = append(args, "--var", "SCHED_DISABLED:1", "--var", "CM_DISABLED:1")
	default:
		t.Fatalf("unknown variant %q", variant)
	}
	if drop := os.Getenv("K8FLARE_GCMO_DROP_CLOSE"); drop != "" {
		args = append(args, "--var", "PUMP_WINDOW_DROP_CLOSE:"+drop)
	}
	if v := os.Getenv("K8FLARE_GCMO_GC_VERBOSITY"); v != "" {
		args = append(args, "--var", "GC_VERBOSITY:"+v)
	}
	cmd := exec.Command("npx", args...)
	cmd.Dir = projectRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start wrangler dev: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		_, _ = cmd.Process.Wait()
		logFile.Close()
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

	server := fmt.Sprintf("http://127.0.0.1:%d", port)
	client, err := kubernetes.NewForConfig(&rest.Config{
		Host:        server,
		BearerToken: "k8flare-dev-token",
		Timeout:     30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if variant == "host" || variant == "sched-dw" {
		startHostControllerManager(t, projectRoot, server, logFile)
	}

	// CI always has a real kubelet heartbeating, which is the only thing
	// that keeps poking the Controllers DO once the workload settles.
	if os.Getenv("K8FLARE_GCMO_NODE") != "0" {
		const node = "gcmo-node"
		if _, err := client.CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: node, Labels: map[string]string{corev1.LabelHostname: node}},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		kubelet := startFakeKubelet(ctx, client, node)
		defer kubelet.Stop()
	}

	if warmup > 0 {
		t.Logf("GCMO variant=%s replicas=%d: warming up %s", variant, replicas, warmup)
		time.Sleep(warmup)
	}

	type result struct {
		firstDependentDeletion time.Duration
		rcGone                 time.Duration
		timedOut               bool
	}
	var results []result
	for run := 0; run < runs; run++ {
		ns := fmt.Sprintf("gcmo-%d", run)
		if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		rc1 := newProbeOwnerRC(t, ctx, client, ns, "simpletest-rc-to-be-deleted", int32(replicas), "gctest_d")
		rc2 := newProbeOwnerRC(t, ctx, client, ns, "simpletest-rc-to-stay", 0, "gctest_s")
		if grace > 0 {
			stopGrace := releasePodsAfterGrace(ctx, client, ns, grace)
			defer stopGrace()
		}

		waitFor(t, 4*time.Minute, fmt.Sprintf("run %d: rc1 created %d pods", run, replicas), func() bool {
			cur, err := client.CoreV1().ReplicationControllers(ns).Get(ctx, rc1.Name, metav1.GetOptions{})
			return err == nil && cur.Status.Replicas == int32(replicas)
		})
		pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		patch := fmt.Sprintf(`{"metadata":{"ownerReferences":[{"apiVersion":"v1","kind":"ReplicationController","name":%q,"uid":%q}]}}`,
			rc2.Name, rc2.UID)
		for i := 0; i < replicas/2; i++ {
			if _, err := client.CoreV1().Pods(ns).Patch(ctx, pods.Items[i].Name,
				types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
				t.Fatalf("run %d: patch pod %s: %v", run, pods.Items[i].Name, err)
			}
		}

		fg := metav1.DeletePropagationForeground
		if err := client.CoreV1().ReplicationControllers(ns).Delete(ctx, rc1.Name, metav1.DeleteOptions{
			PropagationPolicy: &fg,
			Preconditions:     metav1.NewUIDPreconditions(string(rc1.UID)),
		}); err != nil {
			t.Fatalf("run %d: delete rc1: %v", run, err)
		}
		deletedAt := time.Now()
		t.Logf("GCMO run %d: deleted rc1 at %s", run, deletedAt.Format("15:04:05.000"))

		r := result{firstDependentDeletion: -1, rcGone: -1}
		for time.Since(deletedAt) < budget {
			list, lerr := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
			terminating := 0
			if lerr == nil {
				for i := range list.Items {
					if list.Items[i].DeletionTimestamp != nil {
						terminating++
					}
				}
			}
			if terminating > 0 && r.firstDependentDeletion < 0 {
				r.firstDependentDeletion = time.Since(deletedAt)
			}
			_, gerr := client.CoreV1().ReplicationControllers(ns).Get(ctx, rc1.Name, metav1.GetOptions{})
			if errors.IsNotFound(gerr) {
				r.rcGone = time.Since(deletedAt)
				break
			}
			t.Logf("GCMO run %d t+%5.1fs rc1 alive pods=%d terminating=%d",
				run, time.Since(deletedAt).Seconds(), len(list.Items), terminating)
			time.Sleep(2 * time.Second)
		}
		if r.rcGone < 0 {
			r.timedOut = true
			t.Errorf("GCMO run %d: rc1 still present after %s (first dependent deletion at %v)",
				run, budget, r.firstDependentDeletion)
		} else {
			t.Logf("GCMO run %d RESULT: first dependent deletion %v, rc1 gone %v",
				run, r.firstDependentDeletion.Round(time.Millisecond*100), r.rcGone.Round(time.Millisecond*100))
		}
		results = append(results, r)
		_ = client.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
	}

	t.Logf("GCMO DISTRIBUTION variant=%s replicas=%d warmup=%s", variant, replicas, warmup)
	for i, r := range results {
		t.Logf("GCMO  run %d: firstDependentDeletion=%v rcGone=%v timedOut=%v",
			i, r.firstDependentDeletion.Round(100*time.Millisecond), r.rcGone.Round(100*time.Millisecond), r.timedOut)
	}
}

func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// releasePodsAfterGrace stands in for a real kubelet's termination grace
// period: CI's pods keep their deletionTimestamp (and therefore keep
// blocking their owner) until the kubelet confirms the containers are
// gone, while a nodeless dev cluster removes them the instant the GC
// issues the DELETE.
func releasePodsAfterGrace(ctx context.Context, client kubernetes.Interface, ns string, grace time.Duration) func() {
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(time.Second):
			}
			pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
			if err != nil {
				continue
			}
			for i := range pods.Items {
				p := &pods.Items[i]
				if p.DeletionTimestamp == nil || len(p.Finalizers) == 0 {
					continue
				}
				if time.Since(p.DeletionTimestamp.Time) < grace {
					continue
				}
				_ = retry.RetryOnConflict(retry.DefaultRetry, func() error {
					cur, err := client.CoreV1().Pods(ns).Get(ctx, p.Name, metav1.GetOptions{})
					if err != nil {
						return nil
					}
					cur.Finalizers = nil
					_, err = client.CoreV1().Pods(ns).Update(ctx, cur, metav1.UpdateOptions{})
					return err
				})
			}
		}
	}()
	stopped := false
	return func() {
		if stopped {
			return
		}
		stopped = true
		close(stop)
		<-done
	}
}

func newProbeOwnerRC(t *testing.T, ctx context.Context, client kubernetes.Interface, ns, name string, replicas int32, label string) *corev1.ReplicationController {
	t.Helper()
	labels := map[string]string{label: name}
	var finalizers []string
	if envInt("K8FLARE_GCMO_GRACE_SECONDS", 0) > 0 {
		finalizers = []string{"k8flare.test/grace"}
	}
	rc, err := client.CoreV1().ReplicationControllers(ns).Create(ctx, &corev1.ReplicationController{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.ReplicationControllerSpec{
			Replicas: &replicas,
			Selector: labels,
			Template: &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels, Finalizers: finalizers},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: new(int64),
					Containers: []corev1.Container{{
						Name:  "c",
						Image: "registry.k8s.io/pause:3.10",
					}},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create rc %s: %v", name, err)
	}
	return rc
}
