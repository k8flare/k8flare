package workloads

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"testing"

	"k8s.io/client-go/kubernetes/fake"
)

func TestEverySourceIsNeededAndEveryNeedHasASource(t *testing.T) {
	needed := map[string]bool{}
	for _, resources := range controllerNeeds {
		for _, r := range resources {
			needed[r] = true
		}
	}
	delete(needed, "networking.k8s.io")
	listed := map[string]bool{}
	for _, s := range sources(fake.NewSimpleClientset()) {
		listed[s.name] = true
	}
	if !reflect.DeepEqual(listed, needed) {
		t.Fatalf("sources and controllerNeeds disagree: sources only %v, needs only %v", difference(listed, needed), difference(needed, listed))
	}
}

func TestGeneratedTypeScriptPrefixesMatchTheGoList(t *testing.T) {
	data, err := os.ReadFile("../cluster-store/src/zz_generated_wake.ts")
	if err != nil {
		t.Fatal(err)
	}
	got := regexp.MustCompile(`"(/registry/[^"]+)"`).FindAllStringSubmatch(string(data), -1)
	var prefixes []string
	for _, m := range got {
		prefixes = append(prefixes, m[1])
	}
	sort.Strings(prefixes)
	want := append([]string{}, generatedWorkloadPrefixes...)
	sort.Strings(want)
	if !reflect.DeepEqual(prefixes, want) {
		t.Fatalf("typescript prefixes %v, go prefixes %v", prefixes, want)
	}
}

func difference(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
