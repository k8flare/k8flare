package core

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComponentStatusGetList(t *testing.T) {
	r := newComponentStatusREST().(componentStatusREST)
	obj, err := r.Get(context.Background(), "etcd-0", &metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cs := obj.(*corev1.ComponentStatus)
	if cs.Name != "etcd-0" || cs.Conditions[0].Status != corev1.ConditionTrue {
		t.Fatalf("got %#v", cs)
	}
	if _, err := r.Get(context.Background(), "missing", &metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
	listed, err := r.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	items := listed.(*corev1.ComponentStatusList).Items
	if len(items) != 3 {
		t.Fatalf("len %d", len(items))
	}
	if items[0].Name != "controller-manager" || items[1].Name != "etcd-0" || items[2].Name != "scheduler" {
		t.Fatalf("order %#v", items)
	}
}
