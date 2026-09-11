package apiserver_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// CreateOptions/UpdateOptions/PatchOptions were hand-built for the
// upstream store the same way DeleteOptions was (TODO.md P0-3's audit),
// so `?dryRun=All` reached it stripped and every "dry" write was real --
// which is what `kubectl diff` and `kubectl apply --dry-run=server` send.
func TestServerSideDryRunWritesDoNotPersist(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	const ns = "dryrunwrites"

	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().Namespaces().Delete(context.Background(), ns, metav1.DeleteOptions{})
	})

	t.Run("create", func(t *testing.T) {
		created, err := client.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "dryrun-create"},
			Data:       map[string]string{"k": "v"},
		}, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
		if err != nil {
			t.Fatalf("dry-run create: %v", err)
		}
		if created.Data["k"] != "v" {
			t.Errorf("dry-run create must still return the would-be object, got %v", created.Data)
		}
		if _, err := client.CoreV1().ConfigMaps(ns).Get(ctx, "dryrun-create", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Errorf("dry-run create persisted the object: get returned %v, want NotFound", err)
		}
	})

	t.Run("update", func(t *testing.T) {
		cm, err := client.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "dryrun-update"},
			Data:       map[string]string{"k": "before"},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		cm.Data["k"] = "after"
		if _, err := client.CoreV1().ConfigMaps(ns).Update(ctx, cm,
			metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}}); err != nil {
			t.Fatalf("dry-run update: %v", err)
		}
		got, err := client.CoreV1().ConfigMaps(ns).Get(ctx, "dryrun-update", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Data["k"] != "before" {
			t.Errorf("dry-run update persisted: data.k = %q, want before", got.Data["k"])
		}
	})

	t.Run("patch", func(t *testing.T) {
		if _, err := client.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "dryrun-patch"},
			Data:       map[string]string{"k": "before"},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := client.CoreV1().ConfigMaps(ns).Patch(ctx, "dryrun-patch", types.MergePatchType,
			[]byte(`{"data":{"k":"after"}}`), metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}}); err != nil {
			t.Fatalf("dry-run patch: %v", err)
		}
		got, err := client.CoreV1().ConfigMaps(ns).Get(ctx, "dryrun-patch", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Data["k"] != "before" {
			t.Errorf("dry-run patch persisted: data.k = %q, want before", got.Data["k"])
		}
	})

	// A namespace create is the one with post-create side effects
	// (default ServiceAccount, kube-root-ca.crt ConfigMap); a dry-run
	// must not leave them behind either.
	t.Run("createNamespaceSideEffects", func(t *testing.T) {
		const dryNS = "dryrunwrites-ns"
		if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: dryNS},
		}, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}}); err != nil {
			t.Fatalf("dry-run namespace create: %v", err)
		}
		t.Cleanup(func() {
			_ = client.CoreV1().Namespaces().Delete(context.Background(), dryNS, metav1.DeleteOptions{})
		})
		if _, err := client.CoreV1().Namespaces().Get(ctx, dryNS, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Errorf("dry-run namespace create persisted: get returned %v, want NotFound", err)
		}
		if _, err := client.CoreV1().ServiceAccounts(dryNS).Get(ctx, "default", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Errorf("dry-run namespace create left a default ServiceAccount: %v", err)
		}
	})

	// Upstream rejects a dryRun value other than "All" rather than
	// treating it as a real write.
	t.Run("invalidValueRejected", func(t *testing.T) {
		_, err := client.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "dryrun-bogus"},
		}, metav1.CreateOptions{DryRun: []string{"Maybe"}})
		if err == nil {
			t.Fatalf("create with dryRun=Maybe was accepted")
		}
		if _, getErr := client.CoreV1().ConfigMaps(ns).Get(ctx, "dryrun-bogus", metav1.GetOptions{}); !apierrors.IsNotFound(getErr) {
			t.Errorf("create with dryRun=Maybe persisted the object: %v", getErr)
		}
	})
}
