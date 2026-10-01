package admission

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestPVCLimitRangeRejectsBelowMin(t *testing.T) {
	h := newPVCLimitHandler(t, "1Gi", "2Gi")
	out := postAdmit(t, h, pvcLimitReq("500Mi"))
	if out.Allowed {
		t.Fatal("expected min deny")
	}
	if !strings.Contains(out.Message, "minimum storage usage per PersistentVolumeClaim is 1Gi, but request is 500Mi") {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestPVCLimitRangeRejectsAboveMax(t *testing.T) {
	h := newPVCLimitHandler(t, "1Gi", "2Gi")
	out := postAdmit(t, h, pvcLimitReq("3Gi"))
	if out.Allowed {
		t.Fatal("expected max deny")
	}
	if !strings.Contains(out.Message, "maximum storage usage per PersistentVolumeClaim is 2Gi, but request is 3Gi") {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestPVCLimitRangeAllowsInRange(t *testing.T) {
	h := newPVCLimitHandler(t, "1Gi", "2Gi")
	out := postAdmit(t, h, pvcLimitReq("1Gi"))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
}

func TestPVCLimitRangeNoopWithoutRange(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	t.Cleanup(kineSrv.Close)
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, pvcLimitReq("500Mi"))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
}

func TestPVCLimitRangeRejectsMissingRequest(t *testing.T) {
	h := newPVCLimitHandler(t, "1Gi", "")
	req := pvcLimitReq("1Gi")
	spec, _ := req.Object["spec"].(map[string]any)
	spec["resources"] = map[string]any{"requests": map[string]any{}}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected missing request deny")
	}
	if !strings.Contains(out.Message, "minimum storage usage per PersistentVolumeClaim is 1Gi.  No request is specified") {
		t.Fatalf("message = %q", out.Message)
	}
}

func newPVCLimitHandler(t *testing.T, min, max string) http.Handler {
	t.Helper()
	item := corev1.LimitRangeItem{Type: corev1.LimitTypePersistentVolumeClaim}
	if min != "" {
		item.Min = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(min)}
	}
	if max != "" {
		item.Max = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(max)}
	}
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/limitranges/default/pvc-limit": mustJSON(t, corev1.LimitRange{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "LimitRange"},
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-limit", Namespace: "default"},
			Spec:       corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{item}},
		}),
	}})
	t.Cleanup(kineSrv.Close)
	return NewHandler(Config{Kine: rewriteClient(kineSrv)})
}

func pvcLimitReq(size string) admit.Request {
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
			"spec": map[string]any{
				"accessModes": []any{string(corev1.ReadWriteOnce)},
				"resources":   map[string]any{"requests": map[string]any{"storage": size}},
			},
		},
	}
}

func TestLimitRangerDeniesContainerBelowMin(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/limitranges/default/lr": mustJSON(t, corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: "lr", Namespace: "default"},
			Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
				Type: corev1.LimitTypeContainer,
				Min:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
				Max:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
			}}},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "admit",
		Name:      "p",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "p", "namespace": "default"},
			"spec": map[string]any{"containers": []any{map[string]any{
				"name": "c", "image": "img",
				"resources": map[string]any{"requests": map[string]any{"cpu": "10m"}},
			}}},
		},
	})
	if out.Allowed {
		t.Fatal("expected container min deny")
	}
	if !strings.Contains(out.Message, "minimum cpu usage per Container is 50m, but request is 10m") {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestLimitRangerDeniesPodAboveMax(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/limitranges/default/lr": mustJSON(t, corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: "lr", Namespace: "default"},
			Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
				Type: corev1.LimitTypePod,
				Max:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("150m")},
			}}},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "admit",
		Name:      "p",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "p", "namespace": "default"},
			"spec": map[string]any{"containers": []any{
				map[string]any{"name": "a", "image": "img", "resources": map[string]any{"limits": map[string]any{"cpu": "100m"}}},
				map[string]any{"name": "b", "image": "img", "resources": map[string]any{"limits": map[string]any{"cpu": "100m"}}},
			}},
		},
	})
	if out.Allowed {
		t.Fatal("expected pod max deny")
	}
	if !strings.Contains(out.Message, "maximum cpu usage per Pod is 150m, but limit is 200m") {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestLimitRangerDeniesContainerMissingLimit(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/limitranges/default/lr": mustJSON(t, corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: "lr", Namespace: "default"},
			Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
				Type: corev1.LimitTypeContainer,
				Max:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			}}},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "admit",
		Name:      "p",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "p", "namespace": "default"},
			"spec": map[string]any{"containers": []any{map[string]any{"name": "c", "image": "img"}}},
		},
	})
	if out.Allowed {
		t.Fatal("expected missing limit deny")
	}
	if !strings.Contains(out.Message, "maximum cpu usage per Container is 100m. No limit is specified") {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestLimitRangerDeniesUnstructuredBarePod(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/limitranges/default/lr": mustJSON(t, corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: "lr", Namespace: "default"},
			Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
				Type: corev1.LimitTypeContainer,
				Max:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			}}},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	pod := &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
	}
	scheme.Scheme.Default(pod)
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pod)
	if err != nil {
		t.Fatal(err)
	}
	out := postAdmit(t, h, admit.Request{
		Phase:     "admit",
		Name:      "p",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object:    obj,
	})
	if out.Allowed {
		t.Fatalf("expected missing limit deny, object=%v", obj["spec"])
	}
	if !strings.Contains(out.Message, "maximum cpu usage per Container is 100m. No limit is specified") {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestLimitRangerDeniesLimitRequestRatio(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/limitranges/default/lr": mustJSON(t, corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: "lr", Namespace: "default"},
			Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
				Type:                 corev1.LimitTypeContainer,
				MaxLimitRequestRatio: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
			}}},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "admit",
		Name:      "p",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "p", "namespace": "default"},
			"spec": map[string]any{"containers": []any{map[string]any{
				"name": "c", "image": "img",
				"resources": map[string]any{
					"requests": map[string]any{"cpu": "100m"},
					"limits":   map[string]any{"cpu": "400m"},
				},
			}}},
		},
	})
	if out.Allowed {
		t.Fatal("expected ratio deny")
	}
	if !strings.Contains(out.Message, "cpu max limit to request ratio per Container is 2, but provided ratio is") {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestLimitRangerDeniesPodMissingLimit(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/limitranges/default/lr": mustJSON(t, corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: "lr", Namespace: "default"},
			Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
				Type: corev1.LimitTypePod,
				Max:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			}}},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "admit",
		Name:      "p",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "p", "namespace": "default"},
			"spec":     map[string]any{"containers": []any{map[string]any{"name": "c", "image": "img"}}},
		},
	})
	if out.Allowed {
		t.Fatal("expected pod missing limit deny")
	}
	if !strings.Contains(out.Message, "maximum cpu usage per Pod is 100m. No limit is specified") {
		t.Fatalf("message = %q", out.Message)
	}
}
