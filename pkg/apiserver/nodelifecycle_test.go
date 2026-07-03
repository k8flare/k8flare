package apiserver

import (
	"context"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func testNodesStore(storage *Storage) *ResourceStore {
	return NewResourceStore(storage, "nodes", false,
		func() runtime.Object { return &corev1.Node{} },
		func() runtime.Object { return &corev1.NodeList{} },
	)
}

func testLeasesStore(storage *Storage) *ResourceStore {
	return NewResourceStore(storage, "leases", true,
		func() runtime.Object { return &coordinationv1.Lease{} },
		func() runtime.Object { return &coordinationv1.LeaseList{} },
	)
}

// mustCreateLease creates node's Lease with renewTime set to now-age, the
// same shape kubelet maintains under the reserved "kube-node-lease"
// namespace (see reconcileOneNode's doc comment).
func mustCreateLease(t *testing.T, storage *Storage, nodeName string, age time.Duration) {
	t.Helper()
	renew := metav1.NewMicroTime(time.Now().Add(-age))
	_, err := testLeasesStore(storage).Create(context.Background(), corev1.NamespaceNodeLease, &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renew},
	})
	if err != nil {
		t.Fatalf("create lease for %s: %v", nodeName, err)
	}
}

// TestReconcileNodeLifecycle_FreshLeaseIsUntouched checks the common case
// (kubelet renewing on schedule) is a true no-op: no condition/taint churn
// on every alarm tick for a healthy cluster.
func TestReconcileNodeLifecycle_FreshLeaseIsUntouched(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()

	node := mustCreateNodeForLifecycleTest(t, storage, "node-fresh", corev1.ConditionTrue)
	mustCreateLease(t, storage, "node-fresh", 5*time.Second)

	if err := ReconcileNodeLifecycle(ctx, storage); err != nil {
		t.Fatalf("ReconcileNodeLifecycle: %v", err)
	}

	got := getTestNode(t, storage, "node-fresh")
	for _, c := range got.Status.Conditions {
		if c.Status == corev1.ConditionUnknown {
			t.Errorf("expected no Unknown conditions on a freshly-leased node, got %+v", got.Status.Conditions)
		}
	}
	if len(got.Spec.Taints) != 0 {
		t.Errorf("expected no taints on a freshly-leased node, got %+v", got.Spec.Taints)
	}
	_ = node
}

// TestReconcileNodeLifecycle_StaleLeaseMarksUnknownAndTaints is the core
// behavior restored from the deleted nodelifecycle.ts: a Lease older than
// the real upstream NodeMonitorGracePeriod (50s) flips Ready (and the other
// three tracked conditions) to Unknown and applies the unreachable
// NoExecute taint.
func TestReconcileNodeLifecycle_StaleLeaseMarksUnknownAndTaints(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()

	mustCreateNodeForLifecycleTest(t, storage, "node-stale", corev1.ConditionTrue)
	mustCreateLease(t, storage, "node-stale", nodeMonitorGracePeriod+10*time.Second)

	if err := ReconcileNodeLifecycle(ctx, storage); err != nil {
		t.Fatalf("ReconcileNodeLifecycle: %v", err)
	}

	got := getTestNode(t, storage, "node-stale")
	found := false
	for _, c := range got.Status.Conditions {
		if c.Type == corev1.NodeReady {
			found = true
			if c.Status != corev1.ConditionUnknown {
				t.Errorf("expected Ready=Unknown, got %s", c.Status)
			}
			if c.Reason != "NodeStatusUnknown" || c.Message != "Kubelet stopped posting node status." {
				t.Errorf("expected upstream's literal reason/message, got %q/%q", c.Reason, c.Message)
			}
		}
	}
	if !found {
		t.Fatal("expected a Ready condition to still be present")
	}

	hasUnreachable := false
	for _, taint := range got.Spec.Taints {
		if taint.Key == corev1.TaintNodeUnreachable && taint.Effect == corev1.TaintEffectNoExecute {
			hasUnreachable = true
		}
	}
	if !hasUnreachable {
		t.Errorf("expected node.kubernetes.io/unreachable:NoExecute taint, got %+v", got.Spec.Taints)
	}
}

