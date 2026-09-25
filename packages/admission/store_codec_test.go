package admission

import (
	"encoding/json"
	"testing"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestLegacyCodecJSONRoundTrip(t *testing.T) {
	ignore := admissionregv1.Ignore
	vwc := &admissionregv1.ValidatingWebhookConfiguration{
		TypeMeta:   metav1.TypeMeta{APIVersion: "admissionregistration.k8s.io/v1", Kind: "ValidatingWebhookConfiguration"},
		ObjectMeta: metav1.ObjectMeta{Name: "hook", Labels: map[string]string{"k": "v"}},
		Webhooks: []admissionregv1.ValidatingWebhook{{
			Name: "ready.k8s.io",
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"marker": "true"}},
			ObjectSelector:    &metav1.LabelSelector{MatchLabels: map[string]string{"hook": "true"}},
			FailurePolicy:     &ignore,
			ClientConfig: admissionregv1.WebhookClientConfig{
				Service: &admissionregv1.ServiceReference{Name: "e2e-test-webhook", Namespace: "webhook", Path: strPtr("/always-deny")},
			},
			Rules: []admissionregv1.RuleWithOperations{{
				Operations: []admissionregv1.OperationType{admissionregv1.Create},
				Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"configmaps"}},
			}},
		}},
	}
	ns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "markers", Labels: map[string]string{"marker": "true"}},
	}

	vwcData, err := runtime.Encode(scheme.Codecs.LegacyCodec(admissionregv1.SchemeGroupVersion), vwc)
	if err != nil {
		t.Fatal(err)
	}
	nsData, err := runtime.Encode(scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), ns)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("vwc head %q", clip(vwcData, 40))
	t.Logf("ns head %q", clip(nsData, 40))

	var gotVWC admissionregv1.ValidatingWebhookConfiguration
	if err := json.Unmarshal(vwcData, &gotVWC); err != nil {
		t.Fatalf("vwc json: %v", err)
	}
	if len(gotVWC.Webhooks) != 1 || gotVWC.Webhooks[0].NamespaceSelector == nil || gotVWC.Webhooks[0].NamespaceSelector.MatchLabels["marker"] != "true" {
		t.Fatalf("vwc decoded: %+v", gotVWC)
	}
	var gotNS corev1.Namespace
	if err := json.Unmarshal(nsData, &gotNS); err != nil {
		t.Fatalf("ns json: %v", err)
	}
	if gotNS.Labels["marker"] != "true" {
		t.Fatalf("ns labels: %+v", gotNS.Labels)
	}
}

func strPtr(s string) *string { return &s }

func clip(b []byte, n int) string {
	if len(b) < n {
		return string(b)
	}
	return string(b[:n])
}
