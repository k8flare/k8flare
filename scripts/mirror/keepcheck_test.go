package main

import (
	"strings"
	"testing"
)

func refs(clients, informers []string, factory ...string) keepRefs {
	r := keepRefs{clients: map[string]string{}, informerVersions: map[string]string{}, factoryGroups: map[string]string{}}
	for _, c := range clients {
		r.clients[c] = "pos:" + c
	}
	for _, i := range informers {
		r.informerVersions[i] = "pos:" + i
	}
	for _, f := range factory {
		r.factoryGroups[f] = "pos:" + f
	}
	return r
}

func TestDiffKeepAcceptsConsistentTable(t *testing.T) {
	table := []keptGroup{
		{Name: "apps", Versions: []string{"v1"}, InformerFactory: true, NarrowInformer: true},
		{Name: "rbac", Versions: []string{"v1"}},
	}
	got := diffKeep(table, refs([]string{"apps/v1", "rbac/v1"}, []string{"apps/v1"}, "apps"),
		map[string][]string{"apps": {"v1", "v1beta1"}}, nil)
	if len(got) != 0 {
		t.Fatalf("unexpected problems: %v", got)
	}
}

func TestDiffKeepNamesUnreferencedVersion(t *testing.T) {
	table := []keptGroup{{Name: "autoscaling", Versions: []string{"v1", "v2"}}}
	got := diffKeep(table, refs([]string{"autoscaling/v2"}, nil), nil, nil)
	if len(got) != 1 || !strings.Contains(got[0], "autoscaling/v1") {
		t.Fatalf("want one problem naming autoscaling/v1, got %v", got)
	}
}

func TestDiffKeepCountsInformerVersionAsReference(t *testing.T) {
	table := []keptGroup{{Name: "resource", Versions: []string{"v1beta2"}, InformerFactory: true, NarrowInformer: true}}
	got := diffKeep(table, refs(nil, []string{"resource/v1beta2"}, "resource"),
		map[string][]string{"resource": {"v1beta2", "v1"}}, nil)
	if len(got) != 0 {
		t.Fatalf("unexpected problems: %v", got)
	}
}

func TestDiffKeepNamesUnreferencedFactoryGroup(t *testing.T) {
	table := []keptGroup{{Name: "coordination", Versions: []string{"v1"}, InformerFactory: true, NarrowInformer: true}}
	up := map[string][]string{"coordination": {"v1", "v1beta1"}}
	got := diffKeep(table, refs([]string{"coordination/v1"}, nil), up, nil)
	if len(got) != 1 || !strings.Contains(got[0], "coordination") || !strings.Contains(got[0], "InformerFactory") {
		t.Fatalf("want one InformerFactory problem for coordination, got %v", got)
	}
	allow := map[string]string{"coordination/InformerFactory": "reason"}
	if got := diffKeep(table, refs([]string{"coordination/v1"}, nil), up, allow); len(got) != 0 {
		t.Fatalf("allow-list ignored: %v", got)
	}
}

func TestDiffKeepFlagsStaleAllowListEntry(t *testing.T) {
	table := []keptGroup{{Name: "apps", Versions: []string{"v1"}, InformerFactory: true, NarrowInformer: true}}
	allow := map[string]string{"apps/InformerFactory": "reason"}
	got := diffKeep(table, refs([]string{"apps/v1"}, []string{"apps/v1"}, "apps"), map[string][]string{"apps": {"v1", "v1beta1"}}, allow)
	if len(got) != 1 || !strings.Contains(got[0], "apps/InformerFactory") {
		t.Fatalf("want a stale allow-list problem, got %v", got)
	}
}

func TestDiffKeepDerivesNarrowInformerFromUpstreamVersions(t *testing.T) {
	r := refs([]string{"autoscaling/v1", "autoscaling/v2"}, []string{"autoscaling/v2"}, "autoscaling")
	table := []keptGroup{{Name: "autoscaling", Versions: []string{"v1", "v2"}, InformerFactory: true}}
	if got := diffKeep(table, r, map[string][]string{"autoscaling": {"v1", "v2"}}, nil); len(got) != 0 {
		t.Fatalf("unexpected problems: %v", got)
	}
	got := diffKeep(table, r, map[string][]string{"autoscaling": {"v1", "v2", "v3"}}, nil)
	if len(got) != 1 || !strings.Contains(got[0], "NarrowInformer") {
		t.Fatalf("want a NarrowInformer problem, got %v", got)
	}
}

func TestDiffKeepNamesReferencesMissingFromTable(t *testing.T) {
	table := []keptGroup{{Name: "apps", Versions: []string{"v1"}}}
	got := diffKeep(table, refs([]string{"apps/v1", "batch/v1"}, []string{"apps/v1"}, "apps"), nil, nil)
	joined := strings.Join(got, "\n")
	for _, want := range []string{"batch/v1", "InformerFactory"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
}

func TestMissingClientHints(t *testing.T) {
	got := missingClientHints([]string{
		"x.go:1:2: c.CoreV2 undefined (type kubernetes.Interface has no field or method CoreV2)",
		"y.go:1:2: f.Coordination undefined (type informers.SharedInformerFactory has no field or method Coordination)",
		"z.go:1:2: unrelated",
	})
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, `"core"`) || !strings.Contains(joined, `"v2"`) || !strings.Contains(joined, "coordination") || strings.Contains(joined, "unrelated") {
		t.Fatalf("hints: %s", joined)
	}
}
