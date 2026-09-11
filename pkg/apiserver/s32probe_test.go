package apiserver_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

// fakeKubelet imitates the parts of a real kubelet the node-lifecycle and
// workload controllers read: a 10s Lease renewal + Node status heartbeat,
// Running/Ready status on any Pod bound to this node, and actually
// finalizing Pods that were deleted (a terminating Pod nobody removes
// keeps a Deployment permanently unconverged, which production did not
// have).
type fakeKubelet struct {
	client kubernetes.Interface
	node   string
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

func startFakeKubelet(ctx context.Context, client kubernetes.Interface, node string) *fakeKubelet {
	k := &fakeKubelet{
		client: client, node: node,
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	k.tick(ctx)
	go func() {
		defer close(k.done)
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-k.stop:
				return
			case <-t.C:
				k.tick(ctx)
			}
		}
	}()
	return k
}

func (k *fakeKubelet) Stop() {
	k.once.Do(func() {
		close(k.stop)
		<-k.done
	})
}

func (k *fakeKubelet) tick(ctx context.Context) {
	now := metav1.NewMicroTime(time.Now())
	leases := k.client.CoordinationV1().Leases(corev1.NamespaceNodeLease)
	if lease, err := leases.Get(ctx, k.node, metav1.GetOptions{}); err == nil {
		lease.Spec.RenewTime = &now
		_, _ = leases.Update(ctx, lease, metav1.UpdateOptions{})
	} else {
		d := int32(40)
		_, _ = leases.Create(ctx, &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: k.node},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity: &k.node, LeaseDurationSeconds: &d, RenewTime: &now,
			},
		}, metav1.CreateOptions{})
	}
	_ = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := k.client.CoreV1().Nodes().Get(ctx, k.node, metav1.GetOptions{})
		if err != nil {
			return err
		}
		ts := metav1.NewTime(time.Now())
		node.Status.Capacity = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
			corev1.ResourcePods:   resource.MustParse("110"),
		}
		node.Status.Allocatable = node.Status.Capacity
		node.Status.Conditions = []corev1.NodeCondition{{
			Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: "KubeletReady",
			LastHeartbeatTime: ts, LastTransitionTime: ts,
		}}
		_, err = k.client.CoreV1().Nodes().UpdateStatus(ctx, node, metav1.UpdateOptions{})
		return err
	})
	pods, err := k.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	for i := range pods.Items {
		pod := pods.Items[i]
		if pod.Spec.NodeName != k.node {
			continue
		}
		if pod.DeletionTimestamp != nil {
			zero := int64(0)
			_ = k.client.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name,
				metav1.DeleteOptions{GracePeriodSeconds: &zero})
			continue
		}
		if pod.Status.Phase == corev1.PodRunning && podReady(&pod) {
			continue
		}
		_ = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			p, err := k.client.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			ts := metav1.NewTime(time.Now())
			p.Status.Phase = corev1.PodRunning
			p.Status.PodIP = "10.42.9.9"
			p.Status.PodIPs = []corev1.PodIP{{IP: "10.42.9.9"}}
			p.Status.Conditions = []corev1.PodCondition{{
				Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: ts,
			}}
			_, err = k.client.CoreV1().Pods(p.Namespace).UpdateStatus(ctx, p, metav1.UpdateOptions{})
			return err
		})
	}
}

func podReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func TestS32WriteStormProbe(t *testing.T) {
	if os.Getenv("K8FLARE_S32_PROBE") != "1" {
		t.Skip("probe")
	}
	downFor := 10 * time.Minute
	if v := os.Getenv("K8FLARE_S32_DOWN_SECONDS"); v != "" {
		n, _ := strconv.Atoi(v)
		downFor = time.Duration(n) * time.Second
	}
	port := findFreePort(t)
	projectRoot := findProjectRoot(t)
	logPath := os.Getenv("K8FLARE_S32_LOG")
	if logPath == "" {
		logPath = "/tmp/s32-dev.log"
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	hostKCM := os.Getenv("K8FLARE_S32_HOST_KCM") == "1"
	scheme := "http"
	args := []string{"wrangler", "dev",
		"-c", "packages/k8flare-worker/wrangler.jsonc",
		"--enable-containers=false",
		"--local",
		"--port", fmt.Sprintf("%d", port),
		"--persist-to", t.TempDir(),
	}
	if drop := os.Getenv("K8FLARE_S32_DROP_CLOSE"); drop != "" {
		args = append(args, "--var", "PUMP_WINDOW_DROP_CLOSE:"+drop)
	}
	if hostKCM {
		// Same split e2e-conformance.yml's `host` variant runs: the kcm
		// dynamic worker unloaded, the workload controllers served by the
		// real host kube-controller-manager process, and TLS terminated
		// because clientcmd refuses to send a bearer token over plain
		// HTTP (CLAUDE.md's local-development pitfalls).
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
	const ns = "s32"
	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	// Two nodes: with only one, nodelifecycle enters full-disruption mode
	// and stops evicting (S31 addendum).
	const nodeA, nodeB = "s32-a", "s32-b"
	for _, n := range []string{nodeA, nodeB} {
		if _, err := client.CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: n, Labels: map[string]string{corev1.LabelHostname: n}},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	kubeletA := startFakeKubelet(ctx, client, nodeA)
	defer kubeletA.Stop()
	kubeletB := startFakeKubelet(ctx, client, nodeB)
	defer kubeletB.Stop()

	replicas := int32(2)
	if _, err := client.AppsV1().Deployments(ns).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "s32-probe"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "s32"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "s32"}},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{corev1.LabelHostname: nodeB},
					Containers:   []corev1.Container{{Name: "c", Image: "nginx:1.27"}},
				},
			},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	// Host-KCM mode runs no scheduler at all (SCHED_DISABLED, and
	// cmd/scheduler is not started): nothing binds these Pods, so they
	// stay Pending. That is fine for what this mode measures -- taint
	// removal latency, which only involves nodelifecycle.
	waitFor(t, 4*time.Minute, "deployment converged", func() bool {
		d, err := client.AppsV1().Deployments(ns).Get(ctx, "s32-probe", metav1.GetOptions{})
		if err != nil {
			return false
		}
		if hostKCM {
			return d.Status.Replicas == 2
		}
		return d.Status.AvailableReplicas == 2
	})

	report := func(label string) {
		d, err := client.AppsV1().Deployments(ns).Get(ctx, "s32-probe", metav1.GetOptions{})
		if err != nil {
			t.Logf("%s: get deployment: %v", label, err)
			return
		}
		t.Logf("S32 %s t=%s deploy rv=%s repl=%d/upd=%d/ready=%d/avail=%d/unavail=%d",
			label, time.Now().Format("15:04:05"), d.ResourceVersion,
			d.Status.Replicas, d.Status.UpdatedReplicas, d.Status.ReadyReplicas,
			d.Status.AvailableReplicas, d.Status.UnavailableReplicas)
		for _, c := range d.Status.Conditions {
			t.Logf("S32 %s   cond %s=%s reason=%s upd=%s trans=%s",
				label, c.Type, c.Status, c.Reason,
				c.LastUpdateTime.Format("15:04:05"), c.LastTransitionTime.Format("15:04:05"))
		}
		n, err := client.CoreV1().Nodes().Get(ctx, nodeB, metav1.GetOptions{})
		if err == nil {
			t.Logf("S32 %s   node %s rv=%s taints=%v", label, nodeB, n.ResourceVersion, taintKeys(n))
		}
		pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err == nil {
			for i := range pods.Items {
				p := pods.Items[i]
				t.Logf("S32 %s   pod %s node=%s phase=%s ready=%v del=%v",
					label, p.Name, p.Spec.NodeName, p.Status.Phase, podReady(&p), p.DeletionTimestamp != nil)
			}
		}
	}

	report("steady")
	for i := 0; i < 4; i++ {
		time.Sleep(15 * time.Second)
		report(fmt.Sprintf("quiet+%ds", (i+1)*15))
	}

	// Node goes away: no Lease renewal, no status heartbeat, no pod
	// finalization -- exactly what `docker pause` did in production.
	kubeletB.Stop()
	t.Logf("S32 PHASE stop-kubelet at %s", time.Now().Format("15:04:05"))
	waitFor(t, 3*time.Minute, "nodeB unreachable", func() bool {
		n, err := client.CoreV1().Nodes().Get(ctx, nodeB, metav1.GetOptions{})
		return err == nil && hasTaint(n, corev1.TaintNodeUnreachable)
	})
	report("unreachable")
	downDeadline := time.Now().Add(downFor)
	for i := 0; time.Now().Before(downDeadline); i++ {
		time.Sleep(30 * time.Second)
		report(fmt.Sprintf("down+%ds", (i+1)*30))
	}

	t.Logf("S32 PHASE recover at %s", time.Now().Format("15:04:05"))
	recoverAt := time.Now()
	kubeletB2 := startFakeKubelet(ctx, client, nodeB)
	defer kubeletB2.Stop()
	untaintAt := time.Time{}
	for i := 0; i < 60; i++ {
		time.Sleep(10 * time.Second)
		report(fmt.Sprintf("recover+%ds", (i+1)*10))
		n, err := client.CoreV1().Nodes().Get(ctx, nodeB, metav1.GetOptions{})
		if err == nil && !hasTaint(n, corev1.TaintNodeUnreachable) && !hasTaint(n, corev1.TaintNodeNotReady) {
			untaintAt = time.Now()
			break
		}
	}
	if untaintAt.IsZero() {
		t.Logf("S32 RESULT untaint: NEVER within %s", time.Since(recoverAt).Round(time.Second))
	} else {
		t.Logf("S32 RESULT untaint after %s", untaintAt.Sub(recoverAt).Round(time.Second))
	}
	// A window of quiet after recovery, to see whether writes keep coming.
	for i := 0; i < 6; i++ {
		time.Sleep(15 * time.Second)
		report(fmt.Sprintf("after+%ds", (i+1)*15))
	}
	t.Logf("S32 PHASE end at %s", time.Now().Format("15:04:05"))
}

