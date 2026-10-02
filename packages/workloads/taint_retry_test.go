package workloads

import (
	"context"
	"fmt"
	"testing"

	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestClearRecoveredNodesRetriesOnConflict(t *testing.T) {
	now := metav1.Now()
	renew := metav1.NewMicroTime(now.Time)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", ResourceVersion: "1"},
		Spec: v1.NodeSpec{
			Taints: []v1.Taint{
				{Key: v1.TaintNodeNotReady, Effect: v1.TaintEffectNoSchedule, TimeAdded: &now},
			},
		},
		Status: v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}}},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Namespace: v1.NamespaceNodeLease},
		Spec: coordinationv1.LeaseSpec{
			RenewTime:            &renew,
			LeaseDurationSeconds: ptr.To[int32](40),
		},
	}
	client := fake.NewSimpleClientset(node, lease)

	staleNode := node.DeepCopy()

	storedNode, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	storedNode.Spec.PodCIDR = "10.244.0.0/24"
	storedNode.ResourceVersion = "2"
	if _, err := client.CoreV1().Nodes().Update(context.Background(), storedNode, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	first := true
	client.PrependReactor("update", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if first {
			first = false
			return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "nodes"}, "n1", fmt.Errorf("conflict"))
		}
		return false, nil, nil
	})

	if err := clearRecoveredNodes(context.Background(), client, []*v1.Node{staleNode}); err != nil {
		t.Fatal(err)
	}

	fresh, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if hasTaint(fresh, v1.TaintNodeNotReady, v1.TaintEffectNoSchedule) || hasTaint(fresh, v1.TaintNodeUnreachable, v1.TaintEffectNoSchedule) {
		t.Fatalf("node still has not-ready/unreachable taints: %v", fresh.Spec.Taints)
	}
	if fresh.Spec.PodCIDR != "10.244.0.0/24" {
		t.Fatalf("concurrent change lost: PodCIDR = %q, want 10.244.0.0/24", fresh.Spec.PodCIDR)
	}
}

func TestSyncBooksNextMsWhenTaintClearingFails(t *testing.T) {
	now := metav1.Now()
	renew := metav1.NewMicroTime(now.Time)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Spec: v1.NodeSpec{
			Taints: []v1.Taint{
				{Key: v1.TaintNodeNotReady, Effect: v1.TaintEffectNoSchedule, TimeAdded: &now},
			},
		},
		Status: v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}}},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Namespace: v1.NamespaceNodeLease},
		Spec: coordinationv1.LeaseSpec{
			RenewTime:            &renew,
			LeaseDurationSeconds: ptr.To[int32](40),
		},
	}
	client := fake.NewSimpleClientset(node, lease)
	client.PrependReactor("update", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(fmt.Errorf("disk full"))
	})

	result, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"nodes"})
	if err != nil {
		t.Fatal(err)
	}
	if result.NextMs <= 0 {
		t.Fatalf("result.NextMs = %d, want > 0 on taint clear failure", result.NextMs)
	}
}
