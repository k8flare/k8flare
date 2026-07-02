package apiserver_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	testClient *kubernetes.Clientset
	testOnce   sync.Once
	testPort   int
	devCmd     *exec.Cmd
)

func setupWranglerDev(t *testing.T) *kubernetes.Clientset {
	t.Helper()
	testOnce.Do(func() {
		testPort = findFreePort(t)
		projectRoot := findProjectRoot(t)

		devCmd = exec.Command("npx", "wrangler", "dev",
			"-c", "workers/gateway/wrangler.jsonc",
			"-c", "workers/storage/wrangler.jsonc",
			"-c", "workers/apiserver/wrangler.jsonc",
			"-c", "workers/runtime/wrangler.jsonc",
			"--port", fmt.Sprintf("%d", testPort),
			"--persist-to", filepath.Join(projectRoot, ".wrangler", "state"),
			"--log-level", "error",
		)
		devCmd.Dir = projectRoot
		devNull, _ := os.Open(os.DevNull)
		devCmd.Stdout = devNull
		devCmd.Stderr = devNull
		// wrangler dev's actual workerd process is a grandchild, not a direct
		// child of devCmd -- signaling devCmd.Process alone leaves it running.
		// Put the whole tree in its own process group so cleanup can signal
		// all of it at once.
		devCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		if err := devCmd.Start(); err != nil {
			t.Fatalf("Failed to start wrangler dev: %v", err)
		}

		// Wait for server to be ready
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", testPort), 500*time.Millisecond)
			if err == nil {
				conn.Close()
				// Give it a moment to fully initialize
				time.Sleep(2 * time.Second)
				break
			}
			time.Sleep(1 * time.Second)
		}

		var err error
		testClient, err = kubernetes.NewForConfig(&rest.Config{
			Host:        fmt.Sprintf("http://127.0.0.1:%d", testPort),
			BearerToken: "k8flare-dev-token",
		})
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
	})

	t.Cleanup(func() {
		// Only kill on last test - handled by TestMain
	})

	return testClient
}

func TestMain(m *testing.M) {
	code := m.Run()
	if devCmd != nil && devCmd.Process != nil {
		pgid := devCmd.Process.Pid
		syscall.Kill(-pgid, syscall.SIGINT)
		done := make(chan struct{})
		go func() {
			devCmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}
	os.Exit(code)
}

func findFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("Could not find project root (go.mod) from working directory")
	return ""
}

func findPodScheduledCondition(pod *corev1.Pod) *corev1.PodCondition {
	for i := range pod.Status.Conditions {
		if pod.Status.Conditions[i].Type == corev1.PodScheduled {
			return &pod.Status.Conditions[i]
		}
	}
	return nil
}

func TestDiscovery(t *testing.T) {
	client := setupWranglerDev(t)

	t.Run("ServerVersion", func(t *testing.T) {
		ver, err := client.Discovery().ServerVersion()
		if err != nil {
			t.Fatalf("ServerVersion: %v", err)
		}
		if ver.Major != "1" {
			t.Errorf("Expected major=1, got %s", ver.Major)
		}
		t.Logf("Server: %s", ver.GitVersion)
	})

	t.Run("APIResources", func(t *testing.T) {
		resources, err := client.Discovery().ServerResourcesForGroupVersion("v1")
		if err != nil {
			t.Fatalf("ServerResourcesForGroupVersion(v1): %v", err)
		}

		expected := map[string]bool{"namespaces": false, "configmaps": false, "secrets": false}
		for _, r := range resources.APIResources {
			if _, ok := expected[r.Name]; ok {
				expected[r.Name] = true
			}
		}
		for name, found := range expected {
			if !found {
				t.Errorf("Resource %q not found in discovery", name)
			}
		}
	})
}

func TestNamespaceCRUD(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	nsName := "test-ns-crud"

	_ = client.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})

	t.Run("Create", func(t *testing.T) {
		ns, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: nsName},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if ns.Name != nsName {
			t.Errorf("Name: got %q, want %q", ns.Name, nsName)
		}
		if ns.UID == "" {
			t.Error("UID should be set")
		}
		if ns.ResourceVersion == "" {
			t.Error("ResourceVersion should be set")
		}
		if ns.CreationTimestamp.IsZero() {
			t.Error("CreationTimestamp should be set")
		}
	})

	t.Run("Get", func(t *testing.T) {
		ns, err := client.CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if ns.Name != nsName {
			t.Errorf("Name: got %q, want %q", ns.Name, nsName)
		}
	})

	t.Run("List", func(t *testing.T) {
		list, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		found := false
		for _, ns := range list.Items {
			if ns.Name == nsName {
				found = true
			}
		}
		if !found {
			t.Errorf("Namespace %q not in list (%d items)", nsName, len(list.Items))
		}
	})

	t.Run("Update", func(t *testing.T) {
		ns, err := client.CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Get for update: %v", err)
		}
		ns.Labels = map[string]string{"env": "test"}
		updated, err := client.CoreV1().Namespaces().Update(ctx, ns, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Labels["env"] != "test" {
			t.Errorf("Labels: got %v, want env=test", updated.Labels)
		}
		if updated.ResourceVersion == ns.ResourceVersion {
			t.Error("ResourceVersion should change after update")
		}
	})

	t.Run("CreateDuplicate", func(t *testing.T) {
		_, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: nsName},
		}, metav1.CreateOptions{})
		if !errors.IsAlreadyExists(err) {
			t.Errorf("Expected AlreadyExists, got: %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := client.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err = client.CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{})
		if !errors.IsNotFound(err) {
			t.Errorf("Expected NotFound after delete, got: %v", err)
		}
	})
}

