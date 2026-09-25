package admissionregistration

import (
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDefaultsValidatingWebhook(t *testing.T) {
	cfg := &admissionregistrationv1.ValidatingWebhookConfiguration{Webhooks: []admissionregistrationv1.ValidatingWebhook{{
		ClientConfig: admissionregistrationv1.WebhookClientConfig{Service: &admissionregistrationv1.ServiceReference{Name: "hook", Namespace: "default"}},
	}}}
	scheme.Scheme.Default(cfg)
	hook := cfg.Webhooks[0]
	if hook.FailurePolicy == nil || *hook.FailurePolicy != admissionregistrationv1.Fail {
		t.Fatalf("failurePolicy=%v", hook.FailurePolicy)
	}
	if hook.TimeoutSeconds == nil || *hook.TimeoutSeconds != 10 {
		t.Fatalf("timeout=%v", hook.TimeoutSeconds)
	}
	if hook.ClientConfig.Service.Port == nil || *hook.ClientConfig.Service.Port != 443 {
		t.Fatalf("port=%v", hook.ClientConfig.Service.Port)
	}
}
