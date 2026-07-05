package apiserver_test

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestOwnerReferenceCascadeDelete exercises gc.go's CascadeDeleteDependents,
// this apiserver's substitute for upstream kube-controller-manager's
// garbagecollector controller (see docs/general-purpose-k8s-plan.md). No
// real controller runs in this test (setupWranglerDev starts only the
// apiserver stack), so ownerReferences are set by hand here the same way a
// real ReplicaSet/Deployment controller would -- this test is about the
// apiserver's DELETE handler honoring them, not about the controllers that
// normally create them.
func TestOwnerReferenceCascadeDelete(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "test-gc-cascade-ns"

	_ = client.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().Namespaces().Delete(context.Background(), ns, metav1.DeleteOptions{})
	})

	t.Run("BackgroundDeletesChildren", func(t *testing.T) {
		deploy, err := client.AppsV1().Deployments(ns).Create(ctx, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "owner-deploy"},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create deployment: %v", err)
		}

		rsTrue := true
		rs, err := client.AppsV1().ReplicaSets(ns).Create(ctx, &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "owned-rs",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1", Kind: "Deployment",
					Name: deploy.Name, UID: deploy.UID, Controller: &rsTrue,
				}},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create replicaset: %v", err)
		}

		if _, err := client.CoreV1().Pods(ns).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "owned-pod",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1", Kind: "ReplicaSet",
					Name: rs.Name, UID: rs.UID, Controller: &rsTrue,
				}},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("Create pod: %v", err)
		}

		// Deleting the Deployment alone must cascade through the ReplicaSet
		// down to the Pod -- the two-level chain this issue was about
		// (kubectl delete deployment used to leave both behind).
		if err := client.AppsV1().Deployments(ns).Delete(ctx, deploy.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("Delete deployment: %v", err)
		}

		if _, err := client.AppsV1().ReplicaSets(ns).Get(ctx, rs.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Errorf("expected owned ReplicaSet to be cascade-deleted, got: %v", err)
		}
		if _, err := client.CoreV1().Pods(ns).Get(ctx, "owned-pod", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Errorf("expected owned Pod to be cascade-deleted, got: %v", err)
		}
	})

	t.Run("OrphanLeavesChildBehindWithoutOwnerRef", func(t *testing.T) {
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
	})
}
