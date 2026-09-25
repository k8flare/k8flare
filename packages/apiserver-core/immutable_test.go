package core

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestSharedSchemeDoesNotRecognizeInternalNamespace(t *testing.T) {
	internal := schema.GroupVersion{Group: "", Version: runtime.APIVersionInternal}
	if scheme.Scheme.Recognizes(internal.WithKind("Namespace")) {
		t.Fatal("shared scheme recognizes internal Namespace")
	}
}

func TestEmptyConfigMapKeyRejected(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"}, Data: map[string]string{"": "x"}}
	if errs := validateCreate(cm); len(errs) == 0 {
		t.Fatal("expected empty data key rejected")
	}
}

func TestSecretStringDataMergesIntoData(t *testing.T) {
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s"},
		Data:       map[string][]byte{"keep": []byte("old")},
		StringData: map[string]string{"keep": "new", "extra": "x"},
	}
	applySecretStringData(sec)
	if string(sec.Data["keep"]) != "new" || string(sec.Data["extra"]) != "x" {
		t.Fatalf("data = %v", sec.Data)
	}
	if sec.StringData != nil {
		t.Fatalf("stringData = %v", sec.StringData)
	}
}

func TestEmptySecretKeyRejected(t *testing.T) {
	sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"}, StringData: map[string]string{"": "x"}}
	applySecretStringData(sec)
	if errs := validateCreate(sec); len(errs) == 0 {
		t.Fatal("expected empty stringData key rejected")
	}
}

func TestImmutableConfigMapRejectsDataAndFlip(t *testing.T) {
	trueVal := true
	falseVal := false
	old := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default", ResourceVersion: "1", UID: "uid"},
		Immutable:  &trueVal,
		Data:       map[string]string{"k": "v"},
	}
	neu := old.DeepCopy()
	neu.Data["k2"] = "v2"
	if errs := validateUpdate(neu, old); len(errs) == 0 {
		t.Fatal("expected data change rejected")
	}
	got := validateUpdate(neu, old).ToAggregate().Error()
	if !strings.Contains(got, "field is immutable when `immutable` is set") {
		t.Fatalf("got %s", got)
	}
	neu = old.DeepCopy()
	neu.Immutable = &falseVal
	if errs := validateUpdate(neu, old); len(errs) == 0 {
		t.Fatal("expected immutable flip rejected")
	}
	neu = old.DeepCopy()
	neu.Labels = map[string]string{"a": "b"}
	if errs := validateUpdate(neu, old); len(errs) != 0 {
		t.Fatalf("metadata: %v", errs)
	}
}

func TestImmutableSecretRejectsDataAndFlip(t *testing.T) {
	trueVal := true
	falseVal := false
	old := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default", ResourceVersion: "1", UID: "uid"},
		Type:       corev1.SecretTypeOpaque,
		Immutable:  &trueVal,
		Data:       map[string][]byte{"k": []byte("v")},
	}
	neu := old.DeepCopy()
	neu.Data["k2"] = []byte("v2")
	if errs := validateUpdate(neu, old); len(errs) == 0 {
		t.Fatal("expected data change rejected")
	}
	neu = old.DeepCopy()
	neu.Immutable = &falseVal
	if errs := validateUpdate(neu, old); len(errs) == 0 {
		t.Fatal("expected immutable flip rejected")
	}
	neu = old.DeepCopy()
	neu.Type = corev1.SecretTypeBasicAuth
	if errs := validateUpdate(neu, old); len(errs) == 0 {
		t.Fatal("expected type change rejected")
	}
}

func TestMutableConfigMapAllowsDataChange(t *testing.T) {
	old := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default", ResourceVersion: "1", UID: "uid"},
		Data:       map[string]string{"k": "v"},
	}
	neu := old.DeepCopy()
	neu.Data["k2"] = "v2"
	trueVal := true
	neu.Immutable = &trueVal
	if errs := validateUpdate(neu, old); len(errs) != 0 {
		t.Fatalf("mutable: %v", errs)
	}
}