func TestDefaultServiceAccountAutoProvision(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	nsName := "test-sa-provision"

	_ = client.CoreV1().ServiceAccounts(nsName).Delete(ctx, "default", metav1.DeleteOptions{})
	_ = client.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})

	_, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: nsName},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create namespace: %v", err)
	}

	// Synchronous (unlike the scheduler's DO-alarm path) — the SA should
	// already exist by the time Create() above returned. This loop is
	// defensive/style-consistent, not an eventual-consistency wait.
	var sa *corev1.ServiceAccount
	for i := 0; i < 20; i++ {
		sa, err = client.CoreV1().ServiceAccounts(nsName).Get(ctx, "default", metav1.GetOptions{})
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("default ServiceAccount was not created in namespace %q: %v", nsName, err)
	}
	if sa.Name != "default" || sa.Namespace != nsName {
		t.Errorf("got %s/%s, want default/%s", sa.Namespace, sa.Name, nsName)
	}

	client.CoreV1().ServiceAccounts(nsName).Delete(ctx, "default", metav1.DeleteOptions{})
	client.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})
}

// TestRootCAConfigMapAutoProvision covers the kube-root-ca.crt ConfigMap that
// every namespaced official e2e test waits for in its BeforeEach — without
// it, every namespaced conformance test times out before its own logic ever
// runs, regardless of what it's actually testing.
func TestRootCAConfigMapAutoProvision(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	nsName := "test-rootca-provision"

	_ = client.CoreV1().ConfigMaps(nsName).Delete(ctx, "kube-root-ca.crt", metav1.DeleteOptions{})
	_ = client.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})

	_, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: nsName},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create namespace: %v", err)
	}

	var cm *corev1.ConfigMap
	for i := 0; i < 20; i++ {
		cm, err = client.CoreV1().ConfigMaps(nsName).Get(ctx, "kube-root-ca.crt", metav1.GetOptions{})
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("kube-root-ca.crt ConfigMap was not created in namespace %q: %v", nsName, err)
	}
	if cm.Name != "kube-root-ca.crt" || cm.Namespace != nsName {
		t.Errorf("got %s/%s, want kube-root-ca.crt/%s", cm.Namespace, cm.Name, nsName)
	}
	if _, ok := cm.Data["ca.crt"]; !ok {
		t.Error("expected ca.crt key in kube-root-ca.crt ConfigMap data")
	}

	client.CoreV1().ConfigMaps(nsName).Delete(ctx, "kube-root-ca.crt", metav1.DeleteOptions{})
	client.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})
}

func TestNamespaceCascadingDelete(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	nsName := "test-ns-cascade"
	podName := "test-cascade-pod"
	cmName := "test-cascade-cm"

	_ = client.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})

	t.Run("SweepsDependents", func(t *testing.T) {
		if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: nsName},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("Create namespace: %v", err)
		}
		if _, err := client.CoreV1().Pods(nsName).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: nsName},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("Create pod: %v", err)
		}
		if _, err := client.CoreV1().ConfigMaps(nsName).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: nsName},
			Data:       map[string]string{"k": "v"},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("Create configmap: %v", err)
		}

		if err := client.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("Delete namespace: %v", err)
		}

		if _, err := client.CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{}); !errors.IsNotFound(err) {
			t.Errorf("Expected namespace NotFound, got: %v", err)
		}
		if _, err := client.CoreV1().Pods(nsName).Get(ctx, podName, metav1.GetOptions{}); !errors.IsNotFound(err) {
			t.Errorf("Expected pod NotFound after cascade delete, got: %v", err)
		}
		if _, err := client.CoreV1().ConfigMaps(nsName).Get(ctx, cmName, metav1.GetOptions{}); !errors.IsNotFound(err) {
			t.Errorf("Expected configmap NotFound after cascade delete, got: %v", err)
		}
	})

	t.Run("RecreateAfterCascadeDelete", func(t *testing.T) {
		// Directly closes the friction found this session: recreating a
		// namespace + pod with the same names must succeed, not AlreadyExists.
		if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: nsName},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("Recreate namespace: %v", err)
		}
		if _, err := client.CoreV1().Pods(nsName).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: nsName},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("Recreate pod after cascade delete: %v", err)
		}
		_ = client.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})
	})
}

