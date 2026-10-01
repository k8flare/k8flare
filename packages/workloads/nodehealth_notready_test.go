package workloads

import (
	"context"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func leaseRenewedAt(name string, at time.Time) *coordinationv1.Lease {
	renew := metav1.NewMicroTime(at)
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: v1.NamespaceNodeLease},
		Spec: coordinationv1.LeaseSpec{
			RenewTime:            &renew,
			LeaseDurationSeconds: ptr.To[int32](40),
		},
	}
}

func TestNodeHealthTaintsAHeartbeatingNodeThatReportsNotReady(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Spec: v1.NodeSpec{Taints: []v1.Taint{
			{Key: v1.TaintNodeUnreachable, Effect: v1.TaintEffectNoExecute},
		}},
		Status: v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionFalse}}},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		Spec:       v1.PodSpec{NodeName: "n1"},
	}
	client := fake.NewSimpleClientset(node, leaseRenewedAt("n1", time.Now()), pod)
	got, err := NodeHealth(context.Background(), client, "n1")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, effect := range []v1.TaintEffect{v1.TaintEffectNoSchedule, v1.TaintEffectNoExecute} {
		if !hasTaint(fresh, v1.TaintNodeNotReady, effect) {
			t.Fatalf("missing not-ready %s taint: %v", effect, fresh.Spec.Taints)
		}
	}
	if hasTaint(fresh, v1.TaintNodeUnreachable, v1.TaintEffectNoExecute) {
		t.Fatalf("unreachable taint kept: %v", fresh.Spec.Taints)
	}
	if got.Evicted != 1 {
		t.Fatalf("evicted = %d", got.Evicted)
	}
}

func TestNodeHealthMarksPodsNotReadyOnAnUnreachableNode(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Status:     v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}}},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		Spec: v1.PodSpec{
			NodeName:    "n1",
			Tolerations: []v1.Toleration{{Key: v1.TaintNodeUnreachable, Operator: v1.TolerationOpExists, Effect: v1.TaintEffectNoExecute, TolerationSeconds: ptr.To[int64](300)}},
		},
		Status: v1.PodStatus{Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}}},
	}
	client := fake.NewSimpleClientset(node, leaseRenewedAt("n1", time.Now().Add(-2*time.Minute)), pod)
	if _, err := NodeHealth(context.Background(), client, "n1"); err != nil {
		t.Fatal(err)
	}
	fresh, err := client.CoreV1().Pods("default").Get(context.Background(), "p1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status.Conditions[0].Status != v1.ConditionFalse {
		t.Fatalf("Ready = %s", fresh.Status.Conditions[0].Status)
	}
}

func TestNodeHealthLeavesVirtualKubeletNodesAlone(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "cloudflare", Labels: map[string]string{"type": "virtual-kubelet"}},
		Status:     v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}}},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		Spec:       v1.PodSpec{NodeName: "cloudflare"},
	}
	client := fake.NewSimpleClientset(node, leaseRenewedAt("cloudflare", time.Now().Add(-time.Hour)), pod)
	got, err := NodeHealth(context.Background(), client, "cloudflare")
	if err != nil {
		t.Fatal(err)
	}
	if got.Evicted != 0 || got.NextMs != 0 {
		t.Fatalf("result = %+v", got)
	}
	fresh, err := client.CoreV1().Nodes().Get(context.Background(), "cloudflare", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.Spec.Taints) != 0 || fresh.Status.Conditions[0].Status != v1.ConditionTrue {
		t.Fatalf("virtual node was touched: %+v", fresh)
	}
	if _, err := client.CoreV1().Pods("default").Get(context.Background(), "p1", metav1.GetOptions{}); err != nil {
		t.Fatalf("pod evicted: %v", err)
	}
}

func nodeHealthOfAnUnreachableNodeWithPod(t *testing.T, tolerations []v1.Toleration) (*NodeHealthResult, *fake.Clientset) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Status:     v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}}},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		Spec:       v1.PodSpec{NodeName: "n1", Tolerations: tolerations},
	}
	client := fake.NewSimpleClientset(node, leaseRenewedAt("n1", time.Now().Add(-2*time.Minute)), pod)
	got, err := NodeHealth(context.Background(), client, "n1")
	if err != nil {
		t.Fatal(err)
	}
	return got, client
}

func TestNodeHealthKeepsAPodThatToleratesTheTaintForever(t *testing.T) {
	forever := v1.Toleration{Key: v1.TaintNodeUnreachable, Operator: v1.TolerationOpExists, Effect: v1.TaintEffectNoExecute}
	bounded := forever
	bounded.TolerationSeconds = ptr.To[int64](300)
	for name, tc := range map[string]struct {
		tolerations []v1.Toleration
		waiting     int
	}{
		"the only toleration":             {[]v1.Toleration{forever}, 0},
		"the first of two that match":     {[]v1.Toleration{forever, bounded}, 0},
		"behind a bounded one that match": {[]v1.Toleration{bounded, forever}, 1},
	} {
		t.Run(name, func(t *testing.T) {
			got, client := nodeHealthOfAnUnreachableNodeWithPod(t, tc.tolerations)
			if _, err := client.CoreV1().Pods("default").Get(context.Background(), "p1", metav1.GetOptions{}); err != nil {
				t.Fatalf("pod evicted: %v", err)
			}
			if got.Evicted != 0 || got.Waiting != tc.waiting || (got.NextMs > 0) != (tc.waiting > 0) {
				t.Fatalf("result = %+v", got)
			}
		})
	}
}

func TestNodeHealthEvictsAPodWhoseTolerationSecondsAreZero(t *testing.T) {
	got, _ := nodeHealthOfAnUnreachableNodeWithPod(t, []v1.Toleration{{Key: v1.TaintNodeUnreachable, Operator: v1.TolerationOpExists, Effect: v1.TaintEffectNoExecute, TolerationSeconds: ptr.To[int64](0)}})
	if got.Evicted != 1 {
		t.Fatalf("result = %+v", got)
	}
}
