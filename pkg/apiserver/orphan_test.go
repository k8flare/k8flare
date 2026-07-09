package apiserver_test

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestOrphanDependents exercises orphan.go's OrphanDependents, the one
// propagationPolicy this apiserver still handles synchronously in its
// own DELETE handler (see handler.go's HandleResource doc comment).
// Background/Foreground cascade delete used to be tested here too
// (this file's earlier TestOwnerReferenceCascadeDelete), but that's now
// the real, unmodified upstream garbagecollector controller's job
// (pkg/controllers/gc), running asynchronously in its own dynamic
// worker -- which this suite's wrangler dev harness deliberately never
// loads (setupWranglerDev sets KCM_DISABLED=1, gating every poke in
// workers/k8flare/src/controllers/index.ts's fetch(), gc included, so
// pkg/apiserver's own Pods are never touched by a controller mid-test).
// Cascade-delete coverage for the real controller lives in
// e2e-conformance's upstream [sig-api-machinery] Garbage collector
// suite instead, against a cluster where it actually runs.
func TestOrphanDependents(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "test-gc-orphan-ns"

	_ = client.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().Namespaces().Delete(context.Background(), ns, metav1.DeleteOptions{})
	})

	deploy, err := client.AppsV1().Deployments(ns).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "owner-deploy-orphan"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create deployment: %v", err)
	}

	rsTrue := true
	rs, err := client.AppsV1().ReplicaSets(ns).Create(ctx, &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "orphaned-rs",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "Deployment",
				Name: deploy.Name, UID: deploy.UID, Controller: &rsTrue,
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create replicaset: %v", err)
	}

	policy := metav1.DeletePropagationOrphan
	if err := client.AppsV1().Deployments(ns).Delete(ctx, deploy.Name, metav1.DeleteOptions{
		PropagationPolicy: &policy,
	}); err != nil {
		t.Fatalf("Delete deployment with Orphan policy: %v", err)
	}

	got, err := client.AppsV1().ReplicaSets(ns).Get(ctx, rs.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected orphaned ReplicaSet to still exist, got: %v", err)
	}
	if len(got.OwnerReferences) != 0 {
		t.Errorf("expected ownerReferences to be stripped after Orphan delete, got: %+v", got.OwnerReferences)
	}
}