// TestReconcileNodeLifecycle_SynthesizesMissingCondition covers a case the
// deleted nodelifecycle.ts never handled: a Node whose kubelet never
// reported a given condition type at all (not just "stale") gets one
// synthesized as Unknown, matching real
// node_lifecycle_controller.go's monitorNodeHealth.
func TestReconcileNodeLifecycle_SynthesizesMissingCondition(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()

	// No corev1.NodeCondition entries at all -- not even Ready.
	_, err := testNodesStore(storage).Create(ctx, "", &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-bare"}})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	mustCreateLease(t, storage, "node-bare", nodeMonitorGracePeriod+10*time.Second)

	if err := ReconcileNodeLifecycle(ctx, storage); err != nil {
		t.Fatalf("ReconcileNodeLifecycle: %v", err)
	}

	got := getTestNode(t, storage, "node-bare")
	if len(got.Status.Conditions) != 4 {
		t.Fatalf("expected all 4 tracked conditions to be synthesized, got %+v", got.Status.Conditions)
	}
	for _, c := range got.Status.Conditions {
		if c.Status != corev1.ConditionUnknown || c.Reason != "NodeStatusNeverUpdated" {
			t.Errorf("expected a synthesized Unknown/NodeStatusNeverUpdated condition, got %+v", c)
		}
	}
}

// TestReconcileNodeLifecycle_EvictsPodsPastEvictionTimeout checks the
// eviction half of the restored behavior: once stale beyond
// podEvictionTimeout (5m), Pods bound to that node are deleted -- unless
// they carry a forever toleration for the unreachable taint (the one
// addition beyond nodelifecycle.ts's original scope, protecting
// DaemonSet-style pods).
func TestReconcileNodeLifecycle_EvictsPodsPastEvictionTimeout(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()
	ns := "default"

	mustCreateNodeForLifecycleTest(t, storage, "node-gone", corev1.ConditionTrue)
	mustCreateLease(t, storage, "node-gone", podEvictionTimeout+10*time.Second)

	mustCreatePod(t, storage, ns, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "evictable"},
		Spec:       corev1.PodSpec{NodeName: "node-gone"},
	})
	mustCreatePod(t, storage, ns, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "daemon-like"},
		Spec: corev1.PodSpec{
			NodeName: "node-gone",
			Tolerations: []corev1.Toleration{{
				Key:      corev1.TaintNodeUnreachable,
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoExecute,
				// TolerationSeconds left nil: tolerate forever.
			}},
		},
	})

	if err := ReconcileNodeLifecycle(ctx, storage); err != nil {
		t.Fatalf("ReconcileNodeLifecycle: %v", err)
	}

	if _, err := podsResourceStore(storage).Get(ctx, ns, "evictable"); err == nil {
		t.Error("expected \"evictable\" to have been evicted")
	}
	if _, err := podsResourceStore(storage).Get(ctx, ns, "daemon-like"); err != nil {
		t.Errorf("expected \"daemon-like\" (forever toleration) to survive eviction, got: %v", err)
	}
}

// TestReconcileNodeLifecycle_NoLeaseIsNoop covers a Node that has
// registered but has no Lease yet -- must not be treated as stale.
func TestReconcileNodeLifecycle_NoLeaseIsNoop(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()

	mustCreateNodeForLifecycleTest(t, storage, "node-no-lease", corev1.ConditionTrue)

	if err := ReconcileNodeLifecycle(ctx, storage); err != nil {
		t.Fatalf("ReconcileNodeLifecycle: %v", err)
	}

	got := getTestNode(t, storage, "node-no-lease")
	for _, c := range got.Status.Conditions {
		if c.Status == corev1.ConditionUnknown {
			t.Errorf("expected no change for a Node with no Lease yet, got %+v", got.Status.Conditions)
		}
	}
}

func mustCreateNodeForLifecycleTest(t *testing.T, storage *Storage, name string, readyStatus corev1.ConditionStatus) *corev1.Node {
	t.Helper()
	obj, err := testNodesStore(storage).Create(context.Background(), "", &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: readyStatus}},
		},
	})
	if err != nil {
		t.Fatalf("create node %s: %v", name, err)
	}
	return obj.(*corev1.Node)
}

func getTestNode(t *testing.T, storage *Storage, name string) *corev1.Node {
	t.Helper()
	obj, err := testNodesStore(storage).Get(context.Background(), "", name)
	if err != nil {
		t.Fatalf("get node %s: %v", name, err)
	}
	return obj.(*corev1.Node)
}
