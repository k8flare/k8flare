//go:build !js

package apiserver_test

import (
	"context"
	"strings"
	"testing"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var widgetGVR = schema.GroupVersionResource{Group: "test.k8flare.dev", Version: "v1", Resource: "widgets"}

func widgetCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "widgets.test.k8flare.dev"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "test.k8flare.dev",
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: "widgets", Singular: "widget", Kind: "Widget", ListKind: "WidgetList"},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name: "v1", Served: true, Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"spec": {Type: "object", Required: []string{"size"}, Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"size":  {Type: "integer", Minimum: &minSize},
							"color": {Type: "string"},
						}},
					},
				}},
				AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{{Name: "Size", Type: "integer", JSONPath: ".spec.size"}},
			}},
		},
	}
}

func widget(name string, size int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "test.k8flare.dev/v1", "kind": "Widget",
		"metadata": map[string]any{"name": name},
		"spec":     map[string]any{"size": size},
	}}
}

func TestCustomResources(t *testing.T) {
	url, cs := startDevURL(t)
	c := ctx(t)
	cfg := &rest.Config{Host: url, BearerToken: devToken}
	ext := apiextensionsclient.NewForConfigOrDie(cfg)
	dyn := dynamic.NewForConfigOrDie(cfg)

	if _, err := ext.ApiextensionsV1().CustomResourceDefinitions().Create(c, widgetCRD(), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := wait.PollUntilContextTimeout(c, 500*time.Millisecond, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		crd, err := ext.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, "widgets.test.k8flare.dev", metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, cond := range crd.Status.Conditions {
			if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatalf("CRD never became Established: %v", err)
	}

	groups, err := cs.Discovery().ServerGroups()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range groups.Groups {
		if g.Name == "test.k8flare.dev" {
			found = true
		}
	}
	if !found {
		t.Error("/apis does not list test.k8flare.dev")
	}

	widgets := dyn.Resource(widgetGVR).Namespace("default")
	if _, err := widgets.Create(c, widget("w1", 3), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	_, err = widgets.Create(c, widget("bad", 0), metav1.CreateOptions{})
	if !apierrors.IsInvalid(err) {
		t.Errorf("size 0 should fail schema validation, got %v", err)
	}
	list, err := widgets.List(c, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("want 1 widget, got %d", len(list.Items))
	}

	w, err := widgets.Watch(c, metav1.ListOptions{ResourceVersion: list.GetResourceVersion()})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	if _, err := widgets.Create(c, widget("w2", 5), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-w.ResultChan():
		if ev.Type != watch.Added || ev.Object.(*unstructured.Unstructured).GetName() != "w2" {
			t.Errorf("unexpected watch event %s %v", ev.Type, ev.Object)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no watch event for w2")
	}

	var table metav1.Table
	if err := cs.CoreV1().RESTClient().Get().AbsPath("/apis/test.k8flare.dev/v1/namespaces/default/widgets").
		SetHeader("Accept", "application/json;as=Table;v=v1;g=meta.k8s.io,application/json").Do(c).Into(&table); err != nil {
		t.Fatal(err)
	}
	var cols []string
	for _, col := range table.ColumnDefinitions {
		cols = append(cols, col.Name)
	}
	if strings.Join(cols, ",") != "Name,Size" {
		t.Errorf("widget table columns: %v", cols)
	}

	if err := ext.ApiextensionsV1().CustomResourceDefinitions().Delete(c, "widgets.test.k8flare.dev", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := wait.PollUntilContextTimeout(c, 500*time.Millisecond, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := ext.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, "widgets.test.k8flare.dev", metav1.GetOptions{})
		return apierrors.IsNotFound(err), nil
	}); err != nil {
		t.Fatalf("CRD was not finalized and removed: %v", err)
	}
	if _, err := widgets.List(c, metav1.ListOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("widgets should be gone with the CRD, got %v", err)
	}
}

var minSize = 1.0
