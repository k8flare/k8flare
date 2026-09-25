package registry

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPrepareForCreateClearsStatusExceptNode(t *testing.T) {
	svc := &corev1.Service{Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "1.2.3.4"}}}}}
	strategy{}.PrepareForCreate(context.Background(), svc)
	if len(svc.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("service status=%+v", svc.Status)
	}
	node := &corev1.Node{Status: corev1.NodeStatus{Phase: corev1.NodeRunning}}
	strategy{}.PrepareForCreate(context.Background(), node)
	if node.Status.Phase != corev1.NodeRunning {
		t.Fatalf("node phase=%s", node.Status.Phase)
	}
}

func TestPrepareForUpdateKeepsStatus(t *testing.T) {
	old := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Generation: 1}, Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "1.2.3.4"}}}}}
	next := old.DeepCopy()
	next.Status = corev1.ServiceStatus{}
	strategy{}.PrepareForUpdate(context.Background(), next, old)
	if len(next.Status.LoadBalancer.Ingress) != 1 || next.Status.LoadBalancer.Ingress[0].IP != "1.2.3.4" {
		t.Fatalf("status=%+v", next.Status)
	}
}

func TestStatusOnlyPreservesGeneration(t *testing.T) {
	old := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Generation: 1},
		Spec:       corev1.PodSpec{RestartPolicy: corev1.RestartPolicyAlways},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}
	next := old.DeepCopy()
	next.Generation = 99
	next.Spec.RestartPolicy = corev1.RestartPolicyNever
	next.Status.Phase = corev1.PodRunning
	statusOnlyStrategy{}.PrepareForUpdate(context.Background(), next, old)
	if next.Generation != 1 {
		t.Fatalf("generation=%d want 1", next.Generation)
	}
	if next.Spec.RestartPolicy != corev1.RestartPolicyAlways {
		t.Fatalf("spec leaked: %s", next.Spec.RestartPolicy)
	}
	if next.Status.Phase != corev1.PodRunning {
		t.Fatalf("status=%s", next.Status.Phase)
	}
}
