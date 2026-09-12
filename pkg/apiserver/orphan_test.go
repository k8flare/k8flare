package apiserver_test

import (
	"context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestOrphanDependents exercises the apiserver half of the
// graceful-deletion lifecycle (gracefuldelete.go): an Orphan delete
// stamps deletionTimestamp + the "orphan" finalizer without removing
// the owner or touching dependents, and clearing the last finalizer
// completes the deletion. The OTHER half -- actually orphaning /
// cascading dependents -- is the real, unmodified upstream
// garbagecollector controller's job (pkg/controllers/gc), running
// asynchronously in its own dynamic worker, which this suite's wrangler
// dev harness deliberately never loads (setupWranglerDev sets
// KCM_DISABLED=1, gating every poke in
// packages/k8flare-worker/src/controllers/index.ts's fetch(), gc included, so
// pkg/apiserver's own objects are never touched by a controller
// mid-test). GC-driven completion is covered by e2e-conformance's
// upstream [sig-api-machinery] Garbage collector suite instead, against
// a cluster where the GC actually runs.
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

	// Graceful-deletion lifecycle (gracefuldelete.go): the orphan DELETE
	// must NOT remove the owner or touch its dependents itself -- it
	// stamps deletionTimestamp + the "orphan" finalizer and leaves the
	// actual orphaning to the real garbagecollector. This suite runs
	// with KCM_DISABLED=1 (no GC dynamic worker), so the owner stays
	// terminating and the dependent keeps its ownerReference; the full
	// GC-driven completion is covered by e2e-conformance's
	// [sig-api-machinery] Garbage collector group against a live GC.
	gotDeploy, err := client.AppsV1().Deployments(ns).Get(ctx, deploy.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected terminating Deployment to still exist, got: %v", err)
	}
	if gotDeploy.DeletionTimestamp == nil {
		t.Errorf("expected deletionTimestamp to be stamped on the orphan-deleted Deployment")
	}
	foundFinalizer := false
	for _, f := range gotDeploy.Finalizers {
		if f == metav1.FinalizerOrphanDependents {
			foundFinalizer = true
		}
	}
	if !foundFinalizer {
		t.Errorf("expected %q finalizer on the orphan-deleted Deployment, got: %v", metav1.FinalizerOrphanDependents, gotDeploy.Finalizers)
	}

	gotRS, err := client.AppsV1().ReplicaSets(ns).Get(ctx, rs.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected ReplicaSet to still exist, got: %v", err)
	}
	if len(gotRS.OwnerReferences) != 1 {
		t.Errorf("expected the ReplicaSet's ownerReference to be untouched (orphaning belongs to the GC), got: %+v", gotRS.OwnerReferences)
	}

	// Clearing the finalizer while a dependent still points at this owner
	// must be REFUSED. The real garbage collector strips every dependent's
	// ownerReference and only then removes the finalizer; it can get the
	// order wrong here, because its informers are torn down at every
	// pump-window boundary, and when it does the dependents it did not see
	// become garbage the instant the owner goes. Measured at 41% of orphan
	// cascades before this guard existed (docs/platform-verification.md S69).
	//
	// This block used to do the clear directly and call it "the write the
	// real GC performs when it finishes orphaning" -- which is what the GC
	// does when it finishes orphaning WRONGLY.
	stillOwned := gotDeploy.DeepCopy()
	stillOwned.Finalizers = nil
	if _, err := client.AppsV1().Deployments(ns).Update(ctx, stillOwned, metav1.UpdateOptions{}); !apierrors.IsConflict(err) {
		t.Fatalf("clearing the orphan finalizer with a dependent still owned = %v, want Conflict", err)
	}
	if _, err := client.AppsV1().Deployments(ns).Get(ctx, deploy.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("the refused clear must leave the Deployment alone, got: %v", err)
	}

	// Once the dependent is orphaned for real, the clear completes the delete.
	gotRS.OwnerReferences = nil
	if _, err := client.AppsV1().ReplicaSets(ns).Update(ctx, gotRS, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("stripping the ReplicaSet's ownerReference: %v", err)
	}
	current, err := client.AppsV1().Deployments(ns).Get(ctx, deploy.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("re-reading the terminating Deployment: %v", err)
	}
	current.Finalizers = nil
	if _, err := client.AppsV1().Deployments(ns).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("finalizer-clearing update: %v", err)
	}
	if _, err := client.AppsV1().Deployments(ns).Get(ctx, deploy.Name, metav1.GetOptions{}); err == nil {
		t.Errorf("expected the Deployment to be gone after its last finalizer was cleared")
	}
}

