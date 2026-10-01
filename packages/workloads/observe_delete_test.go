package workloads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
)

func deletingAPIServer(t *testing.T, answer func(pod *corev1.Pod)) kubernetes.Interface {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/namespaces/default/pods/web-0" {
			http.NotFound(w, r)
			return
		}
		pod := &corev1.Pod{
			TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "web-0", Namespace: "default", ResourceVersion: "2"},
			Spec:       corev1.PodSpec{NodeName: "n1"},
		}
		answer(pod)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pod)
	}))
	t.Cleanup(srv.Close)
	client, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestGracefulPodDeleteKeepsTheTerminatingPodInTheSnapshot(t *testing.T) {
	client := deletingAPIServer(t, func(pod *corev1.Pod) {
		pod.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(30 * time.Second)}
		pod.DeletionGracePeriodSeconds = ptr.To[int64](30)
	})
	pods := newSnapshotInformer(&corev1.Pod{})
	pods.fill([]runtime.Object{&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-0", Namespace: "default", ResourceVersion: "1"}, Spec: corev1.PodSpec{NodeName: "n1"}}})
	var updated, deleted *corev1.Pod
	if _, err := pods.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(_, obj interface{}) { updated = obj.(*corev1.Pod) },
		DeleteFunc: func(obj interface{}) { deleted = obj.(*corev1.Pod) },
	}); err != nil {
		t.Fatal(err)
	}
	observed := observeWrites(client, []loadedSource{{informer: pods}})
	if err := observed.CoreV1().Pods("default").Delete(context.Background(), "web-0", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if deleted != nil {
		t.Fatal("a graceful delete removed the pod from the snapshot")
	}
	stored, exists, _ := pods.GetIndexer().GetByKey("default/web-0")
	if !exists || stored.(*corev1.Pod).DeletionTimestamp == nil {
		t.Fatalf("snapshot exists=%v pod=%#v", exists, stored)
	}
	if updated == nil || updated.DeletionTimestamp == nil {
		t.Fatalf("handlers saw %#v", updated)
	}
}

func TestImmediatePodDeleteRemovesThePodFromTheSnapshot(t *testing.T) {
	client := deletingAPIServer(t, func(pod *corev1.Pod) {
		pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
		pod.DeletionGracePeriodSeconds = ptr.To[int64](0)
	})
	pods := newSnapshotInformer(&corev1.Pod{})
	pods.fill([]runtime.Object{&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-0", Namespace: "default", ResourceVersion: "1"}}})
	observed := observeWrites(client, []loadedSource{{informer: pods}})
	if err := observed.CoreV1().Pods("default").Delete(context.Background(), "web-0", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, exists, _ := pods.GetIndexer().GetByKey("default/web-0"); exists {
		t.Fatal("an immediate delete left the pod in the snapshot")
	}
}
