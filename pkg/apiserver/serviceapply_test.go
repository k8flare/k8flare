package apiserver

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

// applyInfo is the rest.UpdatedObjectInfo a server-side apply presents:
// the object to store, with no preconditions.
type applyInfo struct{ obj runtime.Object }

func (a applyInfo) Preconditions() *metav1.Preconditions { return nil }
func (a applyInfo) UpdatedObject(context.Context, runtime.Object) (runtime.Object, error) {
	return a.obj, nil
}

// TestServiceApplyCreate_DoesNotLeakClusterIP covers the other way a
// Service create reaches BeginCreate. Server-side apply is wired
// (installer.go's TypeConverter), and an apply of a Service that does not
// exist goes through Store.Update's create branch with forceAllowCreate,
// where the FinishFunc fires before the storage write rather than after.
// Measured: one address per successful apply, none left behind when the
// create is rejected. The CAS-retry path is NOT covered -- the in-memory
// KV these tests use does not inject write conflicts.
func TestServiceApplyCreate_DoesNotLeakClusterIP(t *testing.T) {
	storage := newTestStorage(newFakeKV())
	ctx := context.Background()
	svcStore := newServiceStoreForHookTest(storage)

	alloc := ServiceIPAllocator(storage)
	b0, _, _ := alloc.load(ctx)
	free0 := b0.Free()

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-apply", Namespace: "ns-ssa"}}
	ctx = genericapirequest.WithNamespace(ctx, "ns-ssa")
	out, created, err := svcStore.upstream.Update(ctx, "svc-apply", applyInfo{svc},
		rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, true, &metav1.UpdateOptions{})
	b1, _, _ := alloc.load(ctx)
	got := ""
	if s, ok := out.(*corev1.Service); ok {
		got = s.Spec.ClusterIP
	}
	if err != nil || !created {
		t.Fatalf("apply-create: created=%v err=%v", created, err)
	}
	if got == "" {
		t.Fatal("expected the apply-create to allocate a ClusterIP")
	}
	if n := free0 - b1.Free(); n != 1 {
		t.Errorf("apply-create consumed %d addresses, want 1", n)
	}

	svc2 := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-apply-doomed", Namespace: "ns-ssa"}}
	b2, _, _ := alloc.load(ctx)
	free2 := b2.Free()
	_, _, err2 := svcStore.upstream.Update(ctx, "svc-apply-doomed", applyInfo{svc2},
		func(context.Context, runtime.Object) error { return errRejected },
		rest.ValidateAllObjectUpdateFunc, true, &metav1.UpdateOptions{})
	b3, _, _ := alloc.load(ctx)
	if err2 == nil {
		t.Fatal("expected the rejected apply-create to fail")
	}
	if n := free2 - b3.Free(); n != 0 {
		t.Errorf("a rejected apply-create leaked %d addresses", n)
	}
}

var errRejected = errors.New("rejected by createValidation")