// TestOrphanIsNotWedgedByAnEvent pins the liveness half of the orphan
// guard. The guard refuses to clear the finalizer while any dependent
// still carries the owner's UID -- but the real garbage collector never
// monitors Events (garbagecollector.DefaultIgnoredResources), so an
// Event with an explicit ownerReference is never orphaned and the
// refusal would stand forever. Before gcIgnoredResources the owner
// below could not be deleted at all, by anyone.
func TestOrphanIsNotWedgedByAnEvent(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "test-gc-orphan-event-ns"

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
		ObjectMeta: metav1.ObjectMeta{Name: "owner-deploy-evented"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create deployment: %v", err)
	}

	blocking := true
	if _, err := client.CoreV1().Events(ns).Create(ctx, &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name: "owner-deploy-evented.1",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "Deployment",
				Name: deploy.Name, UID: deploy.UID,
				BlockOwnerDeletion: &blocking,
			}},
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "apps/v1", Kind: "Deployment",
			Namespace: ns, Name: deploy.Name, UID: deploy.UID,
		},
		Reason:  "ScalingReplicaSet",
		Message: "an event that owns nothing and blocks nothing",
		Type:    corev1.EventTypeNormal,
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create event: %v", err)
	}

	policy := metav1.DeletePropagationOrphan
	if err := client.AppsV1().Deployments(ns).Delete(ctx, deploy.Name, metav1.DeleteOptions{
		PropagationPolicy: &policy,
	}); err != nil {
		t.Fatalf("Delete deployment with Orphan policy: %v", err)
	}

	current, err := client.AppsV1().Deployments(ns).Get(ctx, deploy.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("re-reading the terminating Deployment: %v", err)
	}
	current.Finalizers = nil
	if _, err := client.AppsV1().Deployments(ns).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("clearing the orphan finalizer with only an Event owned = %v, want success", err)
	}
	if _, err := client.AppsV1().Deployments(ns).Get(ctx, deploy.Name, metav1.GetOptions{}); err == nil {
		t.Errorf("expected the Deployment to be gone once its last finalizer was cleared")
	}
}

// TestForegroundIsNotWedgedByAnEvent is the same wedge on the other
// guard. RefuseForegroundFinalizeOn asks blockingDependent, which wants
// blockOwnerDeletion -- an Event can carry it, and the collector ignores
// Events either way, so the owner would never finish terminating.
func TestForegroundIsNotWedgedByAnEvent(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "test-gc-foreground-event-ns"

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
		ObjectMeta: metav1.ObjectMeta{Name: "owner-deploy-fg-evented"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create deployment: %v", err)
	}

	blocking := true
	if _, err := client.CoreV1().Events(ns).Create(ctx, &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name: "owner-deploy-fg-evented.1",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "Deployment",
				Name: deploy.Name, UID: deploy.UID,
				BlockOwnerDeletion: &blocking,
			}},
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "apps/v1", Kind: "Deployment",
			Namespace: ns, Name: deploy.Name, UID: deploy.UID,
		},
		Reason:  "ScalingReplicaSet",
		Message: "an event the collector will never look at",
		Type:    corev1.EventTypeNormal,
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create event: %v", err)
	}

	policy := metav1.DeletePropagationForeground
	if err := client.AppsV1().Deployments(ns).Delete(ctx, deploy.Name, metav1.DeleteOptions{
		PropagationPolicy: &policy,
	}); err != nil {
		t.Fatalf("Delete deployment with Foreground policy: %v", err)
	}

	current, err := client.AppsV1().Deployments(ns).Get(ctx, deploy.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("re-reading the terminating Deployment: %v", err)
	}
	current.Finalizers = nil
	if _, err := client.AppsV1().Deployments(ns).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("clearing the foreground finalizer with only an Event owned = %v, want success", err)
	}
	if _, err := client.AppsV1().Deployments(ns).Get(ctx, deploy.Name, metav1.GetOptions{}); err == nil {
		t.Errorf("expected the Deployment to be gone once its last finalizer was cleared")
	}
}
