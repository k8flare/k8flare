package helm

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

var (
	configMapsGVR  = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	servicesGVR    = schema.GroupVersionResource{Version: "v1", Resource: "services"}
	deploymentsGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	crdsGVR        = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
)

func testMapper() meta.RESTMapper {
	m := meta.NewDefaultRESTMapper(nil)
	m.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Version: "v1", Kind: "Service"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Version: "v1", Kind: "Secret"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	m.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}, meta.RESTScopeRoot)
	return m
}

func newFakeClient(objs ...runtime.Object) *fake.FakeDynamicClient {
	c := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		configMapsGVR:  "ConfigMapList",
		servicesGVR:    "ServiceList",
		secretsGVR:     "SecretList",
		deploymentsGVR: "DeploymentList",
		crdsGVR:        "CustomResourceDefinitionList",
		namespacesGVR:  "NamespaceList",
		helmChartsGVR:  "HelmChartList",
		helmConfigsGVR: "HelmChartConfigList",
	}, objs...)
	c.PrependReactor("patch", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		patch := action.(clienttesting.PatchAction)
		if patch.GetPatchType() != types.ApplyPatchType {
			return false, nil, nil
		}
		obj := &unstructured.Unstructured{}
		if err := obj.UnmarshalJSON(patch.GetPatch()); err != nil {
			return true, nil, err
		}
		if _, err := c.Tracker().Get(patch.GetResource(), patch.GetNamespace(), obj.GetName()); apierrors.IsNotFound(err) {
			return true, obj, c.Tracker().Create(patch.GetResource(), obj, patch.GetNamespace())
		}
		return true, obj, c.Tracker().Update(patch.GetResource(), obj, patch.GetNamespace())
	})
	return c
}

func chartRepository(t *testing.T) *httptest.Server {
	t.Helper()
	index := `apiVersion: v1
entries:
  demo:
    - version: 0.2.0-rc.1
      urls: [demo-0.2.0-rc.1.tgz]
    - version: 0.1.0
      urls: [demo-0.1.0.tgz]
`
	archive := readTestdata(t, "demo-0.1.0.tgz")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.yaml":
			w.Write([]byte(index))
		case "/demo-0.1.0.tgz":
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func newController(client *fake.FakeDynamicClient) *Controller {
	return &Controller{
		Client:       client,
		Mapper:       func() (meta.RESTMapper, error) { return testMapper(), nil },
		HTTP:         http.DefaultClient,
		Capabilities: func() (*chartutil.Capabilities, error) { return pinnedCapabilities(), nil },
		Now:          func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
}

func helmChartObject(repo string, spec map[string]any) *unstructured.Unstructured {
	base := map[string]any{"chart": "demo", "repo": repo, "targetNamespace": "apps", "createNamespace": true}
	for k, v := range spec {
		base[k] = v
	}
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.cattle.io/v1",
		"kind":       "HelmChart",
		"spec":       base,
	}}
	u.SetName("demo")
	u.SetNamespace("kube-system")
	return u
}

func getObject(t *testing.T, c *fake.FakeDynamicClient, gvr schema.GroupVersionResource, namespace, name string) *unstructured.Unstructured {
	t.Helper()
	obj, err := c.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get %s %s/%s: %v", gvr.Resource, namespace, name, err)
	}
	return obj
}

func exists(c *fake.FakeDynamicClient, gvr schema.GroupVersionResource, namespace, name string) bool {
	_, err := c.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	return err == nil
}

func writeCount(c *fake.FakeDynamicClient) int {
	n := 0
	for _, a := range c.Actions() {
		switch a.GetVerb() {
		case "create", "update", "patch", "delete":
			n++
		}
	}
	return n
}

func condition(t *testing.T, u *unstructured.Unstructured) map[string]any {
	t.Helper()
	conditions, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	if len(conditions) != 1 {
		t.Fatalf("conditions = %v", conditions)
	}
	return conditions[0].(map[string]any)
}

