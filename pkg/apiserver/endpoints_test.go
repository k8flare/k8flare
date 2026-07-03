package apiserver

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func mustCreatePod(t *testing.T, storage *Storage, ns string, pod *corev1.Pod) *corev1.Pod {
	t.Helper()
	obj, err := podsResourceStore(storage).Create(context.Background(), ns, pod)
	if err != nil {
		t.Fatalf("create pod %s: %v", pod.Name, err)
	}
	return obj.(*corev1.Pod)
}

func mustCreateService(t *testing.T, storage *Storage, ns string, svc *corev1.Service) *corev1.Service {
	t.Helper()
	obj, err := servicesResourceStore(storage).Create(context.Background(), ns, svc)
	if err != nil {
		t.Fatalf("create service %s: %v", svc.Name, err)
	}
	return obj.(*corev1.Service)
}

// TestReconcileNamespaceEndpoints_BasicMatch is an end-to-end check (no
// wrangler dev -- fakeKV/newTestStorage, like clusterip_allocator_test.go)
// that a Service with a selector and one ready, IP-bearing matching Pod gets
// a real EndpointSlice and legacy Endpoints populated, restoring the
// behavior workers/storage/src/endpoints.ts had before its deletion (see
// docs/platform-verification.md's Phase 5 findings).
func TestReconcileNamespaceEndpoints_BasicMatch(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()
	ns := "default"

	mustCreatePod(t, storage, ns, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-1", Labels: map[string]string{"app": "web"}},
		Spec: corev1.PodSpec{
			NodeName:   "node-1",
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
			Selector: map[string]string{"app": "web"},
			Ports:    []corev1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("http")}},
		},
	})

	if err := ReconcileNamespaceEndpoints(ctx, storage, ns); err != nil {
		t.Fatalf("ReconcileNamespaceEndpoints: %v", err)
	}

	sliceObj, err := endpointSlicesResourceStore(storage).Get(ctx, ns, "web")
	if err != nil {
		t.Fatalf("get EndpointSlice: %v", err)
	}
	slice := sliceObj.(*discoveryv1.EndpointSlice)
	if len(slice.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(slice.Endpoints))
	}
	if got := slice.Endpoints[0].Addresses; len(got) != 1 || got[0] != "10.42.0.5" {
		t.Errorf("expected address [10.42.0.5], got %v", got)
	}
	if slice.Endpoints[0].Conditions.Ready == nil || !*slice.Endpoints[0].Conditions.Ready {
		t.Errorf("expected Ready=true")
	}
	if len(slice.Ports) != 1 || slice.Ports[0].Port == nil || *slice.Ports[0].Port != 8080 {
		t.Errorf("expected named port \"http\" resolved to 8080, got %+v", slice.Ports)
	}
	if slice.Labels[discoveryv1.LabelServiceName] != "web" {
		t.Errorf("expected kubernetes.io/service-name label, got %v", slice.Labels)
	}

	epsObj, err := endpointsResourceStore(storage).Get(ctx, ns, "web")
	if err != nil {
		t.Fatalf("get Endpoints: %v", err)
	}
	eps := epsObj.(*corev1.Endpoints)
	if len(eps.Subsets) != 1 || len(eps.Subsets[0].Addresses) != 1 || eps.Subsets[0].Addresses[0].IP != "10.42.0.5" {
		t.Errorf("expected one subset with address 10.42.0.5, got %+v", eps.Subsets)
	}
	if len(eps.Subsets[0].Ports) != 1 || eps.Subsets[0].Ports[0].Port != 8080 {
		t.Errorf("expected legacy Endpoints port 8080, got %+v", eps.Subsets[0].Ports)
	}
}

