package admission

import (
	"net/http/httptest"
	"strings"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	nodev1 "k8s.io/api/node/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestRuntimeClassMissingIsForbidden(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, runtimeClassPodReq("missing-rc"))
	if out.Allowed {
		t.Fatal("expected missing RuntimeClass deny")
	}
	if !strings.Contains(out.Message, `pod rejected: RuntimeClass "missing-rc" not found`) {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestRuntimeClassPresentAllowsCreate(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/runtimeclasses/runc": mustJSON(t, nodev1.RuntimeClass{
			TypeMeta:   metav1.TypeMeta{APIVersion: "node.k8s.io/v1", Kind: "RuntimeClass"},
			ObjectMeta: metav1.ObjectMeta{Name: "runc"},
			Handler:    "runc",
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, runtimeClassPodReq("runc"))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
}

func TestRuntimeClassDeletedIsForbidden(t *testing.T) {
	deleted := metav1.Now()
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/runtimeclasses/gone": mustJSON(t, nodev1.RuntimeClass{
			TypeMeta:   metav1.TypeMeta{APIVersion: "node.k8s.io/v1", Kind: "RuntimeClass"},
			ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &deleted},
			Handler:    "runc",
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, runtimeClassPodReq("gone"))
	if out.Allowed {
		t.Fatal("expected deleted RuntimeClass deny")
	}
	if out.Reason != "" && out.Reason != "Forbidden" {
		t.Fatalf("reason = %q", out.Reason)
	}
	if !strings.Contains(out.Message, `pod rejected: RuntimeClass "gone" not found`) {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestRuntimeClassUnsetAllowsCreate(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := runtimeClassPodReq("")
	spec := req.Object["spec"].(map[string]any)
	delete(spec, "runtimeClassName")
	out := postAdmit(t, h, req)
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
}

func runtimeClassPodReq(rcName string) admit.Request {
	spec := map[string]any{"containers": []any{map[string]any{"name": "c", "image": "img"}}}
	if rcName != "" {
		spec["runtimeClassName"] = rcName
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