func releaseHistory(t *testing.T, c *Controller, namespace string) []*record {
	t.Helper()
	history, err := c.store().history(context.Background(), namespace, "demo")
	if err != nil {
		t.Fatal(err)
	}
	return history
}

func replicas(t *testing.T, c *fake.FakeDynamicClient) int64 {
	t.Helper()
	n, _, _ := unstructured.NestedInt64(getObject(t, c, deploymentsGVR, "apps", "demo-demo").Object, "spec", "replicas")
	return n
}

func TestInstallFromRepositoryAppliesAndRecordsTheRelease(t *testing.T) {
	server := chartRepository(t)
	chart := helmChartObject(server.URL, map[string]any{
		"valuesContent": "cache:\n  size: 5Gi\n",
		"set":           map[string]any{"replicaCount": int64(3)},
	})
	client := newFakeClient(chart)
	c := newController(client)
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if replicas(t, client) != 3 {
		t.Fatalf("replicas = %d, want the --set value", replicas(t, client))
	}
	if got, _, _ := unstructured.NestedString(getObject(t, client, configMapsGVR, "apps", "demo-cache").Object, "data", "size"); got != "5Gi" {
		t.Fatalf("subchart size = %q, want the parent's override", got)
	}
	if !exists(client, servicesGVR, "apps", "demo-demo") || !exists(client, namespacesGVR, "", "apps") {
		t.Fatal("service or target namespace missing")
	}
	if !exists(client, crdsGVR, "", "widgets.demo.example.com") {
		t.Fatal("crds/ object was not installed")
	}
	owned := getObject(t, client, deploymentsGVR, "apps", "demo-demo")
	if owned.GetLabels()[labelManagedBy] != "Helm" || owned.GetAnnotations()[annotationReleaseKey] != "demo" || owned.GetAnnotations()[annotationReleaseNS] != "apps" {
		t.Fatalf("missing Helm ownership metadata: %v %v", owned.GetLabels(), owned.GetAnnotations())
	}
	stored := getObject(t, client, helmChartsGVR, "kube-system", "demo")
	if !containsString(stored.GetFinalizers(), Finalizer) || stored.GetAnnotations()[AnnotationManagedBy] != ManagedBy {
		t.Fatalf("chart not claimed: %v %v", stored.GetFinalizers(), stored.GetAnnotations())
	}
	if condition(t, stored)["status"] != "False" {
		t.Fatalf("status = %v", condition(t, stored))
	}

	secret := getObject(t, client, secretsGVR, "apps", "sh.helm.release.v1.demo.v1")
	if typ, _, _ := unstructured.NestedString(secret.Object, "type"); typ != "helm.sh/release.v1" {
		t.Fatalf("secret type = %q", typ)
	}
	labels := secret.GetLabels()
	if labels["owner"] != "helm" || labels["name"] != "demo" || labels["status"] != "deployed" || labels["version"] != "1" || labels[KeyConfigHash] == "" {
		t.Fatalf("release labels = %v", labels)
	}
	history := releaseHistory(t, c, "apps")
	rel := history[0].release
	if len(history) != 1 || rel.Chart.Metadata.Name != "demo" || rel.Chart.Metadata.Version != "0.1.0" || rel.Info.Status != release.StatusDeployed {
		t.Fatalf("release = %+v", rel)
	}
	if !strings.Contains(rel.Manifest, "# Source: demo/templates/deployment.yaml") || rel.Config["replicaCount"] != float64(3) {
		t.Fatalf("release manifest or config wrong: %v", rel.Config)
	}
}

func TestReconcileIsIdempotent(t *testing.T) {
	server := chartRepository(t)
	client := newFakeClient(helmChartObject(server.URL, nil))
	c := newController(client)
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := writeCount(client)
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if after := writeCount(client); after != before {
		t.Fatalf("second reconcile wrote %d times: %v", after-before, client.Actions()[len(client.Actions())-(after-before):])
	}
}

