//go:build !js

package apiserver_test

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

func TestWatch(t *testing.T) {
	cs := startDev(t)
	c := ctx(t)
	cms := cs.CoreV1().ConfigMaps("default")
	if _, err := cms.Create(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "before"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	list, err := cms.List(c, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	w, err := cms.Watch(c, metav1.ListOptions{ResourceVersion: list.ResourceVersion})
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	defer w.Stop()
	expect := func(want watch.EventType, name string) {
		t.Helper()
		select {
		case ev, ok := <-w.ResultChan():
			if !ok {
				t.Fatalf("watch closed while waiting for %s %s", want, name)
			}
			cm, _ := ev.Object.(*corev1.ConfigMap)
			if ev.Type != want || cm == nil || cm.Name != name {
				t.Fatalf("got %s %v, want %s %s", ev.Type, ev.Object, want, name)
			}
			if cm.ResourceVersion == "" {
				t.Fatalf("event without resourceVersion: %+v", cm.ObjectMeta)
			}
		case <-time.After(20 * time.Second):
			t.Fatalf("timed out waiting for %s %s", want, name)
		}
	}
	created, err := cms.Create(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Data: map[string]string{"k": "1"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	expect(watch.Added, "a")
	created.Data["k"] = "2"
	if _, err := cms.Update(c, created, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	expect(watch.Modified, "a")
	if err := cms.Delete(c, "a", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	expect(watch.Deleted, "a")

	initial, err := cms.Watch(c, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("watch from zero: %v", err)
	}
	defer initial.Stop()
	select {
	case ev := <-initial.ResultChan():
		if cm, _ := ev.Object.(*corev1.ConfigMap); ev.Type != watch.Added || cm == nil || cm.Name != "before" {
			t.Fatalf("initial watch: got %s %v", ev.Type, ev.Object)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("initial watch sent nothing")
	}

	labeled, err := cms.Watch(c, metav1.ListOptions{ResourceVersion: list.ResourceVersion, LabelSelector: "pick=me"})
	if err != nil {
		t.Fatal(err)
	}
	defer labeled.Stop()
	if _, err := cms.Create(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "unlabeled"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := cms.Create(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "labeled", Labels: map[string]string{"pick": "me"}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-labeled.ResultChan():
		if cm, _ := ev.Object.(*corev1.ConfigMap); ev.Type != watch.Added || cm == nil || cm.Name != "labeled" {
			t.Fatalf("labeled watch: got %s %v", ev.Type, ev.Object)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("labeled watch sent nothing")
	}
}
