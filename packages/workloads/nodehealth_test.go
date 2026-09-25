package workloads

import (
	"context"
	"fmt"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestNodeHealthClearsTaintsWhenLeaseHeld(t *testing.T) {
	now := metav1.Now()
	renew := metav1.NewMicroTime(now.Time)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Spec: v1.NodeSpec{Taints: []v1.Taint{
			{Key: v1.TaintNodeUnreachable, Effect: v1.TaintEffectNoSchedule, TimeAdded: &now},
			{Key: v1.TaintNodeUnreachable, Effect: v1.TaintEffectNoExecute, TimeAdded: &now},
		}},
		Status: v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionUnknown, Reason: unknownReason}}},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Namespace: v1.NamespaceNodeLease},
		Spec: coordinationv1.LeaseSpec{
			RenewTime:            &renew,
			LeaseDurationSeconds: ptr.To[int32](40),
		},
	}
	client := fake.NewSimpleClientset(node, lease)
	got, err := NodeHealth(context.Background(), client, "n1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Evicted != 0 {
		t.Fatalf("evicted = %d", got.Evicted)
	}
	fresh, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.Spec.Taints) != 0 {
		t.Fatalf("taints = %v", fresh.Spec.Taints)
	}
	if fresh.Status.Conditions[0].Status != v1.ConditionTrue {
		t.Fatalf("Ready = %s", fresh.Status.Conditions[0].Status)
	}
}

func TestNodeHealthTaintsWhenLeaseExpired(t *testing.T) {
	stale := metav1.NewMicroTime(time.Now().Add(-2 * time.Minute))
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Status:     v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}}},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Namespace: v1.NamespaceNodeLease},
		Spec: coordinationv1.LeaseSpec{
			RenewTime:            &stale,
			LeaseDurationSeconds: ptr.To[int32](40),
		},
	}
	client := fake.NewSimpleClientset(node, lease)
	if _, err := NodeHealth(context.Background(), client, "n1"); err != nil {
		t.Fatal(err)
	}
	fresh, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status.Conditions[0].Status != v1.ConditionUnknown {
		t.Fatalf("Ready = %s", fresh.Status.Conditions[0].Status)
	}
	if !hasTaint(fresh, v1.TaintNodeUnreachable, v1.TaintEffectNoSchedule) {
		t.Fatalf("taints = %v", fresh.Spec.Taints)
	}
}

func TestNodeHealthSkipsWhenLeaseUnreadable(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Status:     v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}}},
	}
	client := fake.NewSimpleClientset(node)
	client.PrependReactor("get", "leases", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	if _, err := NodeHealth(context.Background(), client, "n1"); err != nil {
		t.Fatal(err)
	}
	fresh, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status.Conditions[0].Status != v1.ConditionTrue {
		t.Fatalf("Ready = %s", fresh.Status.Conditions[0].Status)
	}
	if len(fresh.Spec.Taints) != 0 {
		t.Fatalf("taints = %v", fresh.Spec.Taints)
	}
}

func hasTaint(node *v1.Node, key string, effect v1.TaintEffect) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == key && t.Effect == effect {
			return true
		}
	}
	return false
}
