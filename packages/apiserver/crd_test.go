//go:build !js

package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
							"size":     {Type: "integer", Minimum: &minSize},
							"color":    {Type: "string"},
							"replicas": {Type: "integer"},
						}, XValidations: apiextensionsv1.ValidationRules{{Rule: "self.size <= 10", Message: "size must be at most 10"}}},
						"status": {Type: "object", Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"replicas": {Type: "integer"},
							"selector": {Type: "string"},
						}},
					},
				}},
				Subresources: &apiextensionsv1.CustomResourceSubresources{
					Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					Scale:  &apiextensionsv1.CustomResourceSubresourceScale{SpecReplicasPath: ".spec.replicas", StatusReplicasPath: ".status.replicas", LabelSelectorPath: &widgetSelectorPath},
				},
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

	var discovery struct {
		Paths map[string]struct {
			ServerRelativeURL string `json:"serverRelativeURL"`
		} `json:"paths"`
	}
	if err := wait.PollUntilContextTimeout(c, 500*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		body, err := cs.CoreV1().RESTClient().Get().AbsPath("/openapi/v3").SetHeader("Accept", "application/json").DoRaw(ctx)
		if err != nil {
			return false, err
		}
		discovery.Paths = nil
		if err := json.Unmarshal(body, &discovery); err != nil {
			return false, err
		}
		_, hasCRDGroup := discovery.Paths["apis/test.k8flare.dev/v1"]
		return hasCRDGroup, nil
	}); err != nil {
		t.Fatalf("openapi/v3 never listed apis/test.k8flare.dev/v1: %v", err)
	}
	if _, ok := discovery.Paths["api/v1"]; !ok {
		t.Error("/openapi/v3 does not list api/v1")
	}

	widgetDoc := discovery.Paths["apis/test.k8flare.dev/v1"]
	docBody, err := cs.CoreV1().RESTClient().Get().RequestURI(widgetDoc.ServerRelativeURL).SetHeader("Accept", "application/json").DoRaw(c)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(docBody, &doc); err != nil {
		t.Fatal(err)
	}
	widgetSchema := false
	for name := range doc.Components.Schemas {
		if strings.HasSuffix(name, ".v1.Widget") {
			widgetSchema = true
		}
	}
	if !widgetSchema {
		t.Errorf("openapi v3 doc for test.k8flare.dev/v1 has no *.v1.Widget schema, got %v", slices.Sorted(maps.Keys(doc.Components.Schemas)))
	}

	_, err = widgets.Create(c, widget("big", 11), metav1.CreateOptions{})
	wantInvalid(t, err, `Widget.test.k8flare.dev "big" is invalid: spec: Invalid value: size must be at most 10`,
		metav1.StatusCause{Type: metav1.CauseTypeFieldValueInvalid, Message: "Invalid value: size must be at most 10", Field: "spec"})
	w1, err := widgets.Get(c, "w1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	w1.Object["spec"].(map[string]any)["size"] = int64(11)
	_, err = widgets.Update(c, w1, metav1.UpdateOptions{})
	wantInvalid(t, err, `Widget.test.k8flare.dev "w1" is invalid: spec: Invalid value: size must be at most 10`,
		metav1.StatusCause{Type: metav1.CauseTypeFieldValueInvalid, Message: "Invalid value: size must be at most 10", Field: "spec"})

	resources, err := cs.Discovery().ServerResourcesForGroupVersion("test.k8flare.dev/v1")
	if err != nil {
		t.Fatal(err)
	}
	discovered := map[string]metav1.APIResource{}
	for _, r := range resources.APIResources {
		discovered[r.Name] = r
	}
	subresourceVerbs := []string{"get", "patch", "update"}
	if r := discovered["widgets/status"]; r.Kind != "Widget" || !r.Namespaced || !slices.Equal(slices.Sorted(slices.Values(r.Verbs)), subresourceVerbs) {
		t.Errorf("discovery widgets/status: %+v", r)
	}
	if r := discovered["widgets/scale"]; r.Group != "autoscaling" || r.Version != "v1" || r.Kind != "Scale" || !r.Namespaced || !slices.Equal(slices.Sorted(slices.Values(r.Verbs)), subresourceVerbs) {
		t.Errorf("discovery widgets/scale: %+v", r)
	}

	s1 := widget("s1", 2)
	s1.Object["spec"].(map[string]any)["replicas"] = int64(1)
	s1.Object["status"] = map[string]any{"replicas": int64(7)}
	s1, err = widgets.Create(c, s1, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if status, ok := s1.Object["status"]; ok || s1.GetGeneration() != 1 {
		t.Errorf("create: status %v generation %d, want status dropped and generation 1", status, s1.GetGeneration())
	}
	wantStatus := map[string]any{"replicas": int64(1), "selector": "app=s1"}
	s1.Object["status"] = map[string]any{"replicas": int64(1), "selector": "app=s1"}
	s1.Object["spec"].(map[string]any)["size"] = int64(4)
	s1, err = widgets.UpdateStatus(c, s1, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if size, _, _ := unstructured.NestedInt64(s1.Object, "spec", "size"); size != 2 || !reflect.DeepEqual(s1.Object["status"], wantStatus) || s1.GetGeneration() != 1 {
		t.Errorf("status update: spec %v status %v generation %d, want spec.size 2, status %v, generation 1", s1.Object["spec"], s1.Object["status"], s1.GetGeneration(), wantStatus)
	}
	s1.Object["status"] = map[string]any{"replicas": int64(9)}
	s1.Object["spec"].(map[string]any)["size"] = int64(3)
	s1, err = widgets.Update(c, s1, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if size, _, _ := unstructured.NestedInt64(s1.Object, "spec", "size"); size != 3 || !reflect.DeepEqual(s1.Object["status"], wantStatus) || s1.GetGeneration() != 2 {
		t.Errorf("update: spec %v status %v generation %d, want spec.size 3, status %v, generation 2", s1.Object["spec"], s1.Object["status"], s1.GetGeneration(), wantStatus)
	}

	scaleObj, err := widgets.Get(c, "s1", metav1.GetOptions{}, "scale")
	if err != nil {
		t.Fatal(err)
	}
	if scale := scaleOf(t, scaleObj); scale.Name != "s1" || scale.Spec.Replicas != 1 || scale.Status.Replicas != 1 || scale.Status.Selector != "app=s1" {
		t.Errorf("GET scale: spec %+v status %+v, want replicas 1/1 selector app=s1", scale.Spec, scale.Status)
	}
	scaleObj.Object["spec"] = map[string]any{"replicas": int64(4)}
	scaleObj.Object["status"] = map[string]any{"replicas": int64(1), "selector": "changed"}
	scaleObj, err = widgets.Update(c, scaleObj, metav1.UpdateOptions{}, "scale")
	if err != nil {
		t.Fatal(err)
	}
	if scale := scaleOf(t, scaleObj); scale.Spec.Replicas != 4 || scale.Status.Selector != "app=s1" {
		t.Errorf("PUT scale: spec %+v status %+v, want spec.replicas 4 and status unchanged", scale.Spec, scale.Status)
	}
	s1, err = widgets.Get(c, "s1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if replicas, _, _ := unstructured.NestedInt64(s1.Object, "spec", "replicas"); replicas != 4 || !reflect.DeepEqual(s1.Object["status"], wantStatus) || s1.GetGeneration() != 3 {
		t.Errorf("after PUT scale: spec %v status %v generation %d, want spec.replicas 4, status %v, generation 3", s1.Object["spec"], s1.Object["status"], s1.GetGeneration(), wantStatus)
	}
	scaleObj.Object["spec"] = map[string]any{"replicas": int64(-1)}
	_, err = widgets.Update(c, scaleObj, metav1.UpdateOptions{}, "scale")
	wantInvalid(t, err, `Widget.test.k8flare.dev "s1" is invalid: .spec.replicas: Invalid value: -1: should be a non-negative integer`,
		metav1.StatusCause{Type: metav1.CauseTypeFieldValueInvalid, Message: "Invalid value: -1: should be a non-negative integer", Field: ".spec.replicas"})

	held := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "widgets-terminating", Finalizers: []string{"test.k8flare.dev/hold"}}}
	if _, err := cs.CoreV1().Namespaces().Create(c, held, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := cs.CoreV1().Namespaces().Delete(c, held.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	_, err = dyn.Resource(widgetGVR).Namespace(held.Name).Create(c, widget("late", 3), metav1.CreateOptions{})
	if !apierrors.IsForbidden(err) || !apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
		t.Errorf("create in a terminating namespace: want 403 with cause %s, got %v", corev1.NamespaceTerminatingCause, err)
	}
	_, err = dyn.Resource(widgetGVR).Namespace("widgets-missing").Create(c, widget("stray", 3), metav1.CreateOptions{})
	if !apierrors.IsNotFound(err) {
		t.Errorf("create in a missing namespace: want 404, got %v", err)
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

var widgetSelectorPath = ".status.selector"

func wantInvalid(t *testing.T, err error, message string, cause metav1.StatusCause) {
	t.Helper()
	var status apierrors.APIStatus
	if !errors.As(err, &status) {
		t.Errorf("want 422 Invalid %q, got %v", message, err)
		return
	}
	s := status.Status()
	if s.Code != http.StatusUnprocessableEntity || s.Reason != metav1.StatusReasonInvalid || s.Message != message {
		t.Errorf("want 422 Invalid %q, got %d %s %q", message, s.Code, s.Reason, s.Message)
	}
	if s.Details == nil || !slices.Equal(s.Details.Causes, []metav1.StatusCause{cause}) {
		t.Errorf("want cause %+v, got %+v", cause, s.Details)
	}
}

func scaleOf(t *testing.T, obj *unstructured.Unstructured) autoscalingv1.Scale {
	t.Helper()
	var scale autoscalingv1.Scale
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &scale); err != nil {
		t.Fatal(err)
	}
	if scale.APIVersion != "autoscaling/v1" || scale.Kind != "Scale" {
		t.Errorf("scale subresource returned %s %s, want autoscaling/v1 Scale", scale.APIVersion, scale.Kind)
	}
	return scale
}
