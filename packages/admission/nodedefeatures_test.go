package admission

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestNodeDeclaredFeaturesRejectsPodLevelResize(t *testing.T) {
	h := declaredFeatureHandler(t, nil)
	out := postAdmit(t, h, podLevelResizeReq())
	if out.Allowed || !strings.Contains(out.Message, "InPlacePodLevelResourcesVerticalScaling") {
		t.Fatalf("response = %+v", out)
	}
}

func TestNodeDeclaredFeaturesAllowsDeclared(t *testing.T) {
	h := declaredFeatureHandler(t, []string{"InPlacePodLevelResourcesVerticalScaling"})
	out := postAdmit(t, h, podLevelResizeReq())
	if !out.Allowed {
		t.Fatalf("response = %+v", out)
	}
}

func declaredFeatureHandler(t *testing.T, features []string) http.Handler {
	t.Helper()
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{
		"/registry/nodes/n1": mustJSON(t, corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status:     corev1.NodeStatus{DeclaredFeatures: features},
		}),
	}})
	t.Cleanup(kineSrv.Close)
	return NewHandler(Config{Kine: rewriteClient(kineSrv)})
}

func podLevelResizeReq() admit.Request {
	base := map[string]any{
		"metadata": map[string]any{"name": "p", "namespace": "default"},
		"spec": map[string]any{
			"nodeName":   "n1",
			"containers": []any{map[string]any{"name": "c", "image": "pause"}},
		},
	}
	old := cloneMap(base)
	old["metadata"].(map[string]any)["generation"] = int64(1)
	neu := cloneMap(base)
	neu["metadata"].(map[string]any)["generation"] = int64(2)
	neu["spec"].(map[string]any)["resources"] = map[string]any{
		"requests": map[string]any{"cpu": "100m"},
	}
	return admit.Request{
		Phase:     "validate",
		Name:      "p",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "UPDATE",
		Object:    neu,
		OldObject: old,
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		switch t := v.(type) {
		case map[string]any:
			out[k] = cloneMap(t)
		default:
			out[k] = v
		}
	}
	return out
}
