package admission

import (
	"net/http/httptest"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func TestPriorityClassSetsPodPriority(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{
		"/registry/priorityclasses/high": mustJSON(t, schedulingv1.PriorityClass{
			TypeMeta:         metav1.TypeMeta{APIVersion: "scheduling.k8s.io/v1", Kind: "PriorityClass"},
			ObjectMeta:       metav1.ObjectMeta{Name: "high"},
			Value:            1000,
			PreemptionPolicy: ptr.To(corev1.PreemptLowerPriority),
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, priorityPodReq("high", nil))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	spec, _ := out.Object["spec"].(map[string]any)
	if spec["priorityClassName"] != "high" {
		t.Fatalf("priorityClassName = %v", spec["priorityClassName"])
	}
	if intFromJSON(spec["priority"]) != 1000 {
		t.Fatalf("priority = %v", spec["priority"])
	}
	if spec["preemptionPolicy"] != string(corev1.PreemptLowerPriority) {
		t.Fatalf("preemptionPolicy = %v", spec["preemptionPolicy"])
	}
}

func TestPriorityClassMissingIsForbidden(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, priorityPodReq("missing", nil))
	if out.Allowed {
		t.Fatal("expected missing PriorityClass deny")
	}
}

func TestPriorityDefaultWhenNoClass(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, priorityPodReq("", nil))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	spec, _ := out.Object["spec"].(map[string]any)
	if intFromJSON(spec["priority"]) != 0 {
		t.Fatalf("priority = %v", spec["priority"])
	}
	if spec["preemptionPolicy"] != string(corev1.PreemptLowerPriority) {
		t.Fatalf("preemptionPolicy = %v", spec["preemptionPolicy"])
	}
}

func TestPriorityRejectsMismatchedInteger(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{
		"/registry/priorityclasses/high": mustJSON(t, schedulingv1.PriorityClass{
			TypeMeta:   metav1.TypeMeta{APIVersion: "scheduling.k8s.io/v1", Kind: "PriorityClass"},
			ObjectMeta: metav1.ObjectMeta{Name: "high"},
			Value:      1000,
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, priorityPodReq("high", ptr.To(int32(1))))
	if out.Allowed {
		t.Fatal("expected mismatched priority deny")
	}
}

func TestPriorityPreservedOnUpdate(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := priorityPodReq("", nil)
	req.Operation = "UPDATE"
	req.OldObject = map[string]any{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{"name": "p", "namespace": "default"},
		"spec":     map[string]any{"priority": int32(1000), "preemptionPolicy": string(corev1.PreemptLowerPriority), "containers": []any{map[string]any{"name": "c", "image": "img"}}},
	}
	out := postAdmit(t, h, req)
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	spec, _ := out.Object["spec"].(map[string]any)
	if intFromJSON(spec["priority"]) != 1000 {
		t.Fatalf("priority = %v", spec["priority"])
	}
}

func priorityPodReq(class string, priority *int32) admit.Request {
	spec := map[string]any{"containers": []any{map[string]any{"name": "c", "image": "img"}}}
	if class != "" {
		spec["priorityClassName"] = class
	}
	if priority != nil {
		spec["priority"] = *priority
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

func intFromJSON(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	default:
		return -1
	}
}