func TestConfigMapCRUD(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"
	name := "test-cm-crud"

	// Ensure default namespace exists
	client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	_ = client.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{})

	t.Run("Create", func(t *testing.T) {
		cm, err := client.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Data:       map[string]string{"key1": "val1", "key2": "val2"},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if cm.Data["key1"] != "val1" {
			t.Errorf("Data: got %v", cm.Data)
		}
	})

	t.Run("Get", func(t *testing.T) {
		cm, err := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if cm.Data["key1"] != "val1" || cm.Data["key2"] != "val2" {
			t.Errorf("Data mismatch: %v", cm.Data)
		}
	})

	t.Run("Update", func(t *testing.T) {
		cm, _ := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
		cm.Data["key3"] = "val3"
		updated, err := client.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Data["key3"] != "val3" {
			t.Errorf("Update data: got %v", updated.Data)
		}
	})

	t.Run("ConflictUpdate", func(t *testing.T) {
		cm, _ := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
		// First update succeeds
		cm.Data["x"] = "y"
		_, err := client.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("First update: %v", err)
		}
		// Second update with stale RV should conflict
		cm.Data["x"] = "z"
		_, err = client.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{})
		if !errors.IsConflict(err) {
			t.Errorf("Expected Conflict, got: %v", err)
		}
	})

	t.Run("UpdateOmitsUID", func(t *testing.T) {
		cm, _ := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
		originalUID := cm.UID
		if originalUID == "" {
			t.Fatalf("original UID is empty, test is meaningless")
		}
		// A raw PUT that omits uid/creationTimestamp (as a minimal hand-built
		// body would) must not clobber them -- the store should fill in the
		// existing values, matching upstream ObjectMeta update semantics.
		cm.UID = ""
		cm.CreationTimestamp = metav1.Time{}
		cm.Data["uidtest"] = "1"
		updated, err := client.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.UID != originalUID {
			t.Errorf("UID: got %q, want preserved %q", updated.UID, originalUID)
		}
	})

	t.Run("UpdateMismatchedUID", func(t *testing.T) {
		cm, _ := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
		cm.UID = types.UID("wrong-uid-should-be-rejected")
		_, err := client.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{})
		if !errors.IsInvalid(err) {
			t.Errorf("Expected Invalid, got: %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := client.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err = client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
		if !errors.IsNotFound(err) {
			t.Errorf("Expected NotFound, got: %v", err)
		}
	})
}

func TestLimitRangeCRUD(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"
	name := "test-lr-crud"

	client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	_ = client.CoreV1().LimitRanges(ns).Delete(ctx, name, metav1.DeleteOptions{})

	t.Run("Create", func(t *testing.T) {
		lr, err := client.CoreV1().LimitRanges(ns).Create(ctx, &corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: corev1.LimitRangeSpec{
				Limits: []corev1.LimitRangeItem{
					{
						Type:    corev1.LimitTypeContainer,
						Default: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
					},
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if len(lr.Spec.Limits) != 1 {
			t.Errorf("Limits: got %v", lr.Spec.Limits)
		}
	})

	t.Run("Get", func(t *testing.T) {
		lr, err := client.CoreV1().LimitRanges(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if lr.Name != name {
			t.Errorf("Name: got %q, want %q", lr.Name, name)
		}
	})

	t.Run("List", func(t *testing.T) {
		list, err := client.CoreV1().LimitRanges(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		found := false
		for _, lr := range list.Items {
			if lr.Name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("LimitRange %q not in list (%d items)", name, len(list.Items))
		}
	})

	t.Run("Update", func(t *testing.T) {
		lr, _ := client.CoreV1().LimitRanges(ns).Get(ctx, name, metav1.GetOptions{})
		lr.Labels = map[string]string{"env": "test"}
		updated, err := client.CoreV1().LimitRanges(ns).Update(ctx, lr, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Labels["env"] != "test" {
			t.Errorf("Labels: got %v, want env=test", updated.Labels)
		}
	})

	t.Run("ListWithLabelSelector", func(t *testing.T) {
		ns2 := "test-lr-crud-ns2"
		client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns2},
		}, metav1.CreateOptions{})
		_ = client.CoreV1().LimitRanges(ns2).Delete(ctx, "lr-in-ns2", metav1.DeleteOptions{})

		selectorLabels := map[string]string{"lr-selector-test": "yes"}
		client.CoreV1().LimitRanges(ns).Update(ctx, &corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: selectorLabels},
			Spec:       corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{Type: corev1.LimitTypeContainer}}},
		}, metav1.UpdateOptions{})
		if _, err := client.CoreV1().LimitRanges(ns2).Create(ctx, &corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: "lr-in-ns2", Namespace: ns2, Labels: selectorLabels},
			Spec:       corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{Type: corev1.LimitTypeContainer}}},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("Create in ns2: %v", err)
		}

		list, err := client.CoreV1().LimitRanges(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
			LabelSelector: "lr-selector-test=yes",
		})
		if err != nil {
			t.Fatalf("List with labelSelector: %v", err)
		}
		if len(list.Items) != 2 {
			t.Errorf("Expected 2 LimitRanges across namespaces matching selector, got %d", len(list.Items))
		}

		t.Run("DeleteCollection", func(t *testing.T) {
			err := client.CoreV1().LimitRanges(ns2).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
				LabelSelector: "lr-selector-test=yes",
			})
			if err != nil {
				t.Fatalf("DeleteCollection: %v", err)
			}
			if _, err := client.CoreV1().LimitRanges(ns2).Get(ctx, "lr-in-ns2", metav1.GetOptions{}); !errors.IsNotFound(err) {
				t.Errorf("Expected lr-in-ns2 NotFound after DeleteCollection, got: %v", err)
			}
			// The matching LimitRange in ns (not ns2) must survive — DeleteCollection is scoped to ns2 only.
			if _, err := client.CoreV1().LimitRanges(ns).Get(ctx, name, metav1.GetOptions{}); err != nil {
				t.Errorf("Expected %s in namespace %s to survive DeleteCollection scoped to %s, got: %v", name, ns, ns2, err)
			}
		})
	})

	t.Run("Delete", func(t *testing.T) {
		err := client.CoreV1().LimitRanges(ns).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err = client.CoreV1().LimitRanges(ns).Get(ctx, name, metav1.GetOptions{})
		if !errors.IsNotFound(err) {
			t.Errorf("Expected NotFound, got: %v", err)
		}
	})
}

