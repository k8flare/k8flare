package core

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNamespaceCanonicalizeSetsNameLabel(t *testing.T) {
	created := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app", Labels: map[string]string{corev1.LabelMetadataName: "other"}}}
	namespaceCreateStrategy{}.Canonicalize(created)
	if created.Labels[corev1.LabelMetadataName] != "app" {
		t.Fatalf("create label=%q", created.Labels[corev1.LabelMetadataName])
	}
	updated := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "bare"}}
	namespaceUpdateStrategy{}.Canonicalize(updated)
	if updated.Labels[corev1.LabelMetadataName] != "bare" {
		t.Fatalf("update label=%q", updated.Labels[corev1.LabelMetadataName])
	}
}
