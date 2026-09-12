package apiserver

import (
	"context"
	"net"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

// TestDeleteNamespaceDependents_ReleasesServiceClusterIPs guards against a
// bug found by review: ResourceStore.DeleteAllInNamespace (what
// DeleteNamespaceDependents uses to sweep every namespaced resource type)
// works on raw stored bytes, by design, for genericity across resource
// types -- so it never decodes a Service and never calls ReleaseClusterIP
// the way the direct Service DELETE paths in handler.go do. Before the
// fix, deleting a Namespace that contained a Service permanently leaked
// that Service's ClusterIP. Checks the allocator's persisted bitmap
// directly (via the unexported load/offsetFor this white-box test has
// access to) rather than relying on AllocateNext's random-scan strategy to
// eventually re-surface the same address, which isn't a practical thing to
// wait for deterministically against the real /16 ServiceCIDR.
func TestDeleteNamespaceDependents_ReleasesServiceClusterIPs(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()
	ns := "ns-release-test"

	svcStore := NewResourceStore(storage, corev1.SchemeGroupVersion, "services", "service", true,
		func() runtime.Object { return &corev1.Service{} },
		func() runtime.Object { return &corev1.ServiceList{} },
	)

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc"}}
	if err := AssignClusterIP(ctx, storage, svc); err != nil {
		t.Fatalf("AssignClusterIP: %v", err)
	}
	if svc.Spec.ClusterIP == "" {
		t.Fatal("expected AssignClusterIP to set a ClusterIP")
	}
	if _, err := svcStore.Create(ctx, ns, svc, nil); err != nil {
		t.Fatalf("Create: %v", err)
	}

	allocator := ServiceIPAllocator(storage)
	offset, err := allocator.offsetFor(net.ParseIP(svc.Spec.ClusterIP))
	if err != nil {
		t.Fatalf("offsetFor: %v", err)
	}

	bitmapBefore, _, err := allocator.load(ctx)
	if err != nil {
		t.Fatalf("load (before): %v", err)
	}
	if !bitmapBefore.Has(offset) {
		t.Fatalf("expected %s to be marked allocated before the namespace sweep", svc.Spec.ClusterIP)
	}

	if err := DeleteNamespaceDependents(ctx, []*ResourceStore{svcStore}, ns); err != nil {
		t.Fatalf("DeleteNamespaceDependents: %v", err)
	}

	bitmapAfter, _, err := allocator.load(ctx)
	if err != nil {
		t.Fatalf("load (after): %v", err)
	}
	if bitmapAfter.Has(offset) {
		t.Errorf("expected %s to be released by DeleteNamespaceDependents, but it's still marked allocated", svc.Spec.ClusterIP)
	}
}

// newServiceStoreForHookTest builds the real Service store so its
// BeginCreate/AfterDelete hooks can be driven directly. Checking the
// persisted bitmap is the only deterministic way to see what they did:
// AllocateNext picks at random over the /16, so "allocate again and see
// whether the same address comes back" is not a thing to wait for.
func newServiceStoreForHookTest(storage *Storage) *ResourceStore {
	return NewResourceStore(storage, corev1.SchemeGroupVersion, "services", "service", true,
		func() runtime.Object { return &corev1.Service{} },
		func() runtime.Object { return &corev1.ServiceList{} },
	)
}

func allocatedOffset(t *testing.T, storage *Storage, ip string) (int, *ClusterIPAllocator) {
	t.Helper()
	allocator := ServiceIPAllocator(storage)
	offset, err := allocator.offsetFor(net.ParseIP(ip))
	if err != nil {
		t.Fatalf("offsetFor(%s): %v", ip, err)
	}
	return offset, allocator
}

func bitmapHas(t *testing.T, allocator *ClusterIPAllocator, offset int) bool {
	t.Helper()
	bitmap, _, err := allocator.load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return bitmap.Has(offset)
}

// TestServiceAfterDelete_DryRunKeepsClusterIP: a dry-run DELETE reaches
// AfterDelete. The storage layer reads and validates and reports success
// without writing, so a hook that releases unconditionally hands a live
// Service's address back to the allocator -- and the Service is still
// being served with it.
func TestServiceAfterDelete_DryRunKeepsClusterIP(t *testing.T) {
	storage := newTestStorage(newFakeKV())
	ctx := context.Background()
	svcStore := newServiceStoreForHookTest(storage)

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-dryrun"}}
	if err := AssignClusterIP(ctx, storage, svc); err != nil {
		t.Fatalf("AssignClusterIP: %v", err)
	}
	if _, err := svcStore.Create(ctx, "ns-dryrun", svc, nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
	offset, allocator := allocatedOffset(t, storage, svc.Spec.ClusterIP)
	if !bitmapHas(t, allocator, offset) {
		t.Fatalf("expected %s to be allocated after the create", svc.Spec.ClusterIP)
	}

	svcStore.upstream.AfterDelete(svc, &metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}})
	if !bitmapHas(t, allocator, offset) {
		t.Errorf("a dry-run delete released %s while the Service still exists", svc.Spec.ClusterIP)
	}

	svcStore.upstream.AfterDelete(svc, &metav1.DeleteOptions{})
	if bitmapHas(t, allocator, offset) {
		t.Errorf("a real delete left %s allocated", svc.Spec.ClusterIP)
	}
}

