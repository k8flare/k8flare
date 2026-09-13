//go:build !js

package apiserver_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodePodCIDRAndPodLifecycle(t *testing.T) {
	cs := startDev(t)
	c := ctx(t)

	n1, err := cs.CoreV1().Nodes().Create(c, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}, metav1.CreateOptions{})
	if err != nil || n1.Spec.PodCIDR != "10.42.0.0/24" || len(n1.Spec.PodCIDRs) != 1 {
		t.Fatalf("first node: %v %+v", err, n1.Spec)
	}
	n2, err := cs.CoreV1().Nodes().Create(c, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2"}}, metav1.CreateOptions{})
	if err != nil || n2.Spec.PodCIDR != "10.42.1.0/24" {
		t.Fatalf("second node: %v %+v", err, n2.Spec)
	}

	pods := cs.CoreV1().Pods("default")
	scheduled, err := pods.Create(c, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "scheduled"},
		Spec:       corev1.PodSpec{NodeName: "n1", Containers: []corev1.Container{{Name: "c", Image: "img"}}},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if scheduled.Spec.EnableServiceLinks == nil || scheduled.Spec.RestartPolicy != corev1.RestartPolicyAlways || scheduled.Spec.Containers[0].ImagePullPolicy == "" {
		t.Fatalf("pod was not defaulted: %+v", scheduled.Spec)
	}
	scheduled.Status.Phase = corev1.PodRunning
	scheduled.Spec.Containers[0].Image = "changed-through-status"
	updated, err := pods.UpdateStatus(c, scheduled, metav1.UpdateOptions{})
	if err != nil || updated.Status.Phase != corev1.PodRunning || updated.Spec.Containers[0].Image != "img" {
		t.Fatalf("status update: %v phase=%s image=%s", err, updated.Status.Phase, updated.Spec.Containers[0].Image)
	}

	if err := pods.Delete(c, "scheduled", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	pending, err := pods.Get(c, "scheduled", metav1.GetOptions{})
	if err != nil || pending.DeletionTimestamp == nil || pending.DeletionGracePeriodSeconds == nil || *pending.DeletionGracePeriodSeconds != 30 {
		t.Fatalf("graceful delete should only mark a scheduled pod: %v %+v", err, pending.ObjectMeta)
	}
	if err := pods.Delete(c, "scheduled", metav1.DeleteOptions{GracePeriodSeconds: ptr(int64(0))}); err != nil {
		t.Fatal(err)
	}
	if _, err := pods.Get(c, "scheduled", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("final delete: want NotFound, got %v", err)
	}

	if _, err := pods.Create(c, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "unscheduled"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := pods.Delete(c, "unscheduled", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := pods.Get(c, "unscheduled", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("an unscheduled pod should go away at once, got %v", err)
	}
	list, err := pods.List(c, metav1.ListOptions{FieldSelector: "spec.nodeName=n1"})
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("list by nodeName: %v %d", err, len(list.Items))
	}
}
