package admission

import (
	"net/http/httptest"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"k8s.io/apimachinery/pkg/runtime/schema"
	volumeutil "k8s.io/kubernetes/pkg/volume/util"
)

func TestStorageProtectionAddsPVCFinalizer(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, pvcReq(nil))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	if !hasFinalizer(out.Object, volumeutil.PVCProtectionFinalizer) {
		t.Fatalf("finalizers = %v", objectFinalizers(out.Object))
	}
}

func TestStorageProtectionKeepsExistingPVCFinalizer(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := pvcReq(nil)
	req.Object["metadata"].(map[string]any)["finalizers"] = []any{volumeutil.PVCProtectionFinalizer, "example.com/keep"}
	out := postAdmit(t, h, req)
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	got := objectFinalizers(out.Object)
	n := 0
	for _, f := range got {
		if f == volumeutil.PVCProtectionFinalizer {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("finalizers = %v", got)
	}
}

func TestStorageProtectionAddsPVFinalizer(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "admit",
		Name:      "pv",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumes"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "PersistentVolume"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "PersistentVolume",
			"metadata": map[string]any{"name": "pv"},
			"spec":     map[string]any{"capacity": map[string]any{"storage": "1Gi"}},
		},
	})
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	if !hasFinalizer(out.Object, volumeutil.PVProtectionFinalizer) {
		t.Fatalf("finalizers = %v", objectFinalizers(out.Object))
	}
}

func objectFinalizers(obj map[string]any) []string {
	meta, _ := obj["metadata"].(map[string]any)
	raw, _ := meta["finalizers"].([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func hasFinalizer(obj map[string]any, name string) bool {
	for _, f := range objectFinalizers(obj) {
		if f == name {
			return true
		}
	}
	return false
}
