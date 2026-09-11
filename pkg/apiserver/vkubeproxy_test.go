package apiserver

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func podsResourceStore(storage *Storage) *ResourceStore {
	return NewResourceStore(storage, corev1.SchemeGroupVersion, "pods", "pod", true,
		func() runtime.Object { return &corev1.Pod{} },
		func() runtime.Object { return &corev1.PodList{} },
	)
}

func mustCreatePod(t *testing.T, storage *Storage, ns string, pod *corev1.Pod) *corev1.Pod {
	t.Helper()
	obj, err := podsResourceStore(storage).Create(context.Background(), ns, pod, nil)
	if err != nil {
		t.Fatalf("create pod %s: %v", pod.Name, err)
	}
	return obj.(*corev1.Pod)
}

func mustCreateService(t *testing.T, storage *Storage, ns string, svc *corev1.Service) *corev1.Service {
	t.Helper()
	obj, err := servicesResourceStore(storage).Create(context.Background(), ns, svc, nil)
	if err != nil {
		t.Fatalf("create service %s: %v", svc.Name, err)
	}
	return obj.(*corev1.Service)
}

// mustCreateEndpointSlice stores the EndpointSlice the real
// endpointslice controller (kcm dynamic worker,
// pkg/controllers/controllermanager.go) would publish for svc's ready
// backing pod -- the input ResolveVKubeProxyTarget reads. Written
// directly here because the apiserver test lane runs with the
// controllers disabled (KCM_DISABLED=1) and this test is about the
// resolution half only; the real controllers' own output is asserted
// end to end in kcmdw_test.go.
func mustCreateEndpointSlice(t *testing.T, storage *Storage, ns, svcName, portName string, port int32, pod *corev1.Pod, ready bool) {
	t.Helper()
	protocol := corev1.ProtocolTCP
	_, err := endpointSlicesResourceStore(storage).Create(context.Background(), ns, &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: ns,
			Labels:    map[string]string{discoveryv1.LabelServiceName: svcName},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Ports: []discoveryv1.EndpointPort{{
			Name:     &portName,
			Port:     &port,
			Protocol: &protocol,
		}},
		Endpoints: []discoveryv1.Endpoint{{
			Addresses:  []string{pod.Status.PodIP},
			Conditions: discoveryv1.EndpointConditions{Ready: &ready},
			TargetRef: &corev1.ObjectReference{
				Kind:      "Pod",
				Namespace: pod.Namespace,
				Name:      pod.Name,
				UID:       pod.UID,
			},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("create endpointslice %s: %v", svcName, err)
	}
}

// TestResolveVKubeProxyTarget is an end-to-end check (no wrangler dev --
// fakeKV/newTestStorage, same shape as clusterip_allocator_test.go) that
// the ClusterIP -> Service -> EndpointSlice resolution nodes/podproxy.ts's
// handleVKubeProxy now delegates here (vkubeproxy.go) matches what that
// TS code used to compute itself via two separate HTTP round trips.
func TestResolveVKubeProxyTarget(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()
	ns := "default"

	pod := mustCreatePod(t, storage, ns, &corev1.Pod{
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

	mustCreateEndpointSlice(t, storage, ns, "web", "http", 8080, pod, true)

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
		// A Pod that's Ready=false is published in the slice with
		// Conditions.Ready=false and must be excluded here.
		kv2 := newFakeKV()
		storage2 := newTestStorage(kv2)
		notReady := mustCreatePod(t, storage2, ns, &corev1.Pod{
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
		mustCreateEndpointSlice(t, storage2, ns, "web", "http", 8080, notReady, false)
		if _, _, err := ResolveVKubeProxyTarget(ctx, storage2, "10.43.0.10", 80); err == nil {
			t.Error("expected an error when no endpoint is ready, got nil")
		}
	})
}
