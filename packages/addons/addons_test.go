package addons

import (
	"context"
	"regexp"
	"strings"
	"testing"

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
	configMaps = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	secrets    = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	classes    = schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}
)

func testMapper() meta.RESTMapper {
	m := meta.NewDefaultRESTMapper(nil)
	m.Add(schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}, meta.RESTScopeRoot)
	m.Add(schema.GroupVersionKind{Group: "k3s.cattle.io", Version: "v1", Kind: "Addon"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Version: "v1", Kind: "Secret"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "StorageClass"}, meta.RESTScopeRoot)
	return m
}

func newClient(objs ...runtime.Object) *fake.FakeDynamicClient {
	c := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		configMaps: "ConfigMapList",
		secrets:    "SecretList",
		classes:    "StorageClassList",
		addonsGVR:  "AddonList",
		crdsGVR:    "CustomResourceDefinitionList",
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

func verbs(c *fake.FakeDynamicClient, want ...string) []string {
	var got []string
	for _, a := range c.Actions() {
		for _, v := range want {
			if a.GetVerb() == v {
				got = append(got, v+" "+a.GetResource().Resource)
				break
			}
		}
	}
	return got
}

func count(got []string, entry string) int {
	n := 0
	for _, g := range got {
		if g == entry {
			n++
		}
	}
	return n
}

func writes(c *fake.FakeDynamicClient) []string {
	return verbs(c, "create", "update", "patch", "delete")
}

func cmManifest(data string) File {
	return File{Name: "demo.yaml", Content: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: demo
  namespace: kube-system
data:
  key: ` + data + `
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: demo
provisioner: example.com/demo
`)}
}

func newDeployer(c *fake.FakeDynamicClient) *Deployer {
	return &Deployer{Client: c, Mapper: testMapper()}
}

func mustGetAddon(t *testing.T, c *fake.FakeDynamicClient, name string) *unstructured.Unstructured {
	t.Helper()
	got, err := c.Resource(addonsGVR).Namespace("kube-system").Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get addon %s: %v", name, err)
	}
	return got
}

func TestDeployAppliesTemplatedManifestAndTracksAddon(t *testing.T) {
	c := newClient()
	file := cmManifest("%{CLUSTER_DNS}%")
	vars := map[string]string{"%{CLUSTER_DNS}%": "10.43.0.10"}
	if err := newDeployer(c).Deploy(context.Background(), Render([]File{file}, vars), nil); err != nil {
		t.Fatal(err)
	}
	cm, err := c.Resource(configMaps).Namespace("kube-system").Get(context.Background(), "demo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, _, _ := unstructured.NestedString(cm.Object, "data", "key"); got != "10.43.0.10" {
		t.Fatalf("configmap key = %q, want templated value", got)
	}
	if _, err := c.Resource(classes).Get(context.Background(), "demo", metav1.GetOptions{}); err != nil {
		t.Fatalf("cluster-scoped object: %v", err)
	}
	addon := mustGetAddon(t, c, "demo")
	checksum, _, _ := unstructured.NestedString(addon.Object, "spec", "checksum")
	if len(checksum) != 64 {
		t.Fatalf("addon checksum = %q, want sha256 hex", checksum)
	}
	if !strings.Contains(addon.GetAnnotations()[GVKAnnotation], "Kind=ConfigMap") {
		t.Fatalf("gvks annotation = %q", addon.GetAnnotations()[GVKAnnotation])
	}
	if cm.GetLabels()[labelHash] == "" || cm.GetAnnotations()[labelOwnerName] != "demo" {
		t.Fatalf("owner labels missing: %v %v", cm.GetLabels(), cm.GetAnnotations())
	}
}

func TestDeployInstallsAddonCRDOnce(t *testing.T) {
	c := newClient()
	d := newDeployer(c)
	for range 2 {
		if err := d.Deploy(context.Background(), []File{cmManifest("a")}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(verbs(c, "create"), "create customresourcedefinitions"); n != 1 {
		t.Fatalf("crd created %d times, want 1", n)
	}
}

func TestDeployReportsPendingWhileAddonCRDIsNotServed(t *testing.T) {
	c := newClient()
	m := meta.NewDefaultRESTMapper(nil)
	m.Add(schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}, meta.RESTScopeRoot)
	err := (&Deployer{Client: c, Mapper: m}).Deploy(context.Background(), []File{cmManifest("a")}, nil)
	if err != ErrAddonCRDPending {
		t.Fatalf("err = %v, want ErrAddonCRDPending", err)
	}
	if got := writes(c); count(got, "patch configmaps") != 0 {
		t.Fatalf("applied objects before the Addon CRD was served: %v", got)
	}
}

func TestDeployIsIdempotent(t *testing.T) {
	c := newClient()
	d := newDeployer(c)
	files := []File{cmManifest("a")}
	if err := d.Deploy(context.Background(), files, nil); err != nil {
		t.Fatal(err)
	}
	c.ClearActions()
	if err := d.Deploy(context.Background(), files, nil); err != nil {
		t.Fatal(err)
	}
	if got := writes(c); len(got) != 0 {
		t.Fatalf("second run wrote: %v", got)
	}
}

func TestDeployReappliesWhenChecksumChanges(t *testing.T) {
	c := newClient()
	d := newDeployer(c)
	if err := d.Deploy(context.Background(), []File{cmManifest("a")}, nil); err != nil {
		t.Fatal(err)
	}
	before := mustGetAddon(t, c, "demo")
	c.ClearActions()
	if err := d.Deploy(context.Background(), []File{cmManifest("b")}, nil); err != nil {
		t.Fatal(err)
	}
	if n := count(writes(c), "patch configmaps"); n != 1 {
		t.Fatalf("configmap applied %d times, want 1: %v", n, writes(c))
	}
	after := mustGetAddon(t, c, "demo")
	b, _, _ := unstructured.NestedString(before.Object, "spec", "checksum")
	a, _, _ := unstructured.NestedString(after.Object, "spec", "checksum")
	if a == b {
		t.Fatal("addon checksum did not change")
	}
}

func TestDeployPrunesObjectsDroppedFromManifest(t *testing.T) {
	c := newClient()
	d := newDeployer(c)
	if err := d.Deploy(context.Background(), []File{cmManifest("a")}, nil); err != nil {
		t.Fatal(err)
	}
	only := File{Name: "demo.yaml", Content: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: demo
  namespace: kube-system
data:
  key: a
`)}
	if err := d.Deploy(context.Background(), []File{only}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Resource(classes).Get(context.Background(), "demo", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("storage class still present: %v", err)
	}
	if _, err := c.Resource(configMaps).Namespace("kube-system").Get(context.Background(), "demo", metav1.GetOptions{}); err != nil {
		t.Fatalf("kept object was pruned: %v", err)
	}
}

func TestDeployLeavesForeignObjectsAlone(t *testing.T) {
	foreign := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"name": "other", "namespace": "kube-system"},
	}}
	c := newClient(foreign)
	d := newDeployer(c)
	if err := d.Deploy(context.Background(), []File{cmManifest("a")}, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Deploy(context.Background(), []File{cmManifest("b")}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Resource(configMaps).Namespace("kube-system").Get(context.Background(), "other", metav1.GetOptions{}); err != nil {
		t.Fatalf("foreign object deleted: %v", err)
	}
}

func TestDeployHonoursDisable(t *testing.T) {
	c := newClient()
	d := newDeployer(c)
	if err := d.Deploy(context.Background(), []File{cmManifest("a")}, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Deploy(context.Background(), []File{cmManifest("a")}, map[string]bool{"demo": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Resource(configMaps).Namespace("kube-system").Get(context.Background(), "demo", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("configmap survived disable: %v", err)
	}
	if _, err := c.Resource(classes).Get(context.Background(), "demo", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("storage class survived disable: %v", err)
	}
	if _, err := c.Resource(addonsGVR).Namespace("kube-system").Get(context.Background(), "demo", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("addon survived disable: %v", err)
	}
	c.ClearActions()
	if err := d.Deploy(context.Background(), []File{cmManifest("a")}, map[string]bool{"demo": true}); err != nil {
		t.Fatal(err)
	}
	if got := writes(c); len(got) != 0 {
		t.Fatalf("disabled addon written on every pass: %v", got)
	}
}

func TestDeployDisabledFromTheStartAppliesNothing(t *testing.T) {
	c := newClient()
	if err := newDeployer(c).Deploy(context.Background(), []File{cmManifest("a")}, map[string]bool{"demo": true}); err != nil {
		t.Fatal(err)
	}
	if got := writes(c); count(got, "patch configmaps") != 0 || count(got, "create addons") != 0 {
		t.Fatalf("disabled addon deployed: %v", got)
	}
}

func TestDeploySkipsFilesWithSkipMarker(t *testing.T) {
	c := newClient()
	files := []File{cmManifest("a"), {Name: "demo.yaml.skip"}}
	if err := newDeployer(c).Deploy(context.Background(), files, nil); err != nil {
		t.Fatal(err)
	}
	if got := writes(c); count(got, "patch configmaps") != 0 {
		t.Fatalf("skipped file applied: %v", got)
	}
}

func TestDeployIgnoresDotfilesAndOtherExtensions(t *testing.T) {
	c := newClient()
	files := []File{{Name: ".hidden.yaml", Content: cmManifest("a").Content}, {Name: "notes.txt", Content: []byte("x")}}
	if err := newDeployer(c).Deploy(context.Background(), files, nil); err != nil {
		t.Fatal(err)
	}
	if got := writes(c); count(got, "patch configmaps") != 0 || count(got, "create addons") != 0 {
		t.Fatalf("ignored files deployed: %v", got)
	}
}

func TestDeployNamesAddonAfterFirstDotSegment(t *testing.T) {
	c := newClient()
	file := cmManifest("a")
	file.Name = "demo.custom.yaml"
	if err := newDeployer(c).Deploy(context.Background(), []File{file}, nil); err != nil {
		t.Fatal(err)
	}
	mustGetAddon(t, c, "demo")
}

func TestDeployFailedApplyDoesNotRecordChecksum(t *testing.T) {
	c := newClient()
	c.PrependReactor("patch", "configmaps", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(context.DeadlineExceeded)
	})
	if err := newDeployer(c).Deploy(context.Background(), []File{cmManifest("a")}, nil); err == nil {
		t.Fatal("expected error")
	}
	if _, err := c.Resource(addonsGVR).Namespace("kube-system").Get(context.Background(), "demo", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("addon recorded after failed apply: %v", err)
	}
}

var placeholder = regexp.MustCompile(`%\{[A-Z_]+\}%`)

func TestPackagedManifestsRenderWithoutPlaceholders(t *testing.T) {
	files := Packaged()
	names := map[string]bool{}
	for _, f := range files {
		names[f.Name] = true
	}
	for _, want := range []string{"coredns.yaml", "local-storage.yaml", "rolebindings.yaml"} {
		if !names[want] {
			t.Fatalf("packaged set is missing %s", want)
		}
	}
	for _, f := range Render(files, Vars()) {
		if m := placeholder.Find(f.Content); m != nil {
			t.Fatalf("%s still has %s", f.Name, m)
		}
	}
}

func TestVarsMatchTheSupervisorAdvertisement(t *testing.T) {
	v := Vars()
	if v["%{CLUSTER_DNS}%"] != "10.43.0.10" || v["%{CLUSTER_DNS_LIST}%"] != "[10.43.0.10]" {
		t.Fatalf("cluster dns vars = %v", v)
	}
	if v["%{CLUSTER_DOMAIN}%"] != "cluster.local" || v["%{CLUSTER_DNS_IPFAMILYPOLICY}%"] != "SingleStack" {
		t.Fatalf("domain vars = %v", v)
	}
	if v["%{SYSTEM_DEFAULT_REGISTRY}%"] != "" || v["%{DEFAULT_LOCAL_STORAGE_PATH}%"] != "/var/lib/rancher/k3s/storage" {
		t.Fatalf("registry/storage vars = %v", v)
	}
}

func TestPackagedManifestsDecode(t *testing.T) {
	for _, f := range Render(Packaged(), Vars()) {
		objs, err := Decode(f.Content)
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		if len(objs) == 0 {
			t.Fatalf("%s: no objects", f.Name)
		}
	}
}

func TestLocalStorageIsTheDefaultWaitForFirstConsumerClass(t *testing.T) {
	var file File
	for _, f := range Packaged() {
		if f.Name == "local-storage.yaml" {
			file = f
		}
	}
	objs, err := Decode(Render([]File{file}, Vars())[0].Content)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range objs {
		if o.GetKind() != "StorageClass" {
			continue
		}
		mode, _, _ := unstructured.NestedString(o.Object, "volumeBindingMode")
		if o.GetName() != "local-path" || mode != "WaitForFirstConsumer" || o.GetAnnotations()["storageclass.kubernetes.io/is-default-class"] != "true" {
			t.Fatalf("storage class = %v", o.Object)
		}
		return
	}
	t.Fatal("no StorageClass")
}

func TestParseDisable(t *testing.T) {
	got := ParseDisable(" coredns, local-storage ,,")
	if len(got) != 2 || !got["coredns"] || !got["local-storage"] {
		t.Fatalf("got %v", got)
	}
	if len(ParseDisable("")) != 0 {
		t.Fatal("empty list should disable nothing")
	}
}
