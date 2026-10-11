package customresources

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const clusterWidgetCRD = `{
  "apiVersion": "apiextensions.k8s.io/v1",
  "kind": "CustomResourceDefinition",
  "metadata": {"name": "clusterwidgets.example.com"},
  "spec": {
    "group": "example.com",
    "names": {"kind": "ClusterWidget", "plural": "clusterwidgets", "singular": "clusterwidget"},
    "scope": "Cluster",
    "versions": [{"name": "v1", "served": true, "storage": true,
      "schema": {"openAPIV3Schema": {"type": "object", "x-kubernetes-preserve-unknown-fields": true}}}]
  }
}`

func storedNamespace(name, phase string) memoryEntry {
	ns := `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"` + name + `"},"status":{"phase":"` + phase + `"}}`
	return memoryEntry{value: base64.StdEncoding.EncodeToString([]byte(ns)), rev: 1}
}

func postAsAdmin(h http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Remote-User", "alice")
	req.Header.Set("X-Remote-Group", "system:masters")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func statusError(t *testing.T, rec *httptest.ResponseRecorder) error {
	t.Helper()
	status := metav1.Status{}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("status=%d body=%s: %v", rec.Code, rec.Body, err)
	}
	return &apierrors.StatusError{ErrStatus: status}
}

func TestCustomResourceCreateFollowsNamespaceLifecycle(t *testing.T) {
	store := &memoryKine{rev: 1, data: map[string]memoryEntry{
		"/registry/namespaces/active": storedNamespace("active", "Active"),
		"/registry/namespaces/gone":   storedNamespace("gone", "Terminating"),
	}}
	srv := httptest.NewServer(store)
	defer srv.Close()
	h, err := NewHandler(Config{Kine: &http.Client{Transport: rewriteTo{srv.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, crd := range []string{helmChartCRD, clusterWidgetCRD} {
		if rec := postAsAdmin(h, "/apis/apiextensions.k8s.io/v1/customresourcedefinitions", crd); rec.Code != http.StatusCreated {
			t.Fatalf("create CRD: status=%d body=%s", rec.Code, rec.Body)
		}
	}
	chart := func(ns string) string {
		return `{"apiVersion":"helm.cattle.io/v1","kind":"HelmChart","metadata":{"name":"chart","namespace":"` + ns + `"}}`
	}

	active := postAsAdmin(h, "/apis/helm.cattle.io/v1/namespaces/active/helmcharts", chart("active"))
	if active.Code != http.StatusCreated {
		t.Fatalf("active namespace: status=%d body=%s", active.Code, active.Body)
	}
	terminating := postAsAdmin(h, "/apis/helm.cattle.io/v1/namespaces/gone/helmcharts", chart("gone"))
	if terminating.Code != http.StatusForbidden || !apierrors.HasStatusCause(statusError(t, terminating), corev1.NamespaceTerminatingCause) {
		t.Fatalf("terminating namespace: status=%d body=%s", terminating.Code, terminating.Body)
	}
	list := httptest.NewRequest(http.MethodGet, "/apis/helm.cattle.io/v1/namespaces/gone/helmcharts", nil)
	list.Header.Set("Accept", "application/json")
	list.Header.Set("X-Remote-User", "alice")
	list.Header.Set("X-Remote-Group", "system:masters")
	listed := httptest.NewRecorder()
	h.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK {
		t.Fatalf("list in terminating namespace: status=%d body=%s", listed.Code, listed.Body)
	}
	missing := postAsAdmin(h, "/apis/helm.cattle.io/v1/namespaces/missing/helmcharts", chart("missing"))
	if err := statusError(t, missing); !apierrors.IsNotFound(err) || err.(*apierrors.StatusError).ErrStatus.Details.Kind != "namespaces" {
		t.Fatalf("missing namespace: status=%d body=%s", missing.Code, missing.Body)
	}
	widget := postAsAdmin(h, "/apis/example.com/v1/clusterwidgets", `{"apiVersion":"example.com/v1","kind":"ClusterWidget","metadata":{"name":"widget"}}`)
	if widget.Code != http.StatusCreated {
		t.Fatalf("cluster-scoped: status=%d body=%s", widget.Code, widget.Body)
	}
}
