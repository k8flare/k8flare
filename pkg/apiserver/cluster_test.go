package apiserver_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	k8flarev1alpha1 "github.com/k8flare/k8flare/pkg/apis/k8flare/v1alpha1"
)

// clustersGVR is k8flare's own built-in group (pkg/apis/k8flare/v1alpha1,
// registered through apidef.Table). These tests drive it with the dynamic
// client rather than a typed one: there is no generated typed client for
// this group, and the dynamic client exercises exactly what kubectl does
// (discovery -> REST path -> JSON round-trip), which is the point of adding
// the group to the table at all.
var clustersGVR = schema.GroupVersionResource{
	Group: k8flarev1alpha1.GroupName, Version: "v1alpha1", Resource: "clusters",
}

func clusterClient(t *testing.T) dynamic.ResourceInterface {
	t.Helper()
	setupWranglerDev(t)
	dc, err := dynamic.NewForConfig(&rest.Config{
		Host:        fmt.Sprintf("http://127.0.0.1:%d", testPort),
		BearerToken: "k8flare-dev-token",
	})
	if err != nil {
		t.Fatalf("dynamic client: %v", err)
	}
	// Cluster is cluster-scoped, so no Namespace() call.
	return dc.Resource(clustersGVR)
}

func newCluster(name, displayName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "k8flare.com/v1alpha1",
		"kind":       "Cluster",
		"metadata":   map[string]interface{}{"name": name},
		"spec":       map[string]interface{}{"displayName": displayName},
	}}
}

