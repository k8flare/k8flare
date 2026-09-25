package metricsapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseProm(t *testing.T) {
	text := "# comment\nnode_cpu_usage_seconds_total{node=\"n1\"} 1.5\ncontainer_cpu_usage_seconds_total{namespace=\"default\",pod=\"web\"} 2\n"
	snap := emptySnap()
	Apply(snap, "fallback", text)
	if snap.Nodes["n1"].CPUSeconds != 1.5 {
		t.Fatal(snap.Nodes)
	}
	if snap.Pods["default/web"].CPUSeconds != 2 {
		t.Fatal(snap.Pods)
	}
}

func TestUsageDelta(t *testing.T) {
	snap := Snapshot{At: 20_000, PrevAt: 10_000, Nodes: map[string]Usage{"n": {CPUSeconds: 3, MemoryBytes: 10}}, PrevNodes: map[string]Usage{"n": {CPUSeconds: 1}}}
	got := usage(snap.Nodes["n"], &Usage{CPUSeconds: 1}, snap)
	if got["cpu"] != "200000000n" || got["memory"] != "10" {
		t.Fatal(got)
	}
	if windowOf(snap) != "10s" {
		t.Fatal(windowOf(snap))
	}
}

func TestServeNode(t *testing.T) {
	snap := Snapshot{At: 1_000, Nodes: map[string]Usage{"n": {MemoryBytes: 4}}, Pods: map[string]Usage{"default/web": {MemoryBytes: 8}}}
	rr := httptest.NewRecorder()
	writeJSON(rr, nodeMetrics(snap, "n"))
	if !strings.Contains(rr.Body.String(), `"name":"n"`) {
		t.Fatal(rr.Body.String())
	}
	rr = httptest.NewRecorder()
	writeJSON(rr, podMetrics(snap, "default", "web"))
	if !strings.Contains(rr.Body.String(), "PodMetrics") {
		t.Fatal(rr.Body.String())
	}
	if nodeMetrics(snap, "missing") != nil {
		t.Fatal("missing")
	}
}

func TestSameUsageIgnoresTimestamps(t *testing.T) {
	a := Snapshot{At: 1, Nodes: map[string]Usage{"n": {CPUSeconds: 1}}, Pods: map[string]Usage{"default/web": {MemoryBytes: 2}}}
	b := a
	b.At = 2
	b.PrevAt = 1
	if !sameUsage(a, b) {
		t.Fatal("timestamps")
	}
	b.Nodes = map[string]Usage{"n": {CPUSeconds: 2}}
	if sameUsage(a, b) {
		t.Fatal("cpu")
	}
}
