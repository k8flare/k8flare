package admission

import (
	"net/http/httptest"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestDefaultIngressClassSetsIngress(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/ingressclasses/web": mustJSON(t, networkingv1.IngressClass{
			TypeMeta: metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "IngressClass"},
			ObjectMeta: metav1.ObjectMeta{
				Name:              "web",
				CreationTimestamp: metav1.Now(),
				Annotations:       map[string]string{networkingv1.AnnotationIsDefaultIngressClass: "true"},
			},
			Spec: networkingv1.IngressClassSpec{Controller: "k8flare.io/ingress"},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, ingressReq(nil))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	spec, _ := out.Object["spec"].(map[string]any)
	if spec["ingressClassName"] != "web" {
		t.Fatalf("ingressClassName = %v", spec["ingressClassName"])
	}
}

func TestDefaultIngressClassLeavesExplicitClass(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/ingressclasses/web": mustJSON(t, networkingv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "web",
				Annotations: map[string]string{networkingv1.AnnotationIsDefaultIngressClass: "true"},
			},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	named := "other"
	out := postAdmit(t, h, ingressReq(&named))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	spec, _ := out.Object["spec"].(map[string]any)
	if spec["ingressClassName"] != "other" {
		t.Fatalf("ingressClassName = %v", spec["ingressClassName"])
	}
}

func TestDefaultIngressClassNoDefaultIsNoop(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, ingressReq(nil))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
	spec, _ := out.Object["spec"].(map[string]any)
	if _, ok := spec["ingressClassName"]; ok {
		t.Fatalf("ingressClassName = %v", spec["ingressClassName"])
	}
}

func ingressReq(class *string) admit.Request {
	spec := map[string]any{
		"rules": []any{map[string]any{"host": "example.test"}},
	}
	if class != nil {
		spec["ingressClassName"] = *class
	}
	return admit.Request{
		Phase:     "admit",
		Name:      "ing",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		Kind:      schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
			"metadata": map[string]any{"name": "ing", "namespace": "default"},
			"spec":     spec,
		},
	}
}
