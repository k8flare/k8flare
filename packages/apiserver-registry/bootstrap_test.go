package registry_test

import (
	"context"
	"errors"
	"testing"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	"github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRunBootstrap(t *testing.T) {
	cs := registrytest.NewCountingStore(t)
	store := cs.NewStore(t, corev1.SchemeGroupVersion, metav1.APIResource{Name: "configmaps", Kind: "ConfigMap", SingularName: "configmap", Namespaced: true})

	var ensureCalls int
	ensure := func(ctx context.Context) error {
		ensureCalls++
		return nil
	}

	hash1 := registry.HashObjects("version-1")
	ctx := context.Background()

	// 1. First run executes ensure and writes marker
	if err := registry.RunBootstrap(ctx, store, "testgroup", hash1, ensure); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if ensureCalls != 1 {
		t.Fatalf("ensureCalls = %d, want 1", ensureCalls)
	}
	marker, ok := cs.GetMarker("testgroup")
	if !ok || marker != hash1 {
		t.Fatalf("marker = %q, want %q", marker, hash1)
	}

	// 2. Second run with same hash skips ensure (0 ensure calls, 1 marker read)
	cs.MarkerGets.Store(0)
	ensureCalls = 0
	if err := registry.RunBootstrap(ctx, store, "testgroup", hash1, ensure); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if ensureCalls != 0 {
		t.Fatalf("second run ensureCalls = %d, want 0", ensureCalls)
	}
	if gets := cs.MarkerGets.Load(); gets != 1 {
		t.Fatalf("second run marker gets = %d, want 1", gets)
	}

	// 3. Third run with changed hash executes ensure and updates marker
	hash2 := registry.HashObjects("version-2")
	ensureCalls = 0
	if err := registry.RunBootstrap(ctx, store, "testgroup", hash2, ensure); err != nil {
		t.Fatalf("hash change run: %v", err)
	}
	if ensureCalls != 1 {
		t.Fatalf("hash change ensureCalls = %d, want 1", ensureCalls)
	}
	marker, ok = cs.GetMarker("testgroup")
	if !ok || marker != hash2 {
		t.Fatalf("marker = %q, want %q", marker, hash2)
	}

	// 4. Failing ensure returns error and does NOT overwrite marker
	failingEnsure := func(ctx context.Context) error {
		return errors.New("boom")
	}
	hash3 := registry.HashObjects("version-3")
	if err := registry.RunBootstrap(ctx, store, "testgroup", hash3, failingEnsure); err == nil {
		t.Fatal("expected error, got nil")
	}
	marker, _ = cs.GetMarker("testgroup")
	if marker != hash2 {
		t.Fatalf("failing ensure updated marker to %q, want %q", marker, hash2)
	}
}
