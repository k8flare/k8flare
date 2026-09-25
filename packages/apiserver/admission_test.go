//go:build !js

package apiserver_test

import (
	"context"
	"testing"
	"time"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
)

func TestAdmissionExtensions(t *testing.T) {
	url, cs := startDevURL(t)
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

	cfg := devConfig(url, devToken)
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
	if err := wait.PollUntilContextTimeout(c, 500*time.Millisecond, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		got, err := ext.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
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
		t.Fatalf("crd not established: %v", err)
	}
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
}