// TestReconcileNamespaceEndpoints_NotReadyPodGoesToNotReadyAddresses checks
// that a Pod failing its readiness condition is still published (both APIs
// list not-ready addresses separately, they don't omit them) but marked not
// ready in both the legacy Endpoints split and the EndpointSlice condition.
func TestReconcileNamespaceEndpoints_NotReadyPodGoesToNotReadyAddresses(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()
	ns := "default"

	mustCreatePod(t, storage, ns, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-1", Labels: map[string]string{"app": "web"}},
		Status: corev1.PodStatus{
			PodIP:      "10.42.0.6",
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}},
		},
	})
	mustCreateService(t, storage, ns, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "web"}, Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt(80)}}},
	})

	if err := ReconcileNamespaceEndpoints(ctx, storage, ns); err != nil {
		t.Fatalf("ReconcileNamespaceEndpoints: %v", err)
	}

	epsObj, err := endpointsResourceStore(storage).Get(ctx, ns, "web")
	if err != nil {
		t.Fatalf("get Endpoints: %v", err)
	}
	eps := epsObj.(*corev1.Endpoints)
	if len(eps.Subsets) != 1 || len(eps.Subsets[0].Addresses) != 0 || len(eps.Subsets[0].NotReadyAddresses) != 1 {
		t.Errorf("expected the not-ready pod in NotReadyAddresses only, got %+v", eps.Subsets)
	}

	sliceObj, err := endpointSlicesResourceStore(storage).Get(ctx, ns, "web")
	if err != nil {
		t.Fatalf("get EndpointSlice: %v", err)
	}
	slice := sliceObj.(*discoveryv1.EndpointSlice)
	if len(slice.Endpoints) != 1 || slice.Endpoints[0].Conditions.Ready == nil || *slice.Endpoints[0].Conditions.Ready {
		t.Errorf("expected Ready=false, got %+v", slice.Endpoints)
	}
}

// TestReconcileNamespaceEndpoints_SkipsNoSelectorAndExternalName matches
// endpoints.ts's scope exactly: Services without a selector (externally
// managed) or of type ExternalName never get an Endpoints/EndpointSlice
// written here.
func TestReconcileNamespaceEndpoints_SkipsNoSelectorAndExternalName(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()
	ns := "default"

	mustCreateService(t, storage, ns, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "no-selector"},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
	})
	mustCreateService(t, storage, ns, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "external"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeExternalName, ExternalName: "example.com"},
	})

	if err := ReconcileNamespaceEndpoints(ctx, storage, ns); err != nil {
		t.Fatalf("ReconcileNamespaceEndpoints: %v", err)
	}

	for _, name := range []string{"no-selector", "external"} {
		if _, err := endpointsResourceStore(storage).Get(ctx, ns, name); err == nil {
			t.Errorf("expected no Endpoints for %s, but one exists", name)
		}
		if _, err := endpointSlicesResourceStore(storage).Get(ctx, ns, name); err == nil {
			t.Errorf("expected no EndpointSlice for %s, but one exists", name)
		}
	}
}

// TestDeleteServiceEndpoints_RemovesBothObjectsAndIsIdempotent checks the
// Service-delete cleanup path (handler.go) and that calling it twice (or on
// a Service that never got endpoints computed) is a safe no-op, matching
// ReleaseClusterIP/ReleasePodCIDR's idempotent style.
func TestDeleteServiceEndpoints_RemovesBothObjectsAndIsIdempotent(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	ctx := context.Background()
	ns := "default"

	mustCreatePod(t, storage, ns, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-1", Labels: map[string]string{"app": "web"}},
		Status:     corev1.PodStatus{PodIP: "10.42.0.7"},
	})
	mustCreateService(t, storage, ns, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "web"}, Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt(80)}}},
	})
	if err := ReconcileNamespaceEndpoints(ctx, storage, ns); err != nil {
		t.Fatalf("ReconcileNamespaceEndpoints: %v", err)
	}

	DeleteServiceEndpoints(ctx, storage, ns, "web")

	if _, err := endpointsResourceStore(storage).Get(ctx, ns, "web"); err == nil {
		t.Error("expected Endpoints to be deleted")
	}
	if _, err := endpointSlicesResourceStore(storage).Get(ctx, ns, "web"); err == nil {
		t.Error("expected EndpointSlice to be deleted")
	}

	// Calling it again (nothing left to delete) must not panic or leave an
	// error anywhere a caller could observe -- DeleteServiceEndpoints has no
	// return value precisely because it's meant to be safe to call
	// unconditionally, the same as ReleaseClusterIP/ReleasePodCIDR.
	DeleteServiceEndpoints(ctx, storage, ns, "web")
}
