package apiserver

import (
	"context"
	"net"
	"testing"
)

// TestClusterIPAllocator_ReleaseAllowsReuse is a fast, wrangler-dev-free
// unit test (using the same in-memory fakeKV/newTestStorage helpers
// nodepassword_test.go uses) for ClusterIPAllocator.Release, added after a
// review found this project's ClusterIP allocator had no release path at
// all (every Service delete leaked its address permanently). A tiny /30
// CIDR keeps exhaustion practical to reach directly, rather than needing
// thousands of allocations against the real /16 ServiceCIDR.
func TestClusterIPAllocator_ReleaseAllowsReuse(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()

	_, cidr, err := net.ParseCIDR("10.99.0.0/30")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	a := NewClusterIPAllocator(storage, cidr)

	// A /30 has 4 addresses; excluding the network (10.99.0.0) and
	// broadcast (10.99.0.3) addresses (see NewClusterIPAllocator's doc
	// comment on why those are excluded, matching upstream
	// ipallocator.Range), only 2 are ever allocatable:
	// 10.99.0.1 and 10.99.0.2.
	first, err := a.AllocateNext(ctx)
	if err != nil {
		t.Fatalf("AllocateNext #1: %v", err)
	}
	second, err := a.AllocateNext(ctx)
	if err != nil {
		t.Fatalf("AllocateNext #2: %v", err)
	}
	if first.Equal(second) {
		t.Fatalf("AllocateNext returned the same address twice: %s", first)
	}
	for _, ip := range []net.IP{first, second} {
		if net.ParseIP("10.99.0.0").Equal(ip) || net.ParseIP("10.99.0.3").Equal(ip) {
			t.Errorf("AllocateNext returned the network or broadcast address: %s", ip)
		}
	}

	// Range is now full (both usable addresses allocated) -- confirm
	// exhaustion is actually detected, not silently ignored.
	if _, err := a.AllocateNext(ctx); err == nil {
		t.Fatal("expected AllocateNext to fail once the range is exhausted, got nil error")
	}

	// Release one address back...
	if err := a.Release(ctx, first); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// ...and confirm it becomes allocatable again (this is the behavior
	// that was entirely missing before this test existed: nothing
	// previously exercised Release at all).
	reused, err := a.AllocateNext(ctx)
	if err != nil {
		t.Fatalf("AllocateNext after Release: %v", err)
	}
	if !reused.Equal(first) {
		t.Errorf("expected the released address %s to be reused (only one was free), got %s", first, reused)
	}
}

// TestClusterIPAllocator_ReleaseUnallocatedIsNoop matches
// allocator.AllocationBitmap.Release's own idempotent behavior: releasing
// an address that was never allocated (or already released) is not an
// error, so ReleaseClusterIP (clusterip.go) can call this unconditionally
// on every Service delete without first checking whether the address is
// actually still marked allocated.
func TestClusterIPAllocator_ReleaseUnallocatedIsNoop(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()

	_, cidr, err := net.ParseCIDR("10.99.0.0/30")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	a := NewClusterIPAllocator(storage, cidr)

	if err := a.Release(ctx, net.ParseIP("10.99.0.1")); err != nil {
		t.Errorf("Release of a never-allocated address should be a no-op, got: %v", err)
	}
}
