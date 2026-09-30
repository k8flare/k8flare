package core

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

func TestNodeNameOnlySpecChangeDoesNotBumpGeneration(t *testing.T) {
	old := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Generation: 1}, Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyAlways}}
	next := old.DeepCopy()
	next.Spec.NodeName = "n1"
	podUpdateStrategy{RESTUpdateStrategy: nopUpdateStrategy{}}.PrepareForUpdate(context.Background(), next, old)
	if next.Generation != 1 {
		t.Fatalf("generation=%d", next.Generation)
	}
}

func TestSpecChangeOtherThanNodeNameBumpsGeneration(t *testing.T) {
	old := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Generation: 1}, Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyAlways}}
	next := old.DeepCopy()
	next.Spec.RestartPolicy = corev1.RestartPolicyNever
	podUpdateStrategy{RESTUpdateStrategy: bumpOnSpec{}}.PrepareForUpdate(context.Background(), next, old)
	if next.Generation != 2 {
		t.Fatalf("generation=%d", next.Generation)
	}
}

type nopUpdateStrategy struct{ rest.RESTUpdateStrategy }

func (nopUpdateStrategy) PrepareForUpdate(context.Context, runtime.Object, runtime.Object) {}

type bumpOnSpec struct{ rest.RESTUpdateStrategy }

func (bumpOnSpec) PrepareForUpdate(_ context.Context, obj, old runtime.Object) {
	obj.(*corev1.Pod).Generation = old.(*corev1.Pod).Generation + 1
}
