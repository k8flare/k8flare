package customresources

import (
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCRDTypeConverterKnowsListMapKeys(t *testing.T) {
	converter, err := crdTypeConverter()
	if err != nil {
		t.Fatal(err)
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiextensionsv1.SchemeGroupVersion.String(),
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": "crontabs.stable.example.com"},
		"status": map[string]any{
			"conditions": []any{map[string]any{"type": "Established", "status": "True"}},
		},
	}}
	typed, err := converter.ObjectToTyped(obj)
	if err != nil {
		t.Fatal(err)
	}
	set, err := typed.ToFieldSet()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(set.String(), `[type="Established"]`) {
		t.Fatalf("status.conditions is not merged by type: %s", set.String())
	}
}
