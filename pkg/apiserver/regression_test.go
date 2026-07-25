package apiserver_test

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSubresourceBindingRejectsNonPodResource guards against a regression
// introduced (and then fixed) while merging pkg/apiserver/subresource.go's 7
// hand-copied /status blocks into one generic handler: dispatch became "by
// subresource name alone," which incidentally also let a Node reach
// handleBindingSubresource (Pod-only -- it type-asserts the fetched object
// straight to *corev1.Pod). Before the fix, POST .../nodes/{name}/binding
// panicked (pkg/apiserver has no recover() anywhere) instead of cleanly
// 404ing the way the pre-refactor resource+subresource-keyed switch did.
// Verified against a real wrangler dev stack: a panicking request would
// either hang this test past its client-go timeout or surface as a
// connection-reset/500, not a clean 404.
func TestSubresourceBindingRejectsNonPodResource(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	nodeName := "test-binding-wrong-resource-node"

	_ = client.CoreV1().Nodes().Delete(ctx, nodeName, metav1.DeleteOptions{})
	_, err := client.CoreV1().Nodes().Create(ctx, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create node: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().Nodes().Delete(context.Background(), nodeName, metav1.DeleteOptions{})
	})

	binding := &corev1.Binding{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Target:     corev1.ObjectReference{Kind: "Node", Name: "somewhere"},
	}
	err = client.CoreV1().RESTClient().Post().
		Resource("nodes").
		Name(nodeName).
		SubResource("binding").
		Body(binding).
		Do(ctx).
		Error()

	if err == nil {
		t.Fatal("expected an error POSTing to nodes/.../binding, got nil (should not be a supported subresource for Node)")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected a NotFound-shaped error, got: %v", err)
	}
}

// TestNamespaceCollectionDeleteSweepsDependents guards against a bug found
// during review (present since before this refactor, not introduced by
// it): HandleResource's DELETE case checked "is this a namespaces cascade
// delete" only in the named-delete branch, never in the
// name=="" (collection-delete, e.g. DELETE /api/v1/namespaces) branch --
// so a collection-delete of Namespaces skipped DeleteNamespaceDependents
// entirely and would silently orphan every namespaced resource in every
// Namespace it removed. Scoped to a uniquely labeled namespace via
// labelSelector, since every test in this package shares one wrangler dev
// instance/DO and a real "delete every Namespace" call would take out
// "default"/"kube-system"/etc. that other tests depend on.
func TestNamespaceCollectionDeleteSweepsDependents(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "test-collection-delete-cascade-ns"
	const labelSelector = "k8flare-regression-test=collection-delete-cascade"

	_ = client.CoreV1().ConfigMaps(ns).Delete(ctx, "dependent-cm", metav1.DeleteOptions{})
	_ = client.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})

	_, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ns,
			Labels: map[string]string{"k8flare-regression-test": "collection-delete-cascade"},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create namespace: %v", err)
	}

	_, err = client.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "dependent-cm", Namespace: ns},
		Data:       map[string]string{"k": "v"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create dependent ConfigMap: %v", err)
	}

	// NamespaceInterface has no generated DeleteCollection (cluster-scoped
	// resources don't get one from client-go's generator) -- go through the
	// raw REST client to reach the server's generic name=="" collection-
	// delete path directly, the same request shape `kubectl delete
	// namespaces -l ...` would send.
	err = client.CoreV1().RESTClient().Delete().
		Resource("namespaces").
		Param("labelSelector", labelSelector).
		Do(ctx).
		Error()
	if err != nil {
		t.Fatalf("DeleteCollection namespaces: %v", err)
	}

	if _, err := client.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("expected namespace %q to be gone after DeleteCollection, got: %v", ns, err)
	}

	_, err = client.CoreV1().ConfigMaps(ns).Get(ctx, "dependent-cm", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected dependent ConfigMap to have been swept by the namespaces DeleteCollection cascade, got: %v", err)
	}
}

