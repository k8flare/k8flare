//go:build !js

package apiserver_test

import (
	"context"
	"strings"
	"testing"
	"time"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

func TestAdmissionExtensions(t *testing.T) {
	url, cs := startDevURLWith(t, "test", "packages/apiserver/testdata/user-echo/wrangler.jsonc")
	c := ctx(t)
	sideEffects := admissionregv1.SideEffectClassNone
	fail := admissionregv1.Fail
	echo := "https://k8flare.com/worker/admission-echo"

	if _, err := cs.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(c, &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-validate"},
		Webhooks: []admissionregv1.ValidatingWebhook{{
			Name:                    "echo-validate.k8flare.dev",
			ClientConfig:            admissionregv1.WebhookClientConfig{URL: &echo},
			Rules:                   []admissionregv1.RuleWithOperations{{Operations: []admissionregv1.OperationType{admissionregv1.Create}, Rule: admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"configmaps"}}}},
			SideEffects:             &sideEffects,
			AdmissionReviewVersions: []string{"v1"},
			FailurePolicy:           &fail,
		}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(c, &admissionregv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-mutate"},
		Webhooks: []admissionregv1.MutatingWebhook{{
			Name:                    "echo-mutate.k8flare.dev",
			ClientConfig:            admissionregv1.WebhookClientConfig{URL: &echo},
			Rules:                   []admissionregv1.RuleWithOperations{{Operations: []admissionregv1.OperationType{admissionregv1.Create}, Rule: admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"secrets"}}}},
			SideEffects:             &sideEffects,
			AdmissionReviewVersions: []string{"v1"},
			FailurePolicy:           &fail,
		}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	if _, err := cs.CoreV1().ConfigMaps("default").Create(c, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "denied", Annotations: map[string]string{"k8flare.io/admit": "deny"}},
	}, metav1.CreateOptions{}); !apierrors.IsForbidden(err) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if _, err := cs.CoreV1().ConfigMaps("default").Create(c, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "allowed"},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("allowed configmap: %v", err)
	}

	secret, err := cs.CoreV1().Secrets("default").Create(c, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "mutated", Annotations: map[string]string{"k8flare.io/admit": "mutate"}},
		Type:       corev1.SecretTypeOpaque,
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("mutate secret: %v", err)
	}
	if secret.Annotations["k8flare.io/mutated"] != "true" {
		t.Fatalf("secret annotations: %+v", secret.Annotations)
	}

	if _, err := cs.AdmissionregistrationV1().ValidatingAdmissionPolicies().Create(c, &admissionregv1.ValidatingAdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "deny-label"},
		Spec: admissionregv1.ValidatingAdmissionPolicySpec{
			MatchConstraints: &admissionregv1.MatchResources{ResourceRules: []admissionregv1.NamedRuleWithOperations{{
				RuleWithOperations: admissionregv1.RuleWithOperations{
					Operations: []admissionregv1.OperationType{admissionregv1.Create},
					Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"limitranges"}},
				},
			}}},
			Validations: []admissionregv1.Validation{{Expression: `!(has(object.metadata.labels) && object.metadata.labels["k8flare.io/deny"] == "true")`}},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.AdmissionregistrationV1().ValidatingAdmissionPolicyBindings().Create(c, &admissionregv1.ValidatingAdmissionPolicyBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "deny-label"},
		Spec: admissionregv1.ValidatingAdmissionPolicyBindingSpec{
			PolicyName:        "deny-label",
			ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.CoreV1().LimitRanges("default").Create(c, &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: "blocked", Labels: map[string]string{"k8flare.io/deny": "true"}},
	}, metav1.CreateOptions{}); !apierrors.IsInvalid(err) {
		t.Fatalf("expected VAP deny, got %v", err)
	}

	cfg := &rest.Config{Host: url, BearerToken: devToken}
	ext := apiextensionsclient.NewForConfigOrDie(cfg)
	dyn := dynamic.NewForConfigOrDie(cfg)
	crd := widgetCRD()
	crd.Name = "gadgets.test.k8flare.dev"
	crd.Spec.Names.Plural = "gadgets"
	crd.Spec.Names.Singular = "gadget"
	crd.Spec.Names.Kind = "Gadget"
	crd.Spec.Names.ListKind = "GadgetList"
	crd.Annotations = map[string]string{"k8flare.io/controller": "controller-echo"}
	if _, err := ext.ApiextensionsV1().CustomResourceDefinitions().Create(c, crd, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	gvr := schema.GroupVersionResource{Group: "test.k8flare.dev", Version: "v1", Resource: "gadgets"}
	waitEstablished(t, c, ext, crd.Name)
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "test.k8flare.dev/v1", "kind": "Gadget",
		"metadata": map[string]any{"name": "one"},
		"spec":     map[string]any{"size": int64(2)},
	}}
	if _, err := dyn.Resource(gvr).Namespace("default").Create(c, obj, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := wait.PollUntilContextTimeout(c, 500*time.Millisecond, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		got, err := dyn.Resource(gvr).Namespace("default").Get(ctx, "one", metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return got.GetAnnotations()["k8flare.io/reconciled"] == "true", nil
	}); err != nil {
		t.Fatalf("controller-echo did not reconcile: %v", err)
	}

	userEcho := "https://k8flare.com/worker/user-echo"
	notAService := admissionregv1.WebhookClientConfig{Service: &admissionregv1.ServiceReference{Namespace: "default", Name: "not-a-service"}}
	for _, hook := range []struct {
		name        string
		annotations map[string]string
		client      admissionregv1.WebhookClientConfig
	}{
		{"user-echo-url", nil, admissionregv1.WebhookClientConfig{URL: &userEcho}},
		{"user-echo-annotation", map[string]string{"k8flare.com/worker": "user-echo"}, notAService},
	} {
		selector := map[string]string{"k8flare.io/user-echo": hook.name}
		if _, err := cs.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(c, &admissionregv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: hook.name, Annotations: hook.annotations},
			Webhooks: []admissionregv1.ValidatingWebhook{{
				Name:                    hook.name + ".k8flare.dev",
				ClientConfig:            hook.client,
				Rules:                   []admissionregv1.RuleWithOperations{{Operations: []admissionregv1.OperationType{admissionregv1.Create}, Rule: admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"configmaps"}}}},
				ObjectSelector:          &metav1.LabelSelector{MatchLabels: selector},
				SideEffects:             &sideEffects,
				AdmissionReviewVersions: []string{"v1"},
				FailurePolicy:           &fail,
			}},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		_, err := cs.CoreV1().ConfigMaps("default").Create(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: hook.name, Labels: selector}}, metav1.CreateOptions{})
		if !apierrors.IsForbidden(err) || !strings.Contains(err.Error(), "denied by user-echo") {
			t.Fatalf("%s: expected user-echo to deny, got %v", hook.name, err)
		}
	}

	preserve := true
	anyObject := &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object", XPreserveUnknownFields: &preserve}}
	gizmos := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "gizmos.test.k8flare.dev", Annotations: map[string]string{"k8flare.io/controller": "user-echo", "k8flare.com/worker": "user-echo"}},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "test.k8flare.dev",
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: "gizmos", Singular: "gizmo", Kind: "Gizmo", ListKind: "GizmoList"},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true, Schema: anyObject},
				{Name: "v2", Served: true, Schema: anyObject},
			},
			Conversion: &apiextensionsv1.CustomResourceConversion{
				Strategy: apiextensionsv1.WebhookConverter,
				Webhook: &apiextensionsv1.WebhookConversion{
					ClientConfig:             &apiextensionsv1.WebhookClientConfig{Service: &apiextensionsv1.ServiceReference{Namespace: "default", Name: "not-a-service"}},
					ConversionReviewVersions: []string{"v1"},
				},
			},
		},
	}
	if _, err := ext.ApiextensionsV1().CustomResourceDefinitions().Create(c, gizmos, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	waitEstablished(t, c, ext, gizmos.Name)
	gizmosV1 := schema.GroupVersionResource{Group: "test.k8flare.dev", Version: "v1", Resource: "gizmos"}
	if _, err := dyn.Resource(gizmosV1).Namespace("default").Create(c, &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "test.k8flare.dev/v1", "kind": "Gizmo",
		"metadata": map[string]any{"name": "two"},
	}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := wait.PollUntilContextTimeout(c, 500*time.Millisecond, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		got, err := dyn.Resource(gizmosV1).Namespace("default").Get(ctx, "two", metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return got.GetAnnotations()["k8flare.io/reconciled-by"] == "user-echo", nil
	}); err != nil {
		t.Fatalf("user-echo did not reconcile: %v", err)
	}
	converted, err := dyn.Resource(schema.GroupVersionResource{Group: "test.k8flare.dev", Version: "v2", Resource: "gizmos"}).Namespace("default").Get(c, "two", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get through user-echo conversion: %v", err)
	}
	if converted.GetAPIVersion() != "test.k8flare.dev/v2" || converted.GetAnnotations()["k8flare.io/converted-by"] != "user-echo" {
		t.Fatalf("user-echo did not convert: %s %v", converted.GetAPIVersion(), converted.GetAnnotations())
	}

	apiServices := schema.GroupVersionResource{Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices"}
	if _, err := dyn.Resource(apiServices).Create(c, &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{"name": "v1.user-echo.k8flare.dev", "annotations": map[string]any{"k8flare.com/worker": "user-echo"}},
		"spec":     map[string]any{"group": "user-echo.k8flare.dev", "version": "v1", "groupPriorityMinimum": int64(1000), "versionPriority": int64(15)},
	}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	var served []byte
	if err := wait.PollUntilContextTimeout(c, 500*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		served, err = cs.Discovery().RESTClient().Get().AbsPath("/apis/user-echo.k8flare.dev/v1").DoRaw(ctx)
		return err == nil && strings.Contains(string(served), `"user-echoes"`), nil
	}); err != nil {
		t.Fatalf("user-echo did not serve its API: %v %s", err, served)
	}
}

func waitEstablished(t *testing.T, c context.Context, ext *apiextensionsclient.Clientset, name string) {
	t.Helper()
	if err := wait.PollUntilContextTimeout(c, 500*time.Millisecond, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		got, err := ext.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		for _, cond := range got.Status.Conditions {
			if cond.Type == "Established" && cond.Status == "True" {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatalf("crd %s not established: %v", name, err)
	}
}
