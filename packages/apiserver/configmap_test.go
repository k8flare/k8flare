//go:build !js

package apiserver_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const notRootCA = "metadata.name!=kube-root-ca.crt,metadata.name!=cluster-info"

func TestConfigMapVerbs(t *testing.T) {
	cs := startDev(t)
	c := ctx(t)
	cms := cs.CoreV1().ConfigMaps("default")

	created, err := cms.Create(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Data: map[string]string{"k": "v"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ResourceVersion == "" || created.UID == "" {
		t.Fatalf("create returned no resourceVersion/uid: %+v", created.ObjectMeta)
	}
	if _, err := cms.Create(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "a"}}, metav1.CreateOptions{}); !apierrors.IsAlreadyExists(err) {
		t.Fatalf("second create: want AlreadyExists, got %v", err)
	}

	got, err := cms.Get(c, "a", metav1.GetOptions{})
	if err != nil || got.Data["k"] != "v" {
		t.Fatalf("get: %v %+v", err, got)
	}

	got.Data["k"] = "v2"
	updated, err := cms.Update(c, got, metav1.UpdateOptions{})
	if err != nil || updated.Data["k"] != "v2" || updated.ResourceVersion == got.ResourceVersion {
		t.Fatalf("update: %v %+v", err, updated)
	}
	got.ResourceVersion = created.ResourceVersion
	if _, err := cms.Update(c, got, metav1.UpdateOptions{}); !apierrors.IsConflict(err) {
		t.Fatalf("stale update: want Conflict, got %v", err)
	}

	patched, err := cms.Patch(c, "a", types.StrategicMergePatchType, []byte(`{"data":{"p":"1"}}`), metav1.PatchOptions{})
	if err != nil || patched.Data["p"] != "1" || patched.Data["k"] != "v2" {
		t.Fatalf("strategic merge patch: %v %+v", err, patched)
	}
	patched, err = cms.Patch(c, "a", types.MergePatchType, []byte(`{"data":{"m":"2"}}`), metav1.PatchOptions{})
	if err != nil || patched.Data["m"] != "2" {
		t.Fatalf("merge patch: %v %+v", err, patched)
	}
	patched, err = cms.Patch(c, "a", types.ApplyPatchType, []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"a"},"data":{"s":"3"}}`), metav1.PatchOptions{FieldManager: "test", Force: ptr(true)})
	if err != nil || patched.Data["s"] != "3" {
		t.Fatalf("server-side apply: %v %+v", err, patched)
	}

	for _, n := range []string{"b", "c", "d"} {
		if _, err := cms.Create(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: n, Labels: map[string]string{"group": "x"}}}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := cms.List(c, metav1.ListOptions{FieldSelector: notRootCA})
	if err != nil || len(list.Items) != 4 || list.ResourceVersion == "" {
		t.Fatalf("list: %v items=%d rv=%q", err, len(list.Items), list.ResourceVersion)
	}
	list, err = cms.List(c, metav1.ListOptions{LabelSelector: "group=x"})
	if err != nil || len(list.Items) != 3 {
		t.Fatalf("list by label: %v items=%d", err, len(list.Items))
	}
	list, err = cms.List(c, metav1.ListOptions{FieldSelector: "metadata.name=c"})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("list by field: %v items=%d", err, len(list.Items))
	}
	page1, err := cms.List(c, metav1.ListOptions{Limit: 3, FieldSelector: notRootCA})
	if err != nil || len(page1.Items) != 3 || page1.Continue == "" {
		t.Fatalf("page 1: %v items=%d continue=%q", err, len(page1.Items), page1.Continue)
	}
	page2, err := cms.List(c, metav1.ListOptions{Limit: 3, Continue: page1.Continue, FieldSelector: notRootCA})
	if err != nil || len(page2.Items) != 1 || page2.Continue != "" {
		t.Fatalf("page 2: %v items=%d continue=%q", err, len(page2.Items), page2.Continue)
	}
	if _, err := cs.CoreV1().Namespaces().Create(c, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.CoreV1().ConfigMaps("other").Create(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "a"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	all, err := cs.CoreV1().ConfigMaps("").List(c, metav1.ListOptions{FieldSelector: notRootCA})
	if err != nil || len(all.Items) != 5 {
		t.Fatalf("list all namespaces: %v items=%d", err, len(all.Items))
	}

	if err := cms.Delete(c, "a", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := cms.Get(c, "a", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("get after delete: want NotFound, got %v", err)
	}
	if err := cms.DeleteCollection(c, metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: "group=x"}); err != nil {
		t.Fatalf("deletecollection: %v", err)
	}
	list, err = cms.List(c, metav1.ListOptions{FieldSelector: notRootCA})
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("list after deletecollection: %v items=%d", err, len(list.Items))
	}
}

func ptr[T any](v T) *T { return &v }
