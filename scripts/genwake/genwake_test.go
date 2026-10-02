package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/k8flare/k8flare/scripts/internal/upstream"
	"golang.org/x/tools/go/packages"
)

var (
	loadOnce   sync.Once
	loadedRoot string
	loadedPkgs []*packages.Package
	loadErr    error
)

func shards(t *testing.T) (string, []*packages.Package) {
	t.Helper()
	loadOnce.Do(func() {
		loadedRoot, loadErr = upstream.RepoRoot()
		if loadErr != nil {
			return
		}
		loadedPkgs, loadErr = load(loadedRoot)
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	return loadedRoot, loadedPkgs
}

func repoKinds(t *testing.T) kindResources {
	t.Helper()
	root, _ := shards(t)
	kinds, err := loadKinds(filepath.Join(root, servedRegistryFile))
	if err != nil {
		t.Fatal(err)
	}
	return kinds
}

func TestExtractReadsTheInformersEachConstructorIsHanded(t *testing.T) {
	_, pkgs := shards(t)
	got, err := extract(pkgs, repoKinds(t))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"replicaset":                {"pods", "replicasets"},
		"csrsigning":                {"certificatesigningrequests"},
		"csrcleaner":                {"certificatesigningrequests"},
		"endpoints":                 {"endpoints", "pods", "services"},
		"rootca":                    {"configmaps", "namespaces"},
		"persistentvolume":          {"nodes", "persistentvolumeclaims", "persistentvolumes", "pods", "storageclasses"},
		"resourcequota":             {"resourcequotas"},
		"validatingadmissionpolicy": {"validatingadmissionpolicies"},
		"volumeexpand":              {"persistentvolumeclaims"},
		"disruption":                {"deployments", "poddisruptionbudgets", "pods", "replicasets", "replicationcontrollers", "statefulsets"},
	}
	for controller, resources := range want {
		if !reflect.DeepEqual(got.sorted(controller), resources) {
			t.Errorf("%s = %v, want %v", controller, got.sorted(controller), resources)
		}
	}
	if _, ok := got["tainteviction"]; ok {
		t.Errorf("tainteviction is built outside the shards but was derived: %v", got.sorted("tainteviction"))
	}
}

func TestExtractNamesTheControllerWhoseInformerHasNoResource(t *testing.T) {
	_, pkgs := shards(t)
	kinds := repoKinds(t)
	for key := range kinds {
		if key.kind == "Pod" {
			delete(kinds, key)
		}
	}
	_, err := extract(pkgs, kinds)
	if err == nil || !strings.Contains(err.Error(), "controller replicaset") || !strings.Contains(err.Error(), "Pod") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadKindsSkipsSubresources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "served.go")
	src := `package registry

var Served = []ServedGroupVersion{
	{GV: schema.GroupVersion{Group: "", Version: "v1"}, Resources: []metav1.APIResource{
		{Name: "pods", Kind: "Pod"},
		{Name: "pods/status", Kind: "Pod"},
	}},
	{GV: schema.GroupVersion{Group: "apps", Version: "v1"}, Resources: []metav1.APIResource{
		{Name: "deployments", Kind: "Deployment"},
	}},
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	kinds, err := loadKinds(path)
	if err != nil {
		t.Fatal(err)
	}
	want := kindResources{
		{pkg: "k8s.io/api/core/v1", kind: "Pod"}:        "pods",
		{pkg: "k8s.io/api/apps/v1", kind: "Deployment"}: "deployments",
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("kinds = %v", kinds)
	}
}

func TestLoadKindsFailsWithoutServed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "served.go")
	if err := os.WriteFile(path, []byte("package registry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadKinds(path); err == nil || !strings.Contains(err.Error(), "Served") {
		t.Fatalf("err = %v", err)
	}
}

func TestQuotaResourcesReadsEveryCase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota.go")
	src := `package workloads

func quotaExample(gvr schema.GroupVersionResource) (runtime.Object, bool) {
	switch gvr {
	case v1.SchemeGroupVersion.WithResource("pods"):
		return &v1.Pod{}, true
	case appsv1.SchemeGroupVersion.WithResource("replicasets"):
		return &appsv1.ReplicaSet{}, true
	}
	return nil, false
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := quotaResources(path, "quotaExample")
	if err != nil || !reflect.DeepEqual(got, []string{"pods", "replicasets"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := quotaResources(path, "missing"); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyOutsideConstructorsRejectsWhatIsAlreadyDerived(t *testing.T) {
	derived := needs{}
	derived.add("disruption", "jobs")
	err := applyOutsideConstructors(derived, map[string][]string{"disruption": {"jobs"}})
	if err == nil || !strings.Contains(err.Error(), "disruption -> jobs") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorkloadPrefixes(t *testing.T) {
	derived := needs{}
	derived.add("a", "pods")
	derived.add("a", "secrets")
	derived.add("b", "secrets")
	derived.add("b", "networking.k8s.io")
	derived.add("b", "ipaddresses")
	outside := map[string][]string{"b": {"networking.k8s.io"}}
	got, err := workloadPrefixes(derived, outside, []string{"pods"}, []string{"ipaddresses"}, []string{"/registry/gateway.networking.k8s.io/"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/registry/gateway.networking.k8s.io/", "/registry/secrets/"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if _, err := workloadPrefixes(derived, outside, []string{"nodes"}, nil, nil); err == nil || !strings.Contains(err.Error(), "nodes") {
		t.Fatalf("a stale exclusion must fail: %v", err)
	}
	if _, err := workloadPrefixes(derived, outside, []string{"pods"}, []string{"ipaddresses"}, []string{"/registry/secrets/"}); err == nil || !strings.Contains(err.Error(), "/registry/secrets/") {
		t.Fatalf("a redundant extra prefix must fail: %v", err)
	}
}

func TestRenderNeedsSortsControllersAndResources(t *testing.T) {
	n := needs{}
	n.add("b", "services")
	n.add("b", "pods")
	n.add("a", "nodes")
	out, err := renderNeeds(n)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.HasPrefix(text, "// Code generated by scripts/genwake") || !strings.HasSuffix(text, "DO NOT EDIT.\n\npackage workloads\n\nvar controllerNeeds = map[string][]string{\n\t\"a\": {\"nodes\"},\n\t\"b\": {\"pods\", \"services\"},\n}\n") {
		t.Fatalf("output:\n%s", text)
	}
}