func TestPodLimitRangeDefaultingE2E(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "test-pod-limitrange-defaulting"
	podName := "test-lr-default-pod"

	_ = client.CoreV1().Pods(ns).Delete(ctx, podName, metav1.DeleteOptions{})
	_ = client.CoreV1().LimitRanges(ns).Delete(ctx, "defaults", metav1.DeleteOptions{})
	_ = client.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})

	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create namespace: %v", err)
	}
	if _, err := client.CoreV1().LimitRanges(ns).Create(ctx, &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: "defaults", Namespace: ns},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("500Mi"),
					},
					DefaultRequest: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
				},
			},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create LimitRange: %v", err)
	}

	pod, err := client.CoreV1().Pods(ns).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: ns},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "test", Image: "busybox"}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create pod: %v", err)
	}

	c := pod.Spec.Containers[0]
	if got := c.Resources.Limits[corev1.ResourceCPU]; got.Cmp(resource.MustParse("500m")) != 0 {
		t.Errorf("Limits[cpu] = %v, want 500m", got.String())
	}
	if got := c.Resources.Requests[corev1.ResourceCPU]; got.Cmp(resource.MustParse("100m")) != 0 {
		t.Errorf("Requests[cpu] = %v, want 100m", got.String())
	}

	client.CoreV1().Pods(ns).Delete(ctx, podName, metav1.DeleteOptions{})
	client.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
}

