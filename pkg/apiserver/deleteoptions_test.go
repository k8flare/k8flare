package apiserver_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// A UID precondition on DELETE is how a client avoids deleting a
// *recreated* object that happens to carry the name it asked for; the
// real garbagecollector sends one on every delete it issues. The
// apiserver used to build a fresh metav1.DeleteOptions for the upstream
// store and throw the caller's away, so the guard was silently ignored
// and the recreated object was deleted (TODO.md P0-3).
func TestDeleteUIDPreconditionIsEnforced(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	const ns, name = "default", "delopts-uid"

	cm := createDeleteOptionsConfigMap(t, client, ns, name)

	err := client.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{
		Preconditions: metav1.NewUIDPreconditions("00000000-0000-0000-0000-000000000000"),
	})
	if !apierrors.IsConflict(err) {
		t.Fatalf("delete with a stale UID precondition: got %v, want Conflict", err)
	}
	if _, err := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{}); err != nil {
		t.Fatalf("object must survive a failed UID precondition: %v", err)
	}

	if err := client.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{
		Preconditions: metav1.NewUIDPreconditions(string(cm.UID)),
	}); err != nil {
		t.Fatalf("delete with the matching UID precondition: %v", err)
	}
	if _, err := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("after the matching-UID delete: got %v, want NotFound", err)
	}
}

// The resourceVersion precondition is the other half of metav1.Preconditions
// and travelled the same discarded-options path.
func TestDeleteResourceVersionPreconditionIsEnforced(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	const ns, name = "default", "delopts-rv"

	cm := createDeleteOptionsConfigMap(t, client, ns, name)
	t.Cleanup(func() {
		_ = client.CoreV1().ConfigMaps(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	stale := "1"
	err := client.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{ResourceVersion: &stale},
	})
	if !apierrors.IsConflict(err) {
		t.Fatalf("delete with a stale resourceVersion precondition: got %v, want Conflict", err)
	}
	if _, err := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{}); err != nil {
		t.Fatalf("object must survive a failed resourceVersion precondition: %v", err)
	}

	if err := client.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{ResourceVersion: &cm.ResourceVersion},
	}); err != nil {
		t.Fatalf("delete with the matching resourceVersion precondition: %v", err)
	}
}

// Same guard on the graceful-deletion branch (upstreamMarkForDeletion),
// which is the path every Orphan/Foreground delete takes -- including the
// garbagecollector's own UID-guarded foreground deletes.
func TestGracefulDeleteUIDPreconditionIsEnforced(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	const ns, name = "default", "delopts-graceful-uid"

	createDeleteOptionsConfigMap(t, client, ns, name)
	t.Cleanup(func() {
		_ = client.CoreV1().ConfigMaps(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	fg := metav1.DeletePropagationForeground
	err := client.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &fg,
		Preconditions:     metav1.NewUIDPreconditions("00000000-0000-0000-0000-000000000000"),
	})
	if !apierrors.IsConflict(err) {
		t.Fatalf("foreground delete with a stale UID precondition: got %v, want Conflict", err)
	}
	got, err := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("object must survive a failed UID precondition: %v", err)
	}
	if got.DeletionTimestamp != nil {
		t.Fatalf("object was marked for deletion despite the failed UID precondition")
	}
}

// A server-side dry-run DELETE must report what would happen and leave the
// object in place. It reached the store with the dryRun flag stripped, so
// `kubectl delete --dry-run=server` deleted for real.
func TestServerSideDryRunDeleteKeepsObject(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	const ns, name = "default", "delopts-dryrun"

	createDeleteOptionsConfigMap(t, client, ns, name)
	t.Cleanup(func() {
		_ = client.CoreV1().ConfigMaps(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	if err := client.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{
		DryRun: []string{metav1.DryRunAll},
	}); err != nil {
		t.Fatalf("dry-run delete: %v", err)
	}
	if _, err := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{}); err != nil {
		t.Fatalf("object must survive a dry-run delete: %v", err)
	}
}

// The collection verb builds its own per-item deletes, so it needs the
// same dry-run plumbing as the single-object path.
func TestServerSideDryRunDeleteCollectionKeepsObjects(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	const ns = "delopts-collection"

	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().Namespaces().Delete(context.Background(), ns, metav1.DeleteOptions{})
	})
	createDeleteOptionsConfigMap(t, client, ns, "delopts-collection-a")
	createDeleteOptionsConfigMap(t, client, ns, "delopts-collection-b")

	if err := client.CoreV1().ConfigMaps(ns).DeleteCollection(ctx,
		metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}}, metav1.ListOptions{}); err != nil {
		t.Fatalf("dry-run collection delete: %v", err)
	}
	list, err := client.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list after dry-run collection delete: %v", err)
	}
	for _, want := range []string{"delopts-collection-a", "delopts-collection-b"} {
		found := false
		for i := range list.Items {
			if list.Items[i].Name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("configmap %q must survive a dry-run collection delete", want)
		}
	}
}

func createDeleteOptionsConfigMap(t *testing.T, client kubernetes.Interface, ns, name string) *corev1.ConfigMap {
	t.Helper()
	ctx := context.Background()
	_ = client.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{})
	cm, err := client.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       map[string]string{"k": "v"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create configmap %s/%s: %v", ns, name, err)
	}
	return cm
}