func TestUpgradePrunesRemovedObjectsAndSupersedesTheOldRelease(t *testing.T) {
	server := chartRepository(t)
	client := newFakeClient(helmChartObject(server.URL, nil))
	c := newController(client)
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !exists(client, configMapsGVR, "apps", "demo-cache") {
		t.Fatal("subchart configmap missing after install")
	}
	stored := getObject(t, client, helmChartsGVR, "kube-system", "demo")
	unstructured.SetNestedField(stored.Object, map[string]any{"cache.enabled": "false", "replicaCount": int64(5)}, "spec", "set")
	if _, err := client.Resource(helmChartsGVR).Namespace("kube-system").Update(context.Background(), stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if replicas(t, client) != 5 {
		t.Fatalf("replicas = %d after upgrade", replicas(t, client))
	}
	if exists(client, configMapsGVR, "apps", "demo-cache") {
		t.Fatal("object dropped from the chart was not pruned")
	}
	history := releaseHistory(t, c, "apps")
	if len(history) != 2 || history[0].release.Info.Status != release.StatusSuperseded || history[1].release.Info.Status != release.StatusDeployed {
		t.Fatalf("history = %v %v", history[0].release.Info.Status, history[1].release.Info.Status)
	}
	if history[1].release.Info.Description != "Upgrade complete" || !strings.Contains(history[1].release.Manifest, "replicas: 5") {
		t.Fatalf("upgrade release = %+v", history[1].release.Info)
	}
	if getObject(t, client, configMapsGVR, "apps", "demo-demo").Object["data"].(map[string]any)["install"] != false {
		t.Fatal("Release.IsInstall stayed true on upgrade")
	}
}

func TestHelmChartConfigValuesOverrideTheChart(t *testing.T) {
	server := chartRepository(t)
	config := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.cattle.io/v1",
		"kind":       "HelmChartConfig",
		"spec":       map[string]any{"valuesContent": "replicaCount: 7\n"},
	}}
	config.SetName("demo")
	config.SetNamespace("kube-system")
	client := newFakeClient(helmChartObject(server.URL, map[string]any{"valuesContent": "replicaCount: 2\n"}), config)
	if err := newController(client).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if replicas(t, client) != 7 {
		t.Fatalf("replicas = %d, want the HelmChartConfig value", replicas(t, client))
	}
}

func TestUninstallOnDeletionRemovesObjectsAndReleaseButKeepsCRDs(t *testing.T) {
	server := chartRepository(t)
	client := newFakeClient(helmChartObject(server.URL, nil))
	c := newController(client)
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored := getObject(t, client, helmChartsGVR, "kube-system", "demo")
	now := metav1.NewTime(time.Now())
	stored.SetDeletionTimestamp(&now)
	if _, err := client.Resource(helmChartsGVR).Namespace("kube-system").Update(context.Background(), stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if exists(client, configMapsGVR, "apps", "demo-demo") || exists(client, configMapsGVR, "apps", "demo-cache") {
		t.Fatal("configmaps survived the uninstall")
	}
	if exists(client, deploymentsGVR, "apps", "demo-demo") || exists(client, servicesGVR, "apps", "demo-demo") {
		t.Fatal("workload objects survived the uninstall")
	}
	if len(releaseHistory(t, c, "apps")) != 0 {
		t.Fatal("release secrets survived the uninstall")
	}
	if !exists(client, crdsGVR, "", "widgets.demo.example.com") {
		t.Fatal("helm never removes crds/ objects")
	}
	after := getObject(t, client, helmChartsGVR, "kube-system", "demo")
	if containsString(after.GetFinalizers(), Finalizer) {
		t.Fatal("finalizer still set")
	}
}

func TestInstallRefusesToAdoptForeignObjects(t *testing.T) {
	server := chartRepository(t)
	foreign := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "data": map[string]any{"keep": "me"}}}
	foreign.SetName("demo-demo")
	foreign.SetNamespace("apps")
	client := newFakeClient(helmChartObject(server.URL, nil), foreign)
	c := newController(client)
	err := c.Reconcile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid ownership metadata") {
		t.Fatalf("err = %v", err)
	}
	history := releaseHistory(t, c, "apps")
	if len(history) != 1 || history[0].release.Info.Status != release.StatusFailed {
		t.Fatalf("history = %+v", history)
	}
	failed := getObject(t, client, helmChartsGVR, "kube-system", "demo")
	if cond := condition(t, failed); cond["status"] != "True" || !strings.Contains(cond["message"].(string), "ownership") {
		t.Fatalf("condition = %v", cond)
	}
	if _, found, _ := unstructured.NestedString(getObject(t, client, configMapsGVR, "apps", "demo-demo").Object, "data", "keep"); !found {
		t.Fatal("foreign object was overwritten")
	}

	stored := getObject(t, client, helmChartsGVR, "kube-system", "demo")
	unstructured.SetNestedField(stored.Object, true, "spec", "takeOwnership")
	if _, err := client.Resource(helmChartsGVR).Namespace("kube-system").Update(context.Background(), stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	history = releaseHistory(t, c, "apps")
	if last := history[len(history)-1].release; last.Info.Status != release.StatusDeployed {
		t.Fatalf("status = %v after takeOwnership", last.Info.Status)
	}
}

