package customresources

import (
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/controller/openapi/builder"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
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

func TestCustomResourceTypeConverterFromStructuralSchemaMergesListMaps(t *testing.T) {
	crd := &apiextensionsv1.CustomResourceDefinition{
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "stable.example.com",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: "crontabs", Singular: "crontab", Kind: "CronTab", ListKind: "CronTabList"},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name: "v1", Served: true, Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"spec": {Type: "object", Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"jobs": {
								Type:         "array",
								XListType:    stringPtr("map"),
								XListMapKeys: []string{"name"},
								Items: &apiextensionsv1.JSONSchemaPropsOrArray{Schema: &apiextensionsv1.JSONSchemaProps{
									Type:       "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{"name": {Type: "string"}, "image": {Type: "string"}},
								}},
							},
						}},
					},
				}},
			}},
		},
	}
	crd.Name = "crontabs.stable.example.com"
	crdSpec, err := builder.BuildOpenAPIV3(crd, "v1", builder.Options{})
	if err != nil {
		t.Fatal(err)
	}
	staticSpec := &spec3.OpenAPI{
		Version:    "3.0.0",
		Info:       &spec.Info{InfoProps: spec.InfoProps{Title: "Kubernetes CRD Swagger", Version: "v0.1.0"}},
		Components: &spec3.Components{Schemas: staticOpenAPISpec()},
	}
	merged, err := builder.MergeSpecsV3(staticSpec, crdSpec)
	if err != nil {
		t.Fatal(err)
	}
	converter, err := managedfields.NewTypeConverter(merged.Components.Schemas, false)
	if err != nil {
		t.Fatal(err)
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "stable.example.com/v1",
		"kind":       "CronTab",
		"metadata":   map[string]any{"name": "a"},
		"spec":       map[string]any{"jobs": []any{map[string]any{"name": "nightly", "image": "busybox"}}},
	}}
	typed, err := converter.ObjectToTyped(obj)
	if err != nil {
		t.Fatal(err)
	}
	set, err := typed.ToFieldSet()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(set.String(), `[name="nightly"]`) {
		t.Fatalf("spec.jobs is not merged by name: %s", set.String())
	}
}

func stringPtr(s string) *string { return &s }