func startHostControllerManager(t *testing.T, projectRoot, server string, log *os.File) {
	t.Helper()
	bin := t.TempDir() + "/k8flare-controller-manager"
	build := exec.Command("go", "build", "-o", bin, "./cmd/controller-manager/")
	build.Dir = projectRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build controller-manager: %v: %s", err, out)
	}
	cm := exec.Command(bin,
		"--server="+server,
		"--token=k8flare-dev-token",
		"--data-dir="+t.TempDir(),
		"--insecure-skip-tls-verify")
	cm.Dir = projectRoot
	cm.Stdout = log
	cm.Stderr = log
	cm.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cm.Start(); err != nil {
		t.Fatalf("start controller-manager: %v", err)
	}
	t.Cleanup(func() {
		// SIGKILL, not SIGTERM: the real kube-controller-manager keeps
		// running after a group SIGTERM here and Wait then never returns
		// (measured 2026-09-10, the probe hung for 10 minutes past its
		// last phase).
		_ = syscall.Kill(-cm.Process.Pid, syscall.SIGKILL)
		_, _ = cm.Process.Wait()
	})
	time.Sleep(5 * time.Second)
	if cm.ProcessState != nil && cm.ProcessState.Exited() {
		t.Fatalf("controller-manager exited immediately; see the probe log")
	}
}

func taintKeys(n *corev1.Node) []string {
	out := []string{}
	for _, t := range n.Spec.Taints {
		out = append(out, fmt.Sprintf("%s:%s", t.Key, t.Effect))
	}
	return out
}