func TestClusterCRUDAndWatch(t *testing.T) {
	ctx := context.Background()
	cl := clusterClient(t)

	name := fmt.Sprintf("crud-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = cl.Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	// Watch is started before the create so the create event is observed
	// live rather than raced against the initial list.
	w, err := cl.Watch(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("watch clusters: %v", err)
	}
	defer w.Stop()

	created, err := cl.Create(ctx, newCluster(name, "CRUD test"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if got := created.GetName(); got != name {
		t.Errorf("created name = %q, want %q", got, name)
	}
	if created.GetUID() == "" {
		t.Error("created cluster has no UID (upstream registry did not stamp one)")
	}

	got, err := cl.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if dn, _, _ := unstructured.NestedString(got.Object, "spec", "displayName"); dn != "CRUD test" {
		t.Errorf("spec.displayName = %q, want %q", dn, "CRUD test")
	}
	if kind := got.GetKind(); kind != "Cluster" {
		t.Errorf("kind = %q, want Cluster", kind)
	}

	list, err := cl.List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list clusters: %v", err)
	}
	if list.GetKind() != "ClusterList" {
		t.Errorf("list kind = %q, want ClusterList", list.GetKind())
	}
	if !containsCluster(list.Items, name) {
		t.Errorf("list does not contain %q", name)
	}

	if err := unstructured.SetNestedField(got.Object, "renamed", "spec", "displayName"); err != nil {
		t.Fatalf("set displayName: %v", err)
	}
	updated, err := cl.Update(ctx, got, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("update cluster: %v", err)
	}
	if dn, _, _ := unstructured.NestedString(updated.Object, "spec", "displayName"); dn != "renamed" {
		t.Errorf("after update spec.displayName = %q, want renamed", dn)
	}

	if err := cl.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete cluster: %v", err)
	}
	if _, err := cl.Get(ctx, name, metav1.GetOptions{}); !errors.IsNotFound(err) {
		t.Errorf("get after delete: err = %v, want NotFound", err)
	}

	awaitClusterEvent(t, w, watch.Added, name)
	awaitClusterEvent(t, w, watch.Deleted, name)
}

func containsCluster(items []unstructured.Unstructured, name string) bool {
	for i := range items {
		if items[i].GetName() == name {
			return true
		}
	}
	return false
}

// awaitClusterEvent waits for a watch event of the given type naming the
// given cluster, ignoring events for other objects (the suite runs against
// a shared dev server) and bookmarks.
func awaitClusterEvent(t *testing.T, w watch.Interface, want watch.EventType, name string) {
	t.Helper()
	deadline := time.After(30 * time.Second)
	for {
		select {
		case ev, ok := <-w.ResultChan():
			if !ok {
				t.Fatalf("watch channel closed before %s event for %q", want, name)
			}
			obj, isUnstructured := ev.Object.(*unstructured.Unstructured)
			if !isUnstructured || obj.GetName() != name {
				continue
			}
			if ev.Type == want {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s event for %q", want, name)
		}
	}
}

// TestClusterStatusSubresource checks that the generic status subresource
// path (pkg/apiserver/subresource.go, reflection over the object's Status
// field) works for a k8flare-owned type with no special-casing: a status
// write must land and must not disturb spec.
func TestClusterStatusSubresource(t *testing.T) {
	ctx := context.Background()
	cl := clusterClient(t)

	name := fmt.Sprintf("status-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = cl.Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	created, err := cl.Create(ctx, newCluster(name, "keep me"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	if err := unstructured.SetNestedMap(created.Object, map[string]interface{}{
		"phase":              "Ready",
		"doName":             name + "@abc123",
		"endpoint":           "https://example.invalid/c/" + name,
		"observedGeneration": int64(1),
		"tokenSecretRef": map[string]interface{}{
			"namespace": "k8flare-system", "name": "cluster-" + name,
		},
	}, "status"); err != nil {
		t.Fatalf("set status: %v", err)
	}
	// A status write must not be able to change spec: send a mutated spec
	// alongside it and assert the server ignored that half.
	if err := unstructured.SetNestedField(created.Object, "clobbered", "spec", "displayName"); err != nil {
		t.Fatalf("set displayName: %v", err)
	}

	if _, err := cl.UpdateStatus(ctx, created, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update cluster status: %v", err)
	}

	got, err := cl.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if phase, _, _ := unstructured.NestedString(got.Object, "status", "phase"); phase != "Ready" {
		t.Errorf("status.phase = %q, want Ready", phase)
	}
	if ep, _, _ := unstructured.NestedString(got.Object, "status", "endpoint"); ep != "https://example.invalid/c/"+name {
		t.Errorf("status.endpoint = %q, want the written value", ep)
	}
	if ns, _, _ := unstructured.NestedString(got.Object, "status", "tokenSecretRef", "namespace"); ns != "k8flare-system" {
		t.Errorf("status.tokenSecretRef.namespace = %q, want k8flare-system", ns)
	}
	if dn, _, _ := unstructured.NestedString(got.Object, "spec", "displayName"); dn != "keep me" {
		t.Errorf("spec.displayName = %q, want %q -- the status subresource must not write spec", dn, "keep me")
	}
}

// TestClusterProtectedDelete covers clusterprotect.go: the "default"
// Cluster is the management cluster and must not be deletable, while any
// other name deletes normally.
func TestClusterProtectedDelete(t *testing.T) {
	ctx := context.Background()
	cl := clusterClient(t)

	// The dev server's state is persisted across runs (.wrangler/state), and
	// "default" is by construction undeletable, so a re-run finds it already
	// there. Both outcomes are fine.
	if _, err := cl.Create(ctx, newCluster("default", "management"), metav1.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("create default cluster: %v", err)
	}

	err := cl.Delete(ctx, "default", metav1.DeleteOptions{})
	if err == nil {
		t.Fatal("deleting the default cluster succeeded, want 403 Forbidden")
	}
	if !errors.IsForbidden(err) {
		t.Fatalf("delete default cluster: err = %v, want Forbidden", err)
	}
	if _, err := cl.Get(ctx, "default", metav1.GetOptions{}); err != nil {
		t.Errorf("default cluster gone after rejected delete: %v", err)
	}

	other := fmt.Sprintf("deletable-%d", time.Now().UnixNano())
	if _, err := cl.Create(ctx, newCluster(other, "throwaway"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create %s: %v", other, err)
	}
	if err := cl.Delete(ctx, other, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete %s: %v -- only \"default\" is protected", other, err)
	}
}

// TestClusterDeepCopyIndependence exercises the hand-written DeepCopy
// functions in pkg/apis/k8flare/v1alpha1 (this repo has no deepcopy-gen, so
// nothing else checks them): a copy must share no mutable state with its
// source. Pure Go -- no server involved.
func TestClusterDeepCopyIndependence(t *testing.T) {
	orig := &k8flarev1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "team-a",
			Labels:     map[string]string{"tier": "gold"},
			Finalizers: []string{"k8flare.com/cluster-teardown"},
		},
		Spec: k8flarev1alpha1.ClusterSpec{DisplayName: "Team A"},
		Status: k8flarev1alpha1.ClusterStatus{
			Phase:          "Ready",
			TokenSecretRef: &k8flarev1alpha1.SecretReference{Namespace: "k8flare-system", Name: "cluster-team-a"},
			Conditions: []metav1.Condition{{
				Type: "Ready", Status: metav1.ConditionTrue, Reason: "Provisioned",
			}},
		},
	}

	copied, ok := orig.DeepCopyObject().(*k8flarev1alpha1.Cluster)
	if !ok {
		t.Fatal("DeepCopyObject did not return a *Cluster")
	}

	copied.ObjectMeta.Labels["tier"] = "bronze"
	copied.ObjectMeta.Finalizers[0] = "other"
	copied.Status.TokenSecretRef.Name = "hijacked"
	copied.Status.Conditions[0].Reason = "Changed"
	copied.Spec.DisplayName = "Team B"

	if orig.ObjectMeta.Labels["tier"] != "gold" {
		t.Error("mutating the copy's labels changed the original")
	}
	if orig.ObjectMeta.Finalizers[0] != "k8flare.com/cluster-teardown" {
		t.Error("mutating the copy's finalizers changed the original")
	}
	if orig.Status.TokenSecretRef.Name != "cluster-team-a" {
		t.Error("mutating the copy's tokenSecretRef changed the original")
	}
	if orig.Status.Conditions[0].Reason != "Provisioned" {
		t.Error("mutating the copy's conditions changed the original")
	}
	if orig.Spec.DisplayName != "Team A" {
		t.Error("mutating the copy's spec changed the original")
	}

	list := &k8flarev1alpha1.ClusterList{Items: []k8flarev1alpha1.Cluster{*orig}}
	copiedList, ok := list.DeepCopyObject().(*k8flarev1alpha1.ClusterList)
	if !ok {
		t.Fatal("DeepCopyObject did not return a *ClusterList")
	}
	copiedList.Items[0].Spec.DisplayName = "Team C"
	if list.Items[0].Spec.DisplayName != "Team A" {
		t.Error("mutating the copied list's item changed the original list")
	}
}
