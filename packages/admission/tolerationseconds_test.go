package admission

import (
	"net/http/httptest"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const defaultTolerationSeconds = int64(300)

func TestDefaultTolerationSecondsAddsBoth(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, tolerationPodReq(nil))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	got := podTolerations(out.Object)
	if !hasDefaultToleration(got, corev1.TaintNodeNotReady) || !hasDefaultToleration(got, corev1.TaintNodeUnreachable) {
		t.Fatalf("tolerations = %v", got)
	}
}

func TestDefaultTolerationSecondsRestoresDefaultsOnUpdate(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := tolerationPodReq(nil)
	req.Operation = "UPDATE"
	req.OldObject = req.Object
	out := postAdmit(t, h, req)
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	got := podTolerations(out.Object)
	if !hasDefaultToleration(got, corev1.TaintNodeNotReady) || !hasDefaultToleration(got, corev1.TaintNodeUnreachable) {
		t.Fatalf("update should restore the default tolerations: %v", got)
	}
}

func TestDefaultTolerationSecondsKeepsExisting(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	existing := []any{map[string]any{
		"key":    corev1.TaintNodeNotReady,
		"effect": string(corev1.TaintEffectNoExecute),
	}}
	out := postAdmit(t, h, tolerationPodReq(existing))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	got := podTolerations(out.Object)
	notReady := 0
	for _, tln := range got {
		if tln["key"] == corev1.TaintNodeNotReady {
			notReady++
			if _, ok := tln["tolerationSeconds"]; ok {
				t.Fatalf("existing not-ready toleration was rewritten: %v", tln)
			}
		}
	}
	if notReady != 1 || !hasDefaultToleration(got, corev1.TaintNodeUnreachable) {
		t.Fatalf("tolerations = %v", got)
	}
}

func tolerationPodReq(tolerations []any) admit.Request {
	spec := map[string]any{"containers": []any{map[string]any{"name": "c", "image": "img"}}}
	if tolerations != nil {
		spec["tolerations"] = tolerations
	}
	return admit.Request{
		Phase:     "admit",
		Name:      "p",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "p", "namespace": "default"},
			"spec":     spec,
		},
	}
}

func podTolerations(obj map[string]any) []map[string]any {
	spec, _ := obj["spec"].(map[string]any)
	raw, _ := spec["tolerations"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func hasDefaultToleration(list []map[string]any, key string) bool {
	for _, tln := range list {
		if tln["key"] != key {
			continue
		}
		if tln["operator"] != string(corev1.TolerationOpExists) || tln["effect"] != string(corev1.TaintEffectNoExecute) {
			continue
		}
		switch n := tln["tolerationSeconds"].(type) {
		case int64:
			return n == defaultTolerationSeconds
		case float64:
			return int64(n) == defaultTolerationSeconds
		case int:
			return int64(n) == defaultTolerationSeconds
		}
	}
	return false
}
