package admission

import (
	"net/http/httptest"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestDefaultStorageClassSetsPVC(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{
		"/registry/storageclasses/fast": mustJSON(t, storagev1.StorageClass{
			TypeMeta: metav1.TypeMeta{APIVersion: "storage.k8s.io/v1", Kind: "StorageClass"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              "fast",
				CreationTimestamp: metav1.Now(),
				Annotations:       map[string]string{defaultStorageClassAnnotation: "true"},
			},
			Provisioner: "kubernetes.io/no-provisioner",
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, pvcReq(nil))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	spec, _ := out.Object["spec"].(map[string]any)
	if spec["storageClassName"] != "fast" {
		t.Fatalf("storageClassName = %v", spec["storageClassName"])
	}
}

func TestDefaultStorageClassLeavesExplicitClass(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{
		"/registry/storageclasses/fast": mustJSON(t, storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "fast",
				Annotations: map[string]string{defaultStorageClassAnnotation: "true"},
			},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	named := "slow"
	out := postAdmit(t, h, pvcReq(&named))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	spec, _ := out.Object["spec"].(map[string]any)
	if spec["storageClassName"] != "slow" {
		t.Fatalf("storageClassName = %v", spec["storageClassName"])
	}
}

func TestDefaultStorageClassNoDefaultIsNoop(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, pvcReq(nil))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	spec, _ := out.Object["spec"].(map[string]any)
	if _, ok := spec["storageClassName"]; ok {
		t.Fatalf("storageClassName = %v", spec["storageClassName"])
	}
}

func pvcReq(class *string) admit.Request {
	spec := map[string]any{
		"accessModes": []any{string(corev1.ReadWriteOnce)},
		"resources":   map[string]any{"requests": map[string]any{"storage": "1Gi"}},
	}
	if class != nil {
		spec["storageClassName"] = *class
	}
	return admit.Request{
		Phase:     "admit",
		Name:      "claim",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "PersistentVolumeClaim"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "PersistentVolumeClaim",
			"metadata": map[string]any{"name": "claim", "namespace": "default"},
			"spec":     spec,
		},
	}
}