func TestFailedInstallRetriesInPlaceInsteadOfPilingUpRevisions(t *testing.T) {
	server := chartRepository(t)
	foreign := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap"}}
	foreign.SetName("demo-demo")
	foreign.SetNamespace("apps")
	client := newFakeClient(helmChartObject(server.URL, nil), foreign)
	c := newController(client)
	for i := 0; i < 3; i++ {
		if err := c.Reconcile(context.Background()); err == nil {
			t.Fatal("expected the ownership error")
		}
	}
	if n := len(releaseHistory(t, c, "apps")); n != 1 {
		t.Fatalf("%d release revisions, want 1", n)
	}
}

func TestUnmanagedAndForeignManagedChartsAreLeftAlone(t *testing.T) {
	server := chartRepository(t)
	unmanaged := helmChartObject(server.URL, nil)
	unmanaged.SetName("skipped")
	unmanaged.SetAnnotations(map[string]string{AnnotationUnmanaged: "true"})
	foreign := helmChartObject(server.URL, nil)
	foreign.SetName("other")
	foreign.SetAnnotations(map[string]string{AnnotationManagedBy: "k3s-server-1"})
	client := newFakeClient(unmanaged, foreign)
	if err := newController(client).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := writeCount(client); n != 0 {
		t.Fatalf("%d writes for charts this controller does not manage", n)
	}
}

func TestInstallFromInlineChartContent(t *testing.T) {
	inline := base64.StdEncoding.EncodeToString(readTestdata(t, "demo-0.1.0.tgz"))
	chart := helmChartObject("", map[string]any{"chart": "", "chartContent": inline})
	client := newFakeClient(chart)
	if err := newController(client).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if replicas(t, client) != 1 {
		t.Fatalf("replicas = %d", replicas(t, client))
	}
}

func TestUnsupportedHelmVersionIsReportedNotInstalled(t *testing.T) {
	server := chartRepository(t)
	client := newFakeClient(helmChartObject(server.URL, map[string]any{"helmVersion": "v2"}))
	if err := newController(client).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored := getObject(t, client, helmChartsGVR, "kube-system", "demo")
	if cond := condition(t, stored); cond["status"] != "True" || cond["reason"] != "Unsupported version" {
		t.Fatalf("condition = %v", cond)
	}
	if exists(client, deploymentsGVR, "apps", "demo-demo") {
		t.Fatal("installed a v2 chart")
	}
}

func TestReconcileWithoutTheCRDsIsANoOp(t *testing.T) {
	client := newFakeClient()
	client.PrependReactor("list", "helmcharts", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(helmChartsGVR.GroupResource(), "")
	})
	if err := newController(client).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