func TestSecretCRUD(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"
	name := "test-secret-crud"

	client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	_ = client.CoreV1().Secrets(ns).Delete(ctx, name, metav1.DeleteOptions{})

	t.Run("Create", func(t *testing.T) {
		secret, err := client.CoreV1().Secrets(ns).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{"user": []byte("admin"), "pass": []byte("s3cret")},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if string(secret.Data["user"]) != "admin" {
			t.Errorf("Data: got %v", secret.Data)
		}
		if secret.Type != corev1.SecretTypeOpaque {
			t.Errorf("Type: got %s", secret.Type)
		}
	})

	t.Run("Get", func(t *testing.T) {
		secret, err := client.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if string(secret.Data["pass"]) != "s3cret" {
			t.Error("Password data mismatch")
		}
	})

	t.Run("Update", func(t *testing.T) {
		secret, _ := client.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		secret.Data["token"] = []byte("new-token")
		updated, err := client.CoreV1().Secrets(ns).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if string(updated.Data["token"]) != "new-token" {
			t.Error("Update token mismatch")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := client.CoreV1().Secrets(ns).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err = client.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		if !errors.IsNotFound(err) {
			t.Errorf("Expected NotFound, got: %v", err)
		}
	})
}

func TestNotFoundErrors(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()

	t.Run("Namespace", func(t *testing.T) {
		_, err := client.CoreV1().Namespaces().Get(ctx, "nonexistent-12345", metav1.GetOptions{})
		if !errors.IsNotFound(err) {
			t.Errorf("Expected NotFound, got: %v", err)
		}
	})

	t.Run("ConfigMap", func(t *testing.T) {
		_, err := client.CoreV1().ConfigMaps("default").Get(ctx, "nonexistent-12345", metav1.GetOptions{})
		if !errors.IsNotFound(err) {
			t.Errorf("Expected NotFound, got: %v", err)
		}
	})

	t.Run("Secret", func(t *testing.T) {
		_, err := client.CoreV1().Secrets("default").Get(ctx, "nonexistent-12345", metav1.GetOptions{})
		if !errors.IsNotFound(err) {
			t.Errorf("Expected NotFound, got: %v", err)
		}
	})
}

func TestPodCRUD(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"
	name := "test-pod-crud"

	client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	_ = client.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{})

	t.Run("Create", func(t *testing.T) {
		pod, err := client.CoreV1().Pods(ns).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if pod.Name != name {
			t.Errorf("Name: got %q", pod.Name)
		}
		if len(pod.Spec.Containers) != 1 || pod.Spec.Containers[0].Image != "nginx" {
			t.Errorf("Containers: got %v", pod.Spec.Containers)
		}
	})

	t.Run("Get", func(t *testing.T) {
		pod, err := client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if pod.Name != name {
			t.Errorf("Name: got %q", pod.Name)
		}
	})

	t.Run("List", func(t *testing.T) {
		list, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		found := false
		for _, p := range list.Items {
			if p.Name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("Pod %q not in list (%d items)", name, len(list.Items))
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		pod, err := client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		pod.Status.Phase = corev1.PodRunning
		updated, err := client.CoreV1().Pods(ns).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		if updated.Status.Phase != corev1.PodRunning {
			t.Errorf("Phase: got %s", updated.Status.Phase)
		}
	})

	t.Run("Patch", func(t *testing.T) {
		patch := []byte(`{"metadata":{"labels":{"app":"nginx"}}}`)
		patched, err := client.CoreV1().Pods(ns).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			t.Fatalf("Patch: %v", err)
		}
		if patched.Labels["app"] != "nginx" {
			t.Errorf("Labels: got %v", patched.Labels)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := client.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err = client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if !errors.IsNotFound(err) {
			t.Errorf("Expected NotFound, got: %v", err)
		}
	})
}

func TestNodeCRUD(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	name := "test-node-crud"

	_ = client.CoreV1().Nodes().Delete(ctx, name, metav1.DeleteOptions{})

	t.Run("Create", func(t *testing.T) {
		node, err := client.CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if node.Name != name {
			t.Errorf("Name: got %q", node.Name)
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		node, err := client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		node.Status.Conditions = []corev1.NodeCondition{
			{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
		}
		updated, err := client.CoreV1().Nodes().UpdateStatus(ctx, node, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		if len(updated.Status.Conditions) == 0 || updated.Status.Conditions[0].Status != corev1.ConditionTrue {
			t.Errorf("Conditions: got %v", updated.Status.Conditions)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := client.CoreV1().Nodes().Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
	})
}

// TestWorkloadStatusSubresources covers the /status subresource added for
// ReplicaSet, Deployment, DaemonSet, Job, and CronJob -- prerequisite work
// for embedding the real kube-controller-manager, whose typed clients call
// UpdateStatus() (hitting these exact subresources) rather than a
// whole-object Update().
func TestWorkloadStatusSubresources(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"

	client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})

	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "status-sub-test"}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
		},
	}

	t.Run("ReplicaSet", func(t *testing.T) {
		name := "status-sub-rs"
		_ = client.AppsV1().ReplicaSets(ns).Delete(ctx, name, metav1.DeleteOptions{})
		rs, err := client.AppsV1().ReplicaSets(ns).Create(ctx, &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: appsv1.ReplicaSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "status-sub-test"}},
				Template: podTemplate,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		rs.Status.Replicas = 5
		rs.Status.ReadyReplicas = 3
		updated, err := client.AppsV1().ReplicaSets(ns).UpdateStatus(ctx, rs, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		if updated.Status.Replicas != 5 || updated.Status.ReadyReplicas != 3 {
			t.Errorf("Status: got %+v", updated.Status)
		}
	})

	t.Run("Deployment", func(t *testing.T) {
		name := "status-sub-dep"
		_ = client.AppsV1().Deployments(ns).Delete(ctx, name, metav1.DeleteOptions{})
		dep, err := client.AppsV1().Deployments(ns).Create(ctx, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "status-sub-test"}},
				Template: podTemplate,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		dep.Status.Replicas = 3
		dep.Status.AvailableReplicas = 2
		updated, err := client.AppsV1().Deployments(ns).UpdateStatus(ctx, dep, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		if updated.Status.Replicas != 3 || updated.Status.AvailableReplicas != 2 {
			t.Errorf("Status: got %+v", updated.Status)
		}
	})

	t.Run("DaemonSet", func(t *testing.T) {
		name := "status-sub-ds"
		_ = client.AppsV1().DaemonSets(ns).Delete(ctx, name, metav1.DeleteOptions{})
		ds, err := client.AppsV1().DaemonSets(ns).Create(ctx, &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "status-sub-test"}},
				Template: podTemplate,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		ds.Status.DesiredNumberScheduled = 3
		ds.Status.NumberReady = 1
		updated, err := client.AppsV1().DaemonSets(ns).UpdateStatus(ctx, ds, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		if updated.Status.DesiredNumberScheduled != 3 || updated.Status.NumberReady != 1 {
			t.Errorf("Status: got %+v", updated.Status)
		}
	})

	t.Run("Job", func(t *testing.T) {
		name := "status-sub-job"
		_ = client.BatchV1().Jobs(ns).Delete(ctx, name, metav1.DeleteOptions{})
		jobPodTemplate := podTemplate
		jobPodTemplate.Spec.RestartPolicy = corev1.RestartPolicyNever
		job, err := client.BatchV1().Jobs(ns).Create(ctx, &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       batchv1.JobSpec{Template: jobPodTemplate},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		job.Status.Active = 2
		job.Status.Succeeded = 1
		updated, err := client.BatchV1().Jobs(ns).UpdateStatus(ctx, job, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		if updated.Status.Active != 2 || updated.Status.Succeeded != 1 {
			t.Errorf("Status: got %+v", updated.Status)
		}
	})

	t.Run("CronJob", func(t *testing.T) {
		name := "status-sub-cj"
		_ = client.BatchV1().CronJobs(ns).Delete(ctx, name, metav1.DeleteOptions{})
		jobPodTemplate := podTemplate
		jobPodTemplate.Spec.RestartPolicy = corev1.RestartPolicyNever
		cj, err := client.BatchV1().CronJobs(ns).Create(ctx, &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: batchv1.CronJobSpec{
				// Never actually due -- this test only exercises the status
				// subresource, not scheduling.
				Schedule:    "0 0 30 2 *",
				JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{Template: jobPodTemplate}},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		now := metav1.Now()
		cj.Status.LastScheduleTime = &now
		updated, err := client.BatchV1().CronJobs(ns).UpdateStatus(ctx, cj, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		if updated.Status.LastScheduleTime == nil {
			t.Errorf("Status.LastScheduleTime: got nil")
		}
	})
}

func TestLeaseCRUD(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "kube-node-lease"
	name := "test-lease-crud"

	client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	_ = client.CoordinationV1().Leases(ns).Delete(ctx, name, metav1.DeleteOptions{})

	t.Run("Create", func(t *testing.T) {
		dur := int32(40)
		lease, err := client.CoordinationV1().Leases(ns).Create(ctx, &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: coordinationv1.LeaseSpec{
				LeaseDurationSeconds: &dur,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if lease.Name != name {
			t.Errorf("Name: got %q", lease.Name)
		}
	})

	t.Run("Get", func(t *testing.T) {
		lease, err := client.CoordinationV1().Leases(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if *lease.Spec.LeaseDurationSeconds != 40 {
			t.Errorf("LeaseDuration: got %d", *lease.Spec.LeaseDurationSeconds)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := client.CoordinationV1().Leases(ns).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
	})
}

// TestResourceAPIGroup covers the resource.k8s.io/v1 stub types
// (ResourceClaim, ResourceSlice) registered so a real kube-scheduler's
// Dynamic Resource Allocation informers can sync against an empty list
// instead of hanging in WaitForCacheSync, and locks in that events.k8s.io/v1
// is intentionally not discoverable (see docs/control-plane-architecture.md
// for why: client-go's EventBroadcasterAdapter would otherwise prefer it,
// and this server's Event storage always round-trips as core v1 Event).
func TestResourceAPIGroup(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()

	t.Run("Discovery", func(t *testing.T) {
		resources, err := client.Discovery().ServerResourcesForGroupVersion("resource.k8s.io/v1")
		if err != nil {
			t.Fatalf("ServerResourcesForGroupVersion(resource.k8s.io/v1): %v", err)
		}
		expected := map[string]bool{"resourceclaims": false, "resourceslices": false}
		for _, r := range resources.APIResources {
			if _, ok := expected[r.Name]; ok {
				expected[r.Name] = true
			}
		}
		for name, found := range expected {
			if !found {
				t.Errorf("Resource %q not found in discovery", name)
			}
		}
	})

	t.Run("EventsGroupNotDiscoverable", func(t *testing.T) {
		if _, err := client.Discovery().ServerResourcesForGroupVersion("events.k8s.io/v1"); err == nil {
			t.Error("expected events.k8s.io/v1 to be undiscoverable, got no error")
		}
	})

	t.Run("ResourceClaimCRUD", func(t *testing.T) {
		ns := "default"
		name := "test-resourceclaim-crud"
		_ = client.ResourceV1().ResourceClaims(ns).Delete(ctx, name, metav1.DeleteOptions{})

		claim, err := client.ResourceV1().ResourceClaims(ns).Create(ctx, &resourcev1.ResourceClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if claim.Name != name {
			t.Errorf("Name: got %q", claim.Name)
		}

		if _, err := client.ResourceV1().ResourceClaims(ns).Get(ctx, name, metav1.GetOptions{}); err != nil {
			t.Fatalf("Get: %v", err)
		}

		if err := client.ResourceV1().ResourceClaims(ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	})

	t.Run("ResourceSliceCRUD", func(t *testing.T) {
		name := "test-resourceslice-crud"
		_ = client.ResourceV1().ResourceSlices().Delete(ctx, name, metav1.DeleteOptions{})

		slice, err := client.ResourceV1().ResourceSlices().Create(ctx, &resourcev1.ResourceSlice{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: resourcev1.ResourceSliceSpec{
				Driver: "test.example.com",
				Pool: resourcev1.ResourcePool{
					Name:               "test-pool",
					Generation:         1,
					ResourceSliceCount: 1,
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if slice.Name != name {
			t.Errorf("Name: got %q", slice.Name)
		}

		if _, err := client.ResourceV1().ResourceSlices().Get(ctx, name, metav1.GetOptions{}); err != nil {
			t.Fatalf("Get: %v", err)
		}

		if err := client.ResourceV1().ResourceSlices().Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	})
}

func TestSupervisorCACerts(t *testing.T) {
	setupWranglerDev(t) // ensure server is running

	// GET /cacerts - unauthenticated
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/cacerts", testPort))
	if err != nil {
		t.Fatalf("GET /cacerts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Expected Content-Type text/plain, got %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Read body: %v", err)
	}

	// /cacerts returns an empty body so the k3s agent falls back to system CAs.
	// This is intentional: Cloudflare terminates TLS with a publicly trusted cert,
	// so the agent doesn't need a custom CA for the server connection.
	if len(body) != 0 {
		t.Fatalf("Expected empty body, got %d bytes", len(body))
	}
}

func TestSupervisorConfig(t *testing.T) {
	setupWranglerDev(t)

	// GET /v1-k3s/config with Basic Auth (node:token)
	req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/v1-k3s/config", testPort), nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.SetBasicAuth("node", "k8flare-dev-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1-k3s/config: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var config map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		t.Fatalf("Decode JSON: %v", err)
	}

	if domain, ok := config["ClusterDomain"].(string); !ok || domain != "cluster.local" {
		t.Errorf("Expected ClusterDomain=cluster.local, got %v", config["ClusterDomain"])
	}
	// JSON numbers decode as float64
	if port, ok := config["HTTPSPort"].(float64); !ok || int(port) != 6443 {
		t.Errorf("Expected HTTPSPort=6443, got %v", config["HTTPSPort"])
	}
	if npc, ok := config["DisableNPC"].(bool); !ok || !npc {
		t.Errorf("Expected DisableNPC=true, got %v", config["DisableNPC"])
	}

	// Test without auth returns 401
	resp2, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/v1-k3s/config", testPort))
	if err != nil {
		t.Fatalf("GET /v1-k3s/config (no auth): %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401 without auth, got %d", resp2.StatusCode)
	}
}

func TestSupervisorCertSigning(t *testing.T) {
	setupWranglerDev(t)

	// Generate a CSR for testing
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "test-node"},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, key)
	if err != nil {
		t.Fatalf("CreateCertificateRequest: %v", err)
	}

	parsePEMCert := func(t *testing.T, body []byte) *x509.Certificate {
		t.Helper()
		block, _ := pem.Decode(body)
		if block == nil {
			t.Fatal("Failed to decode PEM block from response")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("ParseCertificate: %v", err)
		}
		return cert
	}

	t.Run("ServingKubelet", func(t *testing.T) {
		req, err := http.NewRequest("POST",
			fmt.Sprintf("http://127.0.0.1:%d/v1-k3s/serving-kubelet.crt", testPort),
			bytes.NewReader(csrDER))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.SetBasicAuth("node", "k8flare-dev-token")
		req.Header.Set("K3s-Node-Name", "test-node")
		req.Header.Set("K3s-Node-Password", "test-password-123")
		req.Header.Set("K3s-Node-IP", "192.168.1.100")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST serving-kubelet.crt: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Read body: %v", err)
		}

		cert := parsePEMCert(t, body)

		if cert.Subject.CommonName != "test-node" {
			t.Errorf("Expected CN=test-node, got %q", cert.Subject.CommonName)
		}

		hasServerAuth := false
		for _, usage := range cert.ExtKeyUsage {
			if usage == x509.ExtKeyUsageServerAuth {
				hasServerAuth = true
			}
		}
		if !hasServerAuth {
			t.Error("Expected ExtKeyUsage to contain ServerAuth")
		}

		// Check SANs
		expectedDNS := map[string]bool{"test-node": false, "localhost": false}
		for _, dns := range cert.DNSNames {
			if _, ok := expectedDNS[dns]; ok {
				expectedDNS[dns] = true
			}
		}
		for name, found := range expectedDNS {
			if !found {
				t.Errorf("Expected SAN DNS name %q not found in %v", name, cert.DNSNames)
			}
		}

		foundIP := false
		for _, ip := range cert.IPAddresses {
			if ip.String() == "192.168.1.100" {
				foundIP = true
			}
		}
		if !foundIP {
			t.Errorf("Expected SAN IP 192.168.1.100 not found in %v", cert.IPAddresses)
		}
	})

	t.Run("ClientKubelet", func(t *testing.T) {
		req, err := http.NewRequest("POST",
			fmt.Sprintf("http://127.0.0.1:%d/v1-k3s/client-kubelet.crt", testPort),
			bytes.NewReader(csrDER))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.SetBasicAuth("node", "k8flare-dev-token")
		req.Header.Set("K3s-Node-Name", "test-node")
		req.Header.Set("K3s-Node-Password", "test-password-123")
		req.Header.Set("K3s-Node-IP", "192.168.1.100")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST client-kubelet.crt: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Read body: %v", err)
		}

		cert := parsePEMCert(t, body)

		if cert.Subject.CommonName != "system:node:test-node" {
			t.Errorf("Expected CN=system:node:test-node, got %q", cert.Subject.CommonName)
		}

		foundOrg := false
		for _, org := range cert.Subject.Organization {
			if org == "system:nodes" {
				foundOrg = true
			}
		}
		if !foundOrg {
			t.Errorf("Expected Org to contain system:nodes, got %v", cert.Subject.Organization)
		}

		hasClientAuth := false
		for _, usage := range cert.ExtKeyUsage {
			if usage == x509.ExtKeyUsageClientAuth {
				hasClientAuth = true
			}
		}
		if !hasClientAuth {
			t.Error("Expected ExtKeyUsage to contain ClientAuth")
		}
	})

	t.Run("ClientKubeProxy", func(t *testing.T) {
		req, err := http.NewRequest("POST",
			fmt.Sprintf("http://127.0.0.1:%d/v1-k3s/client-kube-proxy.crt", testPort),
			bytes.NewReader(csrDER))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.SetBasicAuth("node", "k8flare-dev-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST client-kube-proxy.crt: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Read body: %v", err)
		}

		cert := parsePEMCert(t, body)

		if cert.Subject.CommonName != "system:kube-proxy" {
			t.Errorf("Expected CN=system:kube-proxy, got %q", cert.Subject.CommonName)
		}

		hasClientAuth := false
		for _, usage := range cert.ExtKeyUsage {
			if usage == x509.ExtKeyUsageClientAuth {
				hasClientAuth = true
			}
		}
		if !hasClientAuth {
			t.Error("Expected ExtKeyUsage to contain ClientAuth")
		}
	})
}

func TestSupervisorAPIServers(t *testing.T) {
	setupWranglerDev(t)

	req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/v1-k3s/apiservers", testPort), nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.SetBasicAuth("node", "k8flare-dev-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1-k3s/apiservers: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var servers []string
	if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
		t.Fatalf("Decode JSON: %v", err)
	}

	if len(servers) < 1 {
		t.Fatal("Expected at least 1 apiserver entry")
	}

	if !strings.HasPrefix(servers[0], "https://") {
		t.Errorf("Expected first entry to start with https://, got %q", servers[0])
	}
}

func TestSupervisorReadyz(t *testing.T) {
	setupWranglerDev(t)

	req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/v1-k3s/readyz", testPort), nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.SetBasicAuth("node", "k8flare-dev-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1-k3s/readyz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Read body: %v", err)
	}

	if strings.TrimSpace(string(body)) != "ok" {
		t.Errorf("Expected body 'ok', got %q", string(body))
	}
}

func TestBasicAuth(t *testing.T) {
	setupWranglerDev(t)
	ctx := context.Background()

	// Test that Basic Auth works for K8s API endpoints too
	basicClient, err := kubernetes.NewForConfig(&rest.Config{
		Host:     fmt.Sprintf("http://127.0.0.1:%d", testPort),
		Username: "node",
		Password: "k8flare-dev-token",
	})
	if err != nil {
		t.Fatalf("NewForConfig with basic auth: %v", err)
	}

	// Should be able to list namespaces
	nsList, err := basicClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("List namespaces with basic auth: %v", err)
	}

	if len(nsList.Items) == 0 {
		t.Error("Expected at least 1 namespace")
	}
}
