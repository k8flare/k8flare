package apiserver_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// TestForegroundFinalizeWaitsForBlockingDependents pins the guard in
// gracefuldelete.go's refuseForegroundFinalize. The real garbagecollector
// clears the "foregroundDeletion" finalizer once ITS GRAPH shows no
// blocking dependents, and as a resident dynamic worker that graph can be
// arbitrarily stale (S33). This drives the failure directly instead of
// waiting for a stale graph to happen: the test itself plays the part of a
// GC that cleared the finalizer too early, and the apiserver must refuse.
//
// Runs in the apiserver lane (KCM_DISABLED), not the kcm lane, on purpose:
// with the gc dynamic worker live the dependents get deleted for real, and
// the "too early" window is whatever the GC's timing happens to be that
// run. Nothing here creates or deletes Pods but the test.
func TestForegroundFinalizeWaitsForBlockingDependents(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	const ns = "fgguard"

	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	replicas := int32(0)
	rc, err := client.CoreV1().ReplicationControllers(ns).Create(ctx, &corev1.ReplicationController{
		ObjectMeta: metav1.ObjectMeta{Name: "fgguard-rc"},
		Spec: corev1.ReplicationControllerSpec{
			Replicas: &replicas,
			Selector: map[string]string{"app": "fgguard"},
			Template: &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "fgguard"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "registry.k8s.io/pause:3.10"}},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create rc: %v", err)
	}

	yes := true
	podNames := []string{"fgguard-pod-a", "fgguard-pod-b"}
	for _, name := range podNames {
		if _, err := client.CoreV1().Pods(ns).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: map[string]string{"app": "fgguard"},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         "v1",
					Kind:               "ReplicationController",
					Name:               rc.Name,
					UID:                rc.UID,
					Controller:         &yes,
					BlockOwnerDeletion: &yes,
				}},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c", Image: "registry.k8s.io/pause:3.10"}},
			},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create dependent %s: %v", name, err)
		}
	}

	fg := metav1.DeletePropagationForeground
	if err := client.CoreV1().ReplicationControllers(ns).Delete(ctx, rc.Name, metav1.DeleteOptions{
		PropagationPolicy: &fg,
		Preconditions:     metav1.NewUIDPreconditions(string(rc.UID)),
	}); err != nil {
		t.Fatalf("foreground delete rc: %v", err)
	}

	terminating, err := client.CoreV1().ReplicationControllers(ns).Get(ctx, rc.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("rc must survive its own foreground DELETE: %v", err)
	}
	if terminating.DeletionTimestamp == nil {
		t.Error("foreground DELETE left no deletionTimestamp")
	}
	if !hasFinalizer(terminating, metav1.FinalizerDeleteDependents) {
		t.Errorf("foreground DELETE left finalizers %v, want foregroundDeletion", terminating.Finalizers)
	}

	// The premature clear a stale-graph GC issues.
	err = clearFinalizers(ctx, client, ns, rc.Name)
	if !apierrors.IsConflict(err) {
		t.Fatalf("clearing foregroundDeletion with 2 blocking dependents alive = %v, want Conflict", err)
	}
	if _, err := client.CoreV1().ReplicationControllers(ns).Get(ctx, rc.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("rc must outlive its dependents, but the refused clear removed it: %v", err)
	}

	for _, name := range podNames {
		if err := client.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("delete dependent %s: %v", name, err)
		}
	}

	// Same clear, now that nothing blocks: it must complete the deletion.
	if err := clearFinalizers(ctx, client, ns, rc.Name); err != nil {
		t.Fatalf("clearing foregroundDeletion with no dependents left: %v", err)
	}
	if _, err := client.CoreV1().ReplicationControllers(ns).Get(ctx, rc.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("rc after the last dependent went away = %v, want NotFound", err)
	}
}

func hasFinalizer(rc *corev1.ReplicationController, want string) bool {
	for _, f := range rc.Finalizers {
		if f == want {
			return true
		}
	}
	return false
}

// clearFinalizers strips every finalizer from the rc, the write the real
// garbagecollector makes to complete a foreground deletion.
func clearFinalizers(ctx context.Context, client kubernetes.Interface, ns, name string) error {
	cur, err := client.CoreV1().ReplicationControllers(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	cur.Finalizers = nil
	_, err = client.CoreV1().ReplicationControllers(ns).Update(ctx, cur, metav1.UpdateOptions{})
	return err
}