// TestStatusSubresourceRejectsUndeclaredResource guards against a
// regression found by review: HandleSubresource used to dispatch "status"
// requests by subresource name alone, for whatever resource happened to be
// in the URL -- so a Service (which has a real Go .Status field, just no
// "status" entry in its apidef.Table ResourceDef) could still be PUT/PATCHed
// through services/{name}/status despite discovery.go (which is built from
// the same table) never advertising it. Now gated on apidef.HasSubresource.
func TestStatusSubresourceRejectsUndeclaredResource(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"
	name := "test-status-undeclared-svc"

	client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	_ = client.CoreV1().Services(ns).Delete(ctx, name, metav1.DeleteOptions{})

	svc, err := client.CoreV1().Services(ns).Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create service: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().Services(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	err = client.CoreV1().RESTClient().Put().
		Namespace(ns).
		Resource("services").
		Name(name).
		SubResource("status").
		Body(svc).
		Do(ctx).
		Error()

	if err == nil {
		t.Fatal("expected an error PUTting services/.../status, got nil (Service has no declared \"status\" subresource in apidef.Table)")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected a NotFound-shaped error, got: %v", err)
	}
}

// TestMalformedFieldSelectorIsBadRequestNotInternalError guards against a
// regression found by review: a syntactically invalid fieldSelector (as
// opposed to one naming an unsupported-but-well-formed field, which
// correctly 400s already) used to come back from fields.ParseSelector as a
// plain wrapped error, which writeResourceError's errors.As can't unwrap to
// anything but a generic 500 -- misreporting a client typo in
// --field-selector as a server crash.
func TestMalformedFieldSelectorIsBadRequestNotInternalError(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()

	var result corev1.PodList
	err := client.CoreV1().RESTClient().Get().
		Resource("pods").
		Namespace("default").
		Param("fieldSelector", `metadata.name=foo\q`). // invalid escape sequence -- a genuine parse error, not just an unknown field
		Do(ctx).
		Into(&result)

	if err == nil {
		t.Fatal("expected an error for a malformed field selector, got nil")
	}
	if apierrors.IsInternalError(err) {
		t.Errorf("malformed field selector reported as an internal server error (500), want BadRequest (400): %v", err)
	}
	if !apierrors.IsBadRequest(err) {
		t.Errorf("expected a BadRequest-shaped (400) error, got: %v", err)
	}
}

// TestTableRowObjectsCarryTypeMeta guards the 2026-07-25 fix in
// scheme.go's Encode: objects stored WITHOUT TypeMeta (anything created
// server-side -- the bootstrap namespaces here are guaranteed examples)
// used to be embedded Kind-less in Table rows, which makes kubectl's
// Table printer abort the whole listing and render blank NAME columns.
// Every row's embedded object must decode with a non-empty kind.
func TestTableRowObjectsCarryTypeMeta(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()

	raw, err := client.CoreV1().RESTClient().Get().
		AbsPath("/api/v1/namespaces").
		SetHeader("Accept", "application/json;as=Table;v=v1;g=meta.k8s.io,application/json").
		Do(ctx).
		Raw()
	if err != nil {
		t.Fatalf("GET namespaces as Table: %v", err)
	}

	var table metav1.Table
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatalf("decode Table: %v", err)
	}
	if table.Kind != "Table" {
		t.Fatalf("expected kind Table, got %q", table.Kind)
	}
	if len(table.Rows) == 0 {
		t.Fatal("expected at least the bootstrap namespaces in the table")
	}
	for i, row := range table.Rows {
		var obj struct {
			Kind       string `json:"kind"`
			APIVersion string `json:"apiVersion"`
		}
		if err := json.Unmarshal(row.Object.Raw, &obj); err != nil {
			t.Fatalf("row %d: decode embedded object: %v", i, err)
		}
		if obj.Kind == "" || obj.APIVersion == "" {
			t.Errorf("row %d: embedded object missing TypeMeta (kind=%q apiVersion=%q); kubectl blanks the whole table on this", i, obj.Kind, obj.APIVersion)
		}
	}
}
