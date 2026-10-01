package admission

import (
	"net/http/httptest"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestPodTopologyLabelsCopiedOnBind(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/nodes/n1": mustJSON(t, corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{
				corev1.LabelTopologyZone:   "zone-a",
				corev1.LabelTopologyRegion: "region-1",
				"kubernetes.io/hostname":   "n1",
			}},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, admit.Request{
		Phase:       "admit",
		Name:        "p",
		Namespace:   "default",
		Resource:    schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Subresource: "binding",
		Kind:        schema.GroupVersionKind{Version: "v1", Kind: "Binding"},
		Operation:   "CREATE",
		Object: map[string]any{
			"metadata": map[string]any{"name": "p", "namespace": "default"},
			"target":   map[string]any{"kind": "Node", "name": "n1"},
		},
	})
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	labels := objectLabels(out.Object, nil)
	if labels[corev1.LabelTopologyZone] != "zone-a" || labels[corev1.LabelTopologyRegion] != "region-1" {
		t.Fatalf("labels = %v", labels)
	}
	if labels["kubernetes.io/hostname"] != "" {
		t.Fatalf("copied non-topology label: %v", labels)
	}
}

func TestPodTopologyLabelsCopiedWhenNodeNameSet(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/nodes/n1": mustJSON(t, corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{
				corev1.LabelTopologyZone: "zone-a",
			}},
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
			"metadata": map[string]any{"name": "p", "namespace": "default", "labels": map[string]any{"app": "web"}},
			"spec": map[string]any{
				"nodeName":   "n1",
				"containers": []any{map[string]any{"name": "c", "image": "pause"}},
			},
		},
	})
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	labels := objectLabels(out.Object, nil)
	if labels[corev1.LabelTopologyZone] != "zone-a" || labels["app"] != "web" {
		t.Fatalf("labels = %v", labels)
	}
}

func TestPodTopologyLabelsSkipUnscheduled(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
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
			"metadata": map[string]any{"name": "p"},
			"spec":     map[string]any{"containers": []any{map[string]any{"name": "c", "image": "pause"}}},
		},
	})
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	if len(objectLabels(out.Object, nil)) != 0 {
		t.Fatalf("labels = %v", objectLabels(out.Object, nil))
	}
}
