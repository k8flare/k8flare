package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func componentStatusClient(datastoreUp bool, health string) *kine.Client {
	return &kine.Client{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !datastoreUp {
			return nil, fmt.Errorf("cluster unreachable")
		}
		body := `{"revision":7}`
		if r.URL.Path == "/health" {
			body = health
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}}
}

func conditionOf(t *testing.T, r componentStatusREST, name string) corev1.ComponentCondition {
	t.Helper()
	obj, err := r.Get(context.Background(), name, &metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cs := obj.(*corev1.ComponentStatus)
	if cs.Name != name || len(cs.Conditions) != 1 || cs.Conditions[0].Type != corev1.ComponentHealthy {
		t.Fatalf("got %#v", cs)
	}
	return cs.Conditions[0]
}

func TestComponentStatusGetList(t *testing.T) {
	r := newComponentStatusREST(componentStatusClient(true, `{"passes":[]}`)).(componentStatusREST)
	c := conditionOf(t, r, "etcd-0")
	if c.Status != corev1.ConditionTrue || c.Message != "ok" || c.Error != "" {
		t.Fatalf("got %#v", c)
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
	for _, item := range items {
		if item.Conditions[0].Status != corev1.ConditionTrue {
			t.Fatalf("%s: %#v", item.Name, item.Conditions[0])
		}
	}
}

func TestComponentStatusReflectsDatastore(t *testing.T) {
	r := newComponentStatusREST(componentStatusClient(false, "")).(componentStatusREST)
	for _, name := range []string{"etcd-0", "scheduler", "controller-manager"} {
		c := conditionOf(t, r, name)
		if c.Status != corev1.ConditionFalse || !strings.Contains(c.Error, "cluster unreachable") || c.Message != "" {
			t.Fatalf("%s: %#v", name, c)
		}
	}
}

func TestComponentStatusReflectsPasses(t *testing.T) {
	schedulerStuck := `{"outbox":0,"flushFailingMs":0,"passes":[{"target":"scheduler","pendingMs":900000},{"target":"workloads","pendingMs":1000}]}`
	r := newComponentStatusREST(componentStatusClient(true, schedulerStuck)).(componentStatusREST)
	if c := conditionOf(t, r, "scheduler"); c.Status != corev1.ConditionFalse || !strings.Contains(c.Error, "scheduler pending") {
		t.Fatalf("scheduler: %#v", c)
	}
	for _, name := range []string{"etcd-0", "controller-manager"} {
		if c := conditionOf(t, r, name); c.Status != corev1.ConditionTrue {
			t.Fatalf("%s: %#v", name, c)
		}
	}

	managerStuck := `{"outbox":3,"flushFailingMs":900000,"passes":[{"target":"scheduler","pendingMs":0},{"target":"gc","pendingMs":900000}]}`
	r = newComponentStatusREST(componentStatusClient(true, managerStuck)).(componentStatusREST)
	if c := conditionOf(t, r, "controller-manager"); c.Status != corev1.ConditionFalse || !strings.Contains(c.Error, "outbox flush failing") {
		t.Fatalf("controller-manager: %#v", c)
	}
	for _, name := range []string{"etcd-0", "scheduler"} {
		if c := conditionOf(t, r, name); c.Status != corev1.ConditionTrue {
			t.Fatalf("%s: %#v", name, c)
		}
	}
}
