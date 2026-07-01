package apiserver

import (
	"context"
	"encoding/base64"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEnsureDefaultServiceAccount_Creates(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	saStore := NewServiceAccountStore(storage)
	ctx := context.Background()

	ensureDefaultServiceAccount(ctx, saStore, "test-ns")

	entry, ok := kv.data["/serviceaccounts/test-ns/default"]
	if !ok {
		t.Fatal("expected default ServiceAccount to be stored")
	}

	storedValue, err := base64.StdEncoding.DecodeString(entry.value)
	if err != nil {
		t.Fatalf("decode stored value: %v", err)
	}

	var sa corev1.ServiceAccount
	if err := DecodeFromStorage(storedValue, &sa); err != nil {
		t.Fatalf("decode stored ServiceAccount: %v", err)
	}
	if sa.Name != "default" || sa.Namespace != "test-ns" {
		t.Errorf("got %s/%s, want test-ns/default", sa.Namespace, sa.Name)
	}
}

func TestEnsureDefaultServiceAccount_IdempotentOnSecondCall(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	saStore := NewServiceAccountStore(storage)
	ctx := context.Background()

	ensureDefaultServiceAccount(ctx, saStore, "test-ns")
	ensureDefaultServiceAccount(ctx, saStore, "test-ns") // AlreadyExists must be swallowed, not panic.

	if len(kv.data) != 1 {
		t.Fatalf("expected exactly one stored entry, got %d", len(kv.data))
	}
	if _, ok := kv.data["/serviceaccounts/test-ns/default"]; !ok {
		t.Fatal("expected default ServiceAccount entry to exist")
	}
}

func TestEnsureDefaultServiceAccount_NilStore(t *testing.T) {
	ctx := context.Background()

	// Must not panic when the serviceaccounts store isn't wired up.
	ensureDefaultServiceAccount(ctx, nil, "test-ns")
}

func TestApplyPostCreateEffects_Namespace_CreatesDefaultServiceAccount(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	stores := map[string]*ResourceStore{
		"serviceaccounts": NewServiceAccountStore(storage),
	}
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
	ApplyPostCreateEffects(ctx, stores, ns)

	if _, ok := kv.data["/serviceaccounts/test-ns/default"]; !ok {
		t.Fatal("expected default ServiceAccount to be created for the new namespace")
	}
}

func TestApplyPostCreateEffects_NonNamespace_NoOp(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	stores := map[string]*ResourceStore{
		"serviceaccounts": NewServiceAccountStore(storage),
	}
	ctx := context.Background()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "test-ns"}}
	ApplyPostCreateEffects(ctx, stores, pod)

	if len(kv.data) != 0 {
		t.Errorf("expected no ServiceAccount to be created for a non-Namespace object, got %d entries", len(kv.data))
	}
}
