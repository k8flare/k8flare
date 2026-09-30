package core

import (
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
