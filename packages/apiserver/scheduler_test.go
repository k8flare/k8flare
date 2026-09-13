//go:build !js

package apiserver_test

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

func TestScheduler(t *testing.T) {
	cs := startDev(t)
	c, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	createNode(t, cs, c, "s1")
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "unscheduled"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}}}
	if _, err := cs.CoreV1().Pods("default").Create(c, pod, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	waitBound(t, cs, c, "unscheduled", "s1")
}

func TestSchedulerWakesOnNode(t *testing.T) {
	cs := startDev(t)
	c, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "early"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}}}
	if _, err := cs.CoreV1().Pods("default").Create(c, pod, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Second)
	createNode(t, cs, c, "s2")
	waitBound(t, cs, c, "early", "s2")
}

func createNode(t *testing.T, cs *kubernetes.Clientset, c context.Context, name string) {
	t.Helper()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if _, err := cs.CoreV1().Nodes().Create(c, node, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	node.Status = corev1.NodeStatus{
		Capacity:    corev1.ResourceList{corev1.ResourcePods: resource.MustParse("110"), corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("4Gi")},
		Allocatable: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("110"), corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("4Gi")},
		Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
	}
	if _, err := cs.CoreV1().Nodes().UpdateStatus(c, node, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func waitBound(t *testing.T, cs *kubernetes.Clientset, c context.Context, pod, node string) {
	t.Helper()
	if err := wait.PollUntilContextTimeout(c, time.Second, 90*time.Second, true, func(ctx context.Context) (bool, error) {
		got, err := cs.CoreV1().Pods("default").Get(ctx, pod, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return got.Spec.NodeName == node, nil
	}); err != nil {
		events, _ := cs.CoreV1().Events("default").List(c, metav1.ListOptions{FieldSelector: "involvedObject.name=" + pod})
		for _, ev := range events.Items {
			t.Logf("event %s: %s", ev.Reason, ev.Message)
		}
		t.Fatalf("pod %s was not bound to %s: %v", pod, node, err)
	}
}
