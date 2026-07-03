package apiserver

import (
	"context"
	"net"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestPodCIDRAllocator_AllocateExhaustRelease is a fast, wrangler-dev-free
// unit test (same in-memory fakeKV/newTestStorage helpers
// clusterip_allocator_test.go uses), mirroring
// TestClusterIPAllocator_ReleaseAllowsReuse's shape for PodCIDRAllocator. A
// /30 cluster CIDR split into /31 blocks keeps exhaustion (2 blocks) practical
// to reach directly, rather than needing 256 allocations against the real
// /16 clusterCIDR.
func TestPodCIDRAllocator_AllocateExhaustRelease(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()

	_, cidr, err := net.ParseCIDR("10.99.0.0/30")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	a := NewPodCIDRAllocator(storage, cidr, 31)

	first, err := a.AllocateNext(ctx)
	if err != nil {
		t.Fatalf("AllocateNext #1: %v", err)
	}
	second, err := a.AllocateNext(ctx)
	if err != nil {
		t.Fatalf("AllocateNext #2: %v", err)
	}
	if first.String() == second.String() {
		t.Fatalf("AllocateNext returned the same block twice: %s", first)
	}

	// Range is now full (both /31 blocks allocated) -- confirm exhaustion is
	// actually detected, not silently ignored.
	if _, err := a.AllocateNext(ctx); err == nil {
		t.Fatal("expected AllocateNext to fail once the range is exhausted, got nil error")
	}

	// Release one block back...
	if err := a.Release(ctx, first.String()); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// ...and confirm it becomes allocatable again.
	reused, err := a.AllocateNext(ctx)
	if err != nil {
		t.Fatalf("AllocateNext after Release: %v", err)
	}
	if reused.String() != first.String() {
		t.Errorf("expected the released block %s to be reused (only one was free), got %s", first, reused)
	}
}

// TestPodCIDRAllocator_ReleaseUnallocatedIsNoop matches
// ClusterIPAllocator's equivalent: releasing a block that was never
// allocated (or already released) is not an error, so ReleasePodCIDR
// (nodecidr.go) can call this unconditionally on every Node delete.
func TestPodCIDRAllocator_ReleaseUnallocatedIsNoop(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()

	_, cidr, err := net.ParseCIDR("10.99.0.0/24")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	a := NewPodCIDRAllocator(storage, cidr, 28)

	if err := a.Release(ctx, "10.99.0.16/28"); err != nil {
		t.Errorf("Release of a never-allocated block should be a no-op, got: %v", err)
	}
}

// TestAssignPodCIDR_SkipsNodeThatAlreadyHasOne mirrors
// serviceNeedsClusterIP's "already set" skip, for Nodes: a Node created with
// an explicit spec.podCIDR (e.g. a BYO VM node whose kubelet/agent already
// negotiated one some other way) must not be silently overwritten.
func TestAssignPodCIDR_SkipsNodeThatAlreadyHasOne(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "byo-node"},
		Spec:       corev1.NodeSpec{PodCIDR: "10.42.99.0/24", PodCIDRs: []string{"10.42.99.0/24"}},
	}
	if err := AssignPodCIDR(ctx, storage, node); err != nil {
		t.Fatalf("AssignPodCIDR: %v", err)
	}
	if node.Spec.PodCIDR != "10.42.99.0/24" {
		t.Errorf("expected pre-set PodCIDR to be left alone, got %s", node.Spec.PodCIDR)
	}
}

// TestAssignPodCIDR_DistinctNodesGetDistinctBlocks is a basic end-to-end
// sanity check against the real clusterCIDR/nodeCIDRMaskSize (10.42.0.0/16,
// /24 blocks) that two Nodes created back-to-back get different,
// non-overlapping PodCIDRs.
func TestAssignPodCIDR_DistinctNodesGetDistinctBlocks(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()

	n1 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	n2 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2"}}

	if err := AssignPodCIDR(ctx, storage, n1); err != nil {
		t.Fatalf("AssignPodCIDR n1: %v", err)
	}
	if err := AssignPodCIDR(ctx, storage, n2); err != nil {
		t.Fatalf("AssignPodCIDR n2: %v", err)
	}

	if n1.Spec.PodCIDR == "" || n2.Spec.PodCIDR == "" {
		t.Fatalf("expected both nodes to get a PodCIDR, got %q and %q", n1.Spec.PodCIDR, n2.Spec.PodCIDR)
	}
	if n1.Spec.PodCIDR == n2.Spec.PodCIDR {
		t.Errorf("expected distinct PodCIDRs, both got %s", n1.Spec.PodCIDR)
	}
}
