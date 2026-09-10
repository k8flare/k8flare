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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

func TestGCForegroundProbe(t *testing.T) {
	if os.Getenv("K8FLARE_GC_PROBE") != "1" {
		t.Skip("probe")
	}
	replicas := int32(10)
	if v := os.Getenv("K8FLARE_GC_REPLICAS"); v != "" {
		n, _ := strconv.Atoi(v)
		replicas = int32(n)
	}
	settle := 0 * time.Second
	if v := os.Getenv("K8FLARE_GC_SETTLE_SECONDS"); v != "" {
		n, _ := strconv.Atoi(v)
		settle = time.Duration(n) * time.Second
	}
	port := findFreePort(t)
	projectRoot := findProjectRoot(t)
	logPath := os.Getenv("K8FLARE_GC_LOG")
	if logPath == "" {
		logPath = "/tmp/gc-dev.log"
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
	}
	if drop := os.Getenv("K8FLARE_GC_DROP_CLOSE"); drop != "" {
		args = append(args, "--var", "PUMP_WINDOW_DROP_CLOSE:"+drop)
	}
	handMade := os.Getenv("K8FLARE_GC_HANDMADE") == "1"
	hostKCM := os.Getenv("K8FLARE_GC_HOST_KCM") == "1"
	scheme := "http"
	if handMade {
		args = append(args, "--var", "CM_DISABLED:1", "--var", "SCHED_DISABLED:1")
	}
	if hostKCM {
		certDir := t.TempDir()
		key, crt := certDir+"/dev.key", certDir+"/dev.crt"
		openssl := exec.Command("openssl", "req", "-x509", "-nodes", "-newkey", "rsa:2048",
			"-days", "1", "-keyout", key, "-out", crt,
			"-subj", "/CN=127.0.0.1", "-addext", "subjectAltName=IP:127.0.0.1,DNS:localhost")
		if out, err := openssl.CombinedOutput(); err != nil {
			t.Fatalf("generate dev cert: %v: %s", err, out)
		}
		args = append(args,
			"--var", "CM_DISABLED:1",
			"--var", "SCHED_DISABLED:1",
			"--local-protocol", "https",
			"--https-key-path", key,
			"--https-cert-path", crt)
		scheme = "https"
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

	server := fmt.Sprintf("%s://127.0.0.1:%d", scheme, port)
	client, err := kubernetes.NewForConfig(&rest.Config{
		Host:            server,
		BearerToken:     "k8flare-dev-token",
		Timeout:         30 * time.Second,
		TLSClientConfig: rest.TLSClientConfig{Insecure: scheme == "https"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hostKCM {
		startHostControllerManager(t, projectRoot, server, logFile)
	}
	ctx := context.Background()
	const ns = "gcfg"
	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	// CI's failing job has a real node heartbeating throughout, so the
	// Controllers DO is poked continuously and the gc dynamic worker's
	// pump windows churn for minutes before this test's rc exists. With
	// no node at all nothing pokes, no window ever closes, and the whole
	// probe runs inside the first window -- which is why it passed.
	if warm := os.Getenv("K8FLARE_GC_WARMUP_SECONDS"); warm != "" {
		const node = "gcfg-node"
		if _, err := client.CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: node, Labels: map[string]string{corev1.LabelHostname: node}},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		kubelet := startFakeKubelet(ctx, client, node)
		defer kubelet.Stop()
		n, _ := strconv.Atoi(warm)
		t.Logf("GCP warming up %ds with a heartbeating node before creating the rc", n)
		time.Sleep(time.Duration(n) * time.Second)
	}

	rc, err := client.CoreV1().ReplicationControllers(ns).Create(ctx, &corev1.ReplicationController{
		ObjectMeta: metav1.ObjectMeta{Name: "simpletest-rc"},
		Spec: corev1.ReplicationControllerSpec{
			Replicas: &replicas,
			Selector: map[string]string{"app": "gcfg"},
			Template: &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "gcfg"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "registry.k8s.io/pause:3.10"}},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// A kubelet keeps a deleted Pod visible for its grace period, so in CI
	// the GC's DELETE of a dependent does not remove it -- it goes
	// Terminating and stays a blocking dependent. There is no kubelet
	// here, so a finalizer stands in for that grace period.
	linger := os.Getenv("K8FLARE_GC_LINGER") == "1"
	if handMade {
		yes := true
		for i := 0; i < int(replicas); i++ {
			var finalizers []string
			if linger {
				finalizers = []string{"k8flare.test/linger"}
			}
			if _, err := client.CoreV1().Pods(ns).Create(ctx, &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:       fmt.Sprintf("simpletest-rc-%02d", i),
					Labels:     map[string]string{"app": "gcfg"},
					Finalizers: finalizers,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         "v1",
						Kind:               "ReplicationController",
						Name:               rc.Name,
						UID:                rc.UID,
						Controller:         &yes,
						BlockOwnerDeletion: &yes,
					}},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "registry.k8s.io/pause:3.10"}},
				},
			}, metav1.CreateOptions{}); err != nil {
				t.Fatalf("create hand-made pod %d: %v", i, err)
			}
		}
	}
	waitFor(t, 5*time.Minute, "rc created its pods", func() bool {
		pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return false
		}
		t.Logf("GCP pods=%d", len(pods.Items))
		return len(pods.Items) == int(replicas)
	})

	if settle > 0 {
		t.Logf("GCP settling %s before the foreground delete", settle)
		time.Sleep(settle)
	}

	rc, err = client.CoreV1().ReplicationControllers(ns).Get(ctx, "simpletest-rc", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fg := metav1.DeletePropagationForeground
	if err := client.CoreV1().ReplicationControllers(ns).Delete(ctx, "simpletest-rc", metav1.DeleteOptions{
		PropagationPolicy: &fg,
		Preconditions:     metav1.NewUIDPreconditions(string(rc.UID)),
	}); err != nil {
		t.Fatal(err)
	}
	deletedAt := time.Now()
	t.Logf("GCP deleted rc at %s", deletedAt.Format("15:04:05.000"))

	if linger {
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			for {
				select {
				case <-stop:
					return
				case <-time.After(2 * time.Second):
				}
				pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
				if err != nil {
					continue
				}
				for i := range pods.Items {
					p := pods.Items[i]
					if p.DeletionTimestamp == nil || time.Since(p.DeletionTimestamp.Time) < 25*time.Second {
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
	}

	podsAtRCGone := -1
	rcGoneAt := time.Time{}
	lastPods := int(replicas)
	for time.Since(deletedAt) < 5*time.Minute {
		pods, perr := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if perr == nil {
			lastPods = len(pods.Items)
		}
		cur, gerr := client.CoreV1().ReplicationControllers(ns).Get(ctx, "simpletest-rc", metav1.GetOptions{})
		if errors.IsNotFound(gerr) {
			rcGoneAt = time.Now()
			podsAtRCGone = lastPods
			break
		}
		if gerr == nil {
			t.Logf("GCP t+%4.1fs rc alive del=%v finalizers=%v pods=%d",
				time.Since(deletedAt).Seconds(), cur.DeletionTimestamp != nil, cur.Finalizers, lastPods)
		}
		time.Sleep(time.Second)
	}
	if podsAtRCGone < 0 {
		t.Fatalf("GCP RESULT: rc still present after 5m (pods=%d)", lastPods)
	}
	t.Logf("GCP RESULT: rc vanished %.1fs after delete with %d pods still present",
		rcGoneAt.Sub(deletedAt).Seconds(), podsAtRCGone)
	if podsAtRCGone != 0 {
		t.Errorf("GCP DEFECT REPRODUCED: foreground-deleted rc vanished with %d dependents still alive", podsAtRCGone)
	}
}
