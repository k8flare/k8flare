package apiserver

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TestResolveVKubeProxyTarget is an end-to-end check (no wrangler dev --
// fakeKV/newTestStorage, same shape as endpoints_test.go) that the
// ClusterIP -> Service -> EndpointSlice resolution nodes/podproxy.ts's
// handleVKubeProxy now delegates here (vkubeproxy.go) matches what that
// TS code used to compute itself via two separate HTTP round trips.
func TestResolveVKubeProxyTarget(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()
	ns := "default"

	mustCreatePod(t, storage, ns, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-1", Labels: map[string]string{"app": "web"}},
		Spec: corev1.PodSpec{
			NodeName:   "cf-web-1-abcd1234",
			Containers: []corev1.Container{{Name: "c", Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP}}}},
		},
		Status: corev1.PodStatus{
			PodIP:      "10.42.0.5",
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	})

	mustCreateService(t, storage, ns, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web"},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.43.0.10",
			Selector:  map[string]string{"app": "web"},
			Ports:     []corev1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("http")}},
		},
	})

	if err := ReconcileNamespaceEndpoints(ctx, storage, ns); err != nil {
		t.Fatalf("ReconcileNamespaceEndpoints: %v", err)
	}

	podUID, containerPort, err := ResolveVKubeProxyTarget(ctx, storage, "10.43.0.10", 80)
	if err != nil {
		t.Fatalf("ResolveVKubeProxyTarget: %v", err)
	}
	if containerPort != 8080 {
		t.Errorf("containerPort = %d, want 8080", containerPort)
	}
	if podUID == "" {
		t.Error("podUID is empty")
	}

	t.Run("UnknownClusterIP", func(t *testing.T) {
		if _, _, err := ResolveVKubeProxyTarget(ctx, storage, "10.43.0.99", 80); err == nil {
			t.Error("expected an error for an unknown ClusterIP, got nil")
		}
	})

	t.Run("UnknownPort", func(t *testing.T) {
		if _, _, err := ResolveVKubeProxyTarget(ctx, storage, "10.43.0.10", 9999); err == nil {
			t.Error("expected an error for a Service port that doesn't exist, got nil")
		}
	})

	t.Run("NoReadyEndpoint", func(t *testing.T) {
		// A Pod that's Ready=false should be excluded (endpoints.go
		// still publishes it in the not-ready address list, not the
		// ready one).
		kv2 := newFakeKV()
		storage2 := newTestStorage(kv2)
		mustCreatePod(t, storage2, ns, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "web-2", Labels: map[string]string{"app": "web"}},
			Spec: corev1.PodSpec{
				NodeName:   "cf-web-2-abcd1234",
				Containers: []corev1.Container{{Name: "c", Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP}}}},
			},
			Status: corev1.PodStatus{
				PodIP:      "10.42.0.6",
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}},
			},
		})
		mustCreateService(t, storage2, ns, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web"},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.43.0.10",
				Selector:  map[string]string{"app": "web"},
				Ports:     []corev1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("http")}},
			},
		})
		if err := ReconcileNamespaceEndpoints(ctx, storage2, ns); err != nil {
			t.Fatalf("ReconcileNamespaceEndpoints: %v", err)
		}
		if _, _, err := ResolveVKubeProxyTarget(ctx, storage2, "10.43.0.10", 80); err == nil {
			t.Error("expected an error when no endpoint is ready, got nil")
		}
	})
}
