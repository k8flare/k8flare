package admission

import (
	"net/http/httptest"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func TestPVCResizeRejectsUnboundExpand(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, pvcResizeReq("1Gi", "2Gi", string(corev1.ClaimPending), "fast"))
	if out.Allowed {
		t.Fatal("expected unbound expand deny")
	}
}

func TestPVCResizeRejectsClassWithoutExpansion(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{
		"/registry/storageclasses/fast": mustJSON(t, storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{Name: "fast"},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, pvcResizeReq("1Gi", "2Gi", string(corev1.ClaimBound), "fast"))
	if out.Allowed {
		t.Fatal("expected class without expansion deny")
	}
}

func TestPVCResizeAllowsExpandWhenClassPermits(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{
		"/registry/storageclasses/fast": mustJSON(t, storagev1.StorageClass{
			ObjectMeta:           metav1.ObjectMeta{Name: "fast"},
			AllowVolumeExpansion: ptr.To(true),
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, pvcResizeReq("1Gi", "2Gi", string(corev1.ClaimBound), "fast"))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
}

func TestPVCResizeAllowsSameSize(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, pvcResizeReq("1Gi", "1Gi", string(corev1.ClaimPending), ""))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
}

func pvcResizeReq(oldSize, newSize, phase, class string) admit.Request {
	old := pvcObject(oldSize, class, phase)
	neu := pvcObject(newSize, class, phase)
	return admit.Request{
		Phase:     "validate",
		Name:      "claim",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "PersistentVolumeClaim"},
		Operation: "UPDATE",
		Object:    neu,
		OldObject: old,
	}
}

func pvcObject(size, class, phase string) map[string]any {
	spec := map[string]any{
		"accessModes": []any{string(corev1.ReadWriteOnce)},
		"resources":   map[string]any{"requests": map[string]any{"storage": size}},
	}
	if class != "" {
		spec["storageClassName"] = class
	}
	return map[string]any{
		"apiVersion": "v1", "kind": "PersistentVolumeClaim",
		"metadata": map[string]any{"name": "claim", "namespace": "default"},
		"spec":     spec,
		"status":   map[string]any{"phase": phase},
	}
}
