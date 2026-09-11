package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	coordv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// TestSmokeAgainstDev exercises the probe itself against a `wrangler dev`
// instance started by smoke.sh. It is NOT a test of production: dev cannot
// reproduce the runtime semantics S30 broke, and there is no kubelet here,
// so the pod is driven to Running by fakeKubelet below.
func TestSmokeAgainstDev(t *testing.T) {
	if os.Getenv("K8FLARE_PROBE_SMOKE") != "1" {
		t.Skip("set K8FLARE_PROBE_SMOKE=1 and run cmd/prodprobe/smoke.sh")
	}
	url := os.Getenv("K8FLARE_PROBE_URL")
	token := os.Getenv("K8FLARE_PROBE_TOKEN")
	if url == "" || token == "" {
		t.Fatal("K8FLARE_PROBE_URL and K8FLARE_PROBE_TOKEN must point at the dev server")
	}

	cfg := config{
		url:            strings.TrimSuffix(url, "/"),
		token:          token,
		name:           "k8flare-prodprobe-smoke",
		ns:             "default",
		image:          "registry.k8s.io/pause:3.10",
		compute:        "",
		runningTimeout: 3 * time.Minute,
		deleteTimeout:  3 * time.Minute,
		pollInterval:   time.Second,
		parking:        false,
	}
	cs, err := newClientset(cfg.url, cfg.token)
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	t.Run("probe fails when no pod can run", func(t *testing.T) {
		short := cfg
		short.runningTimeout = 20 * time.Second
		err := run(context.Background(), short, testWriter{t})
		if err == nil {
			t.Fatal("want a failure: nothing can schedule a pod on a node-less dev cluster")
		}
		if !strings.Contains(err.Error(), "reached Running") {
			t.Fatalf("want the pod-never-ran failure, got: %v", err)
		}
		t.Logf("expected failure: %v", err)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nodeName := "prodprobe-smoke-node"
	if err := createFakeNode(ctx, cs, nodeName); err != nil {
		t.Fatalf("create fake node: %v", err)
	}
	t.Cleanup(func() {
		_ = cs.CoreV1().Nodes().Delete(context.Background(), nodeName, metav1.DeleteOptions{})
	})
	go fakeKubelet(ctx, cs, nodeName, cfg.ns, "app="+cfg.name)

	t.Run("probe passes end to end", func(t *testing.T) {
		if err := run(context.Background(), cfg, testWriter{t}); err != nil {
			t.Fatalf("probe failed: %v", err)
		}
	})
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func createFakeNode(ctx context.Context, cs kubernetes.Interface, name string) error {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"kubernetes.io/hostname": name},
		},
	}
	if _, err := cs.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	capacity := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("2"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
		corev1.ResourcePods:   resource.MustParse("110"),
	}
	fresh, err := cs.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	fresh.Status.Capacity = capacity
	fresh.Status.Allocatable = capacity
	fresh.Status.Conditions = []corev1.NodeCondition{{
		Type:              corev1.NodeReady,
		Status:            corev1.ConditionTrue,
		LastHeartbeatTime: metav1.Now(),
		Reason:            "KubeletReady",
	}}
	_, err = cs.CoreV1().Nodes().UpdateStatus(ctx, fresh, metav1.UpdateOptions{})
	return err
}

// fakeKubelet stands in for the one thing wrangler dev has no way to run:
// something that turns a bound pod into a Running one, renews the node
// lease so nodelifecycle does not taint the node out from under the
// scheduler, and reaps pods once they are terminating.
func fakeKubelet(ctx context.Context, cs kubernetes.Interface, nodeName, ns, selector string) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	lastLease := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if time.Since(lastLease) > 5*time.Second {
			renewLease(ctx, cs, nodeName)
			lastLease = time.Now()
		}
		pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			continue
		}
		for i := range pods.Items {
			p := &pods.Items[i]
			if p.DeletionTimestamp != nil {
				grace := int64(0)
				_ = cs.CoreV1().Pods(ns).Delete(ctx, p.Name, metav1.DeleteOptions{GracePeriodSeconds: &grace})
				continue
			}
			if p.Spec.NodeName == "" || p.Status.Phase == corev1.PodRunning {
				continue
			}
			p.Status.Phase = corev1.PodRunning
			p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
			_, _ = cs.CoreV1().Pods(ns).UpdateStatus(ctx, p, metav1.UpdateOptions{})
		}
	}
}

func renewLease(ctx context.Context, cs kubernetes.Interface, nodeName string) {
	const ns = "kube-node-lease"
	now := metav1.NewMicroTime(time.Now())
	duration := int32(40)
	lease, err := cs.CoordinationV1().Leases(ns).Get(ctx, nodeName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, _ = cs.CoordinationV1().Leases(ns).Create(ctx, &coordv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName, Namespace: ns},
			Spec: coordv1.LeaseSpec{
				HolderIdentity:       &nodeName,
				LeaseDurationSeconds: &duration,
				RenewTime:            &now,
			},
		}, metav1.CreateOptions{})
		return
	}
	if err != nil {
		return
	}
	lease.Spec.RenewTime = &now
	_, _ = cs.CoordinationV1().Leases(ns).Update(ctx, lease, metav1.UpdateOptions{})
}
