package scheduling

import (
	"testing"

	schedhelpers "k8s.io/kubernetes/pkg/apis/scheduling/v1"
)

func TestSystemPriorityClassesMatchUpstream(t *testing.T) {
	got := schedhelpers.SystemPriorityClasses()
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	byName := map[string]int32{}
	for _, pc := range got {
		byName[pc.Name] = pc.Value
	}
	if byName["system-node-critical"] != 2000001000 {
		t.Fatalf("system-node-critical = %d", byName["system-node-critical"])
	}
	if byName["system-cluster-critical"] != 2000000000 {
		t.Fatalf("system-cluster-critical = %d", byName["system-cluster-critical"])
	}
}
