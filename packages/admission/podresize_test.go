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
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestPodResizeRejectsOverAllocatable(t *testing.T) {
	h := resizeHandler(t, corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{corev1.LabelOSStable: "linux"}},
		Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		}},
	})
	out := postAdmit(t, h, resizeReq(2, "200m", "32Mi"))
	if out.Allowed || !strings.Contains(out.Message, "enough allocatable") {
		t.Fatalf("response = %+v", out)
	}
}

func TestPodResizeAllowsWithinAllocatable(t *testing.T) {
	h := resizeHandler(t, corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{corev1.LabelOSStable: "linux"}},
		Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		}},
	})
	out := postAdmit(t, h, resizeReq(2, "100m", "32Mi"))
	if !out.Allowed {
		t.Fatalf("response = %+v", out)
	}
}

func TestPodResizeRejectsNonLinux(t *testing.T) {
	h := resizeHandler(t, corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{corev1.LabelOSStable: "windows"}},
		Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		}},
	})
	out := postAdmit(t, h, resizeReq(2, "100m", "32Mi"))
	if out.Allowed || !strings.Contains(out.Message, "only supported on linux") {
		t.Fatalf("response = %+v", out)
	}
}

func resizeHandler(t *testing.T, node corev1.Node) http.Handler {
	t.Helper()
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{
		"/registry/nodes/n1": mustJSON(t, node),
	}})
	t.Cleanup(kineSrv.Close)
	return NewHandler(Config{Kine: rewriteClient(kineSrv)})
}

func resizeReq(generation int64, cpu, memory string) admit.Request {
	pod := func(gen int64) map[string]any {
		return map[string]any{
			"metadata": map[string]any{"name": "p", "namespace": "default", "generation": gen},
			"spec": map[string]any{
				"nodeName": "n1",
				"containers": []any{map[string]any{
					"name":  "c",
					"image": "pause",
					"resources": map[string]any{
						"requests": map[string]any{"cpu": cpu, "memory": memory},
					},
				}},
			},
		}
	}
	return admit.Request{
		Phase:       "validate",
		Name:        "p",
		Namespace:   "default",
		Resource:    schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Subresource: "resize",
		Kind:        schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation:   "UPDATE",
		Object:      pod(generation),
		OldObject:   pod(generation - 1),
	}
}
