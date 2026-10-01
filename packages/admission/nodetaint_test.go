package admission

import (
	"net/http/httptest"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestTaintNodesByConditionAddsNotReady(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, nodeCreateReq(nil))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	if !hasNotReadyNoSchedule(nodeTaints(out.Object)) {
		t.Fatalf("taints = %v", nodeTaints(out.Object))
	}
}

func TestTaintNodesByConditionKeepsExisting(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	existing := []any{map[string]any{
		"key":    corev1.TaintNodeNotReady,
		"effect": string(corev1.TaintEffectNoSchedule),
	}}
	out := postAdmit(t, h, nodeCreateReq(existing))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	got := nodeTaints(out.Object)
	n := 0
	for _, taint := range got {
		if taint["key"] == corev1.TaintNodeNotReady && taint["effect"] == string(corev1.TaintEffectNoSchedule) {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("taints = %v", got)
	}
}

func TestTaintNodesByConditionSkipsUpdate(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodeCreateReq(nil)
	req.Operation = "UPDATE"
	out := postAdmit(t, h, req)
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	if hasNotReadyNoSchedule(nodeTaints(out.Object)) {
		t.Fatalf("update should not add taint: %v", nodeTaints(out.Object))
	}
}

func nodeCreateReq(taints []any) admit.Request {
	spec := map[string]any{}
	if taints != nil {
		spec["taints"] = taints
	}
	return admit.Request{
		Phase:     "admit",
		Name:      "probe",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "nodes"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Node"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Node",
			"metadata": map[string]any{"name": "probe"},
			"spec":     spec,
		},
	}
}

func nodeTaints(obj map[string]any) []map[string]any {
	spec, _ := obj["spec"].(map[string]any)
	raw, _ := spec["taints"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func hasNotReadyNoSchedule(taints []map[string]any) bool {
	for _, taint := range taints {
		if taint["key"] == corev1.TaintNodeNotReady && taint["effect"] == string(corev1.TaintEffectNoSchedule) {
			return true
		}
	}
	return false
}