// TestServiceBeginCreate_ReleasesOnFailedCreate: the allocation is a
// persisted write made before the object reaches storage, so a create
// that fails afterwards -- AlreadyExists, validation, a storage error --
// leaks the address unless the FinishFunc hands it back.
func TestServiceBeginCreate_ReleasesOnFailedCreate(t *testing.T) {
	storage := newTestStorage(newFakeKV())
	ctx := context.Background()
	svcStore := newServiceStoreForHookTest(storage)

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-doomed"}}
	finish, err := svcStore.upstream.BeginCreate(ctx, svc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("BeginCreate: %v", err)
	}
	if svc.Spec.ClusterIP == "" {
		t.Fatal("expected BeginCreate to allocate a ClusterIP")
	}
	offset, allocator := allocatedOffset(t, storage, svc.Spec.ClusterIP)
	if !bitmapHas(t, allocator, offset) {
		t.Fatalf("expected %s to be allocated by BeginCreate", svc.Spec.ClusterIP)
	}

	finish(ctx, false)
	if bitmapHas(t, allocator, offset) {
		t.Errorf("a create that never reached storage leaked %s", svc.Spec.ClusterIP)
	}
}

// TestServiceBeginCreate_LeavesAnExplicitClusterIPAlone: the release must
// hand back only what this hook allocated. A Service that names its own
// ClusterIP is not allocated from the bitmap at all, so releasing it on a
// failed create would free an address belonging to whoever does hold it.
func TestServiceBeginCreate_LeavesAnExplicitClusterIPAlone(t *testing.T) {
	storage := newTestStorage(newFakeKV())
	ctx := context.Background()
	svcStore := newServiceStoreForHookTest(storage)

	held := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-holder"}}
	if err := AssignClusterIP(ctx, storage, held); err != nil {
		t.Fatalf("AssignClusterIP: %v", err)
	}
	offset, allocator := allocatedOffset(t, storage, held.Spec.ClusterIP)

	squatter := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-squatter"},
		Spec:       corev1.ServiceSpec{ClusterIP: held.Spec.ClusterIP},
	}
	finish, err := svcStore.upstream.BeginCreate(ctx, squatter, &metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("BeginCreate: %v", err)
	}
	finish(ctx, false)

	if !bitmapHas(t, allocator, offset) {
		t.Errorf("a failed create of a Service naming %s released the holder's address", held.Spec.ClusterIP)
	}
}
