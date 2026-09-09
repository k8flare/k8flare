package apiserver_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var ioctxSignatures = []string{
	"Cannot perform I/O on behalf of a different request",
	"call to released function",
	"Network connection lost",
}

func TestIOContextProbe(t *testing.T) {
	if os.Getenv("K8FLARE_IOCTX_PROBE") != "1" {
		t.Skip("probe")
	}
	workers := 24
	if v := os.Getenv("K8FLARE_IOCTX_WORKERS"); v != "" {
		workers, _ = strconv.Atoi(v)
	}
	load := 150 * time.Second
	if v := os.Getenv("K8FLARE_IOCTX_SECONDS"); v != "" {
		n, _ := strconv.Atoi(v)
		load = time.Duration(n) * time.Second
	}
	logPath := os.Getenv("K8FLARE_IOCTX_LOG")
	if logPath == "" {
		logPath = "/tmp/ioctx-dev.log"
	}

	hostShape := os.Getenv("K8FLARE_IOCTX_MODE") != "dw"

	port := findFreePort(t)
	projectRoot := findProjectRoot(t)
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
	if drop := os.Getenv("K8FLARE_IOCTX_DROP_CLOSE"); drop != "" {
		args = append(args, "--var", "PUMP_WINDOW_DROP_CLOSE:"+drop)
	}
	scheme := "http"
	if hostShape {
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
	newClient := func() kubernetes.Interface {
		c, err := kubernetes.NewForConfig(&rest.Config{
			Host:            server,
			BearerToken:     "k8flare-dev-token",
			Timeout:         30 * time.Second,
			QPS:             -1,
			TLSClientConfig: rest.TLSClientConfig{Insecure: scheme == "https"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	client := newClient()
	ctx := context.Background()
	const ns = "ioctx"
	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	const node = "ioctx-node"
	if _, err := client.CoreV1().Nodes().Create(ctx, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: node, Labels: map[string]string{corev1.LabelHostname: node}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	kubelet := startFakeKubelet(ctx, client, node)
	defer kubelet.Stop()
	if hostShape {
		startHostControllerManager(t, projectRoot, server, logFile)
	} else {
		replicas := int32(4)
		if _, err := client.AppsV1().Deployments(ns).Create(ctx, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "ioctx-web"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "ioctx-web"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "ioctx-web"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "nginx", Image: "nginx:1.27"}},
					},
				},
			},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create deployment: %v", err)
		}
		waitFor(t, 5*time.Minute, "controllers loaded and created pods", func() bool {
			pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
				LabelSelector: "app=ioctx-web",
			})
			return err == nil && len(pods.Items) == int(replicas)
		})
	}

	for i := 0; i < workers; i++ {
		if _, err := client.CoreV1().Pods(ns).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   fmt.Sprintf("ioctx-%02d", i),
				Labels: map[string]string{"app": "ioctx"},
			},
			Spec: corev1.PodSpec{
				NodeName:   node,
				Containers: []corev1.Container{{Name: "c", Image: "registry.k8s.io/pause:3.10"}},
			},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create pod %d: %v", i, err)
		}
	}

	watchCtx, cancelWatch := context.WithCancel(ctx)
	for i := 0; i < 8; i++ {
		go func() {
			w, err := newClient().CoreV1().Pods(ns).Watch(watchCtx, metav1.ListOptions{})
			if err != nil {
				t.Logf("IOCTX watch open: %v", err)
				return
			}
			defer w.Stop()
			for range w.ResultChan() {
			}
		}()
	}

	var patches, posts, failures atomic.Int64
	var wg sync.WaitGroup
	stopAt := time.Now().Add(load)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := newClient()
			pod := fmt.Sprintf("ioctx-%02d", i)
			for round := 0; time.Now().Before(stopAt); round++ {
				patch := fmt.Sprintf(`{"metadata":{"annotations":{"ioctx/round":"%d"}}}`, round)
				if _, err := c.CoreV1().Pods(ns).Patch(ctx, pod, types.MergePatchType,
					[]byte(patch), metav1.PatchOptions{}); err != nil {
					failures.Add(1)
					t.Logf("IOCTX patch %s: %v", pod, err)
				} else {
					patches.Add(1)
				}
				if _, err := c.CoreV1().Events(ns).Create(ctx, &corev1.Event{
					ObjectMeta:     metav1.ObjectMeta{GenerateName: "ioctx-"},
					InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: ns, Name: pod},
					Reason:         "Probe",
					Message:        fmt.Sprintf("round %d", round),
					Type:           corev1.EventTypeNormal,
				}, metav1.CreateOptions{}); err != nil {
					failures.Add(1)
					t.Logf("IOCTX event %s: %v", pod, err)
				} else {
					posts.Add(1)
				}
			}
		}(i)
	}
	progress := time.NewTicker(30 * time.Second)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-progress.C:
				counts := scanIOContextLog(t, logPath)
				t.Logf("IOCTX patches=%d posts=%d failures=%d log=%v",
					patches.Load(), posts.Load(), failures.Load(), counts)
			}
		}
	}()
	time.Sleep(min(load/3, 40*time.Second))
	t.Logf("IOCTX dropping %d watch clients mid-stream", 8)
	cancelWatch()
	wg.Wait()
	close(done)
	progress.Stop()

	t.Logf("IOCTX touching resources no watch stream has seen for the whole run")
	for i := 0; i < 4; i++ {
		if _, err := client.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ioctx-cold-%d", i)},
			Data:       map[string]string{"k": "v"},
		}, metav1.CreateOptions{}); err != nil {
			t.Logf("IOCTX create configmap: %v", err)
		}
		if _, err := client.CoreV1().Secrets(ns).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ioctx-cold-%d", i)},
			StringData: map[string]string{"k": "v"},
		}, metav1.CreateOptions{}); err != nil {
			t.Logf("IOCTX create secret: %v", err)
		}
		time.Sleep(5 * time.Second)
	}
	time.Sleep(20 * time.Second)

	counts := scanIOContextLog(t, logPath)
	t.Logf("IOCTX RESULT patches=%d posts=%d client-failures=%d", patches.Load(), posts.Load(), failures.Load())
	for _, sig := range ioctxSignatures {
		t.Logf("IOCTX RESULT %q x%d", sig, counts[sig])
	}
	for _, sig := range ioctxSignatures[:2] {
		if counts[sig] > 0 {
			t.Errorf("IOCTX DEFECT REPRODUCED: %q appeared %d times in the dev log", sig, counts[sig])
		}
	}
}

func scanIOContextLog(t *testing.T, path string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	data, err := os.ReadFile(path)
	if err != nil {
		return counts
	}
	for _, sig := range ioctxSignatures {
		counts[sig] = strings.Count(string(data), sig)
	}
	return counts
}
