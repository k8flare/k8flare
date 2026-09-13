//go:build !js

package apiserver_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTables(t *testing.T) {
	cs := startDev(t)
	c := ctx(t)
	if _, err := cs.CoreV1().Nodes().Create(c, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "t1"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.CoreV1().Pods("default").Create(c, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1"}, Spec: corev1.PodSpec{NodeName: "t1", Containers: []corev1.Container{{Name: "c", Image: "img"}}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	table := func(path string) *metav1.Table {
		t.Helper()
		var out metav1.Table
		err := cs.CoreV1().RESTClient().Get().AbsPath(path).
			SetHeader("Accept", "application/json;as=Table;v=v1;g=meta.k8s.io,application/json").
			Do(c).Into(&out)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return &out
	}
	names := func(tb *metav1.Table) []string {
		var n []string
		for _, col := range tb.ColumnDefinitions {
			n = append(n, col.Name)
		}
		return n
	}
	pods := table("/api/v1/namespaces/default/pods")
	if got := names(pods); len(pods.Rows) != 1 || len(got) < 5 || got[0] != "Name" || got[1] != "Ready" || got[2] != "Status" {
		t.Fatalf("pods table: rows=%d columns=%v", len(pods.Rows), got)
	}
	if pods.Rows[0].Object.Object == nil {
		if pods.Rows[0].Object.Raw == nil {
			t.Fatalf("pods table: row without object metadata")
		}
	}
	nodes := table("/api/v1/nodes")
	if got := names(nodes); len(got) < 4 || got[0] != "Name" || got[1] != "Status" || got[2] != "Roles" {
		t.Fatalf("nodes table: %v", got)
	}
	leases := table("/apis/coordination.k8s.io/v1/namespaces/kube-node-lease/leases")
	if got := names(leases); len(got) < 2 || got[1] != "Holder" {
		t.Fatalf("leases table: %v", got)
	}
}
