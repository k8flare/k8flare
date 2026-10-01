package admission

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestResourceQuotaDeniesLoadBalancerOverNodePort(t *testing.T) {
	store := &memStore{data: map[string][]byte{
		"/registry/resourcequotas/default/q": mustJSON(t, corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "default"},
			Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
				corev1.ResourceServices:              resource.MustParse("10"),
				corev1.ResourceServicesNodePorts:     resource.MustParse("1"),
				corev1.ResourceServicesLoadBalancers: resource.MustParse("1"),
			}},
			Status: corev1.ResourceQuotaStatus{Hard: corev1.ResourceList{
				corev1.ResourceServices:              resource.MustParse("10"),
				corev1.ResourceServicesNodePorts:     resource.MustParse("1"),
				corev1.ResourceServicesLoadBalancers: resource.MustParse("1"),
			}},
		}),
		"/registry/services/default/clusterip": mustJSON(t, corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "clusterip", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Ports: []corev1.ServicePort{{Port: 80}}},
		}),
		"/registry/services/default/nodeport": mustJSON(t, corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "nodeport", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Ports: []corev1.ServicePort{{Port: 80}}},
		}),
	}}
	kine := httptest.NewServer(store)
	defer kine.Close()
	h := NewHandler(Config{Kine: rewriteClient(kine)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "validate",
		Name:      "lb",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "services"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Service"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]any{"name": "lb", "namespace": "default"},
			"spec": map[string]any{
				"type":                          "LoadBalancer",
				"allocateLoadBalancerNodePorts": true,
				"ports":                         []any{map[string]any{"port": 80}},
			},
		},
	})
	if out.Allowed {
		t.Fatal("expected nodeport quota deny")
	}
	out = postAdmit(t, h, admit.Request{
		Phase:     "validate",
		Name:      "cip2",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "services"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Service"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]any{"name": "cip2", "namespace": "default"},
			"spec":     map[string]any{"type": "ClusterIP", "ports": []any{map[string]any{"port": 80}}},
		},
	})
	if !out.Allowed {
		t.Fatalf("clusterip should be allowed: %+v", out)
	}
}

func TestResourceQuotaDeniesLoadBalancerWhenStatusUsedIsStale(t *testing.T) {
	zero := corev1.ResourceList{
		corev1.ResourceServices:              resource.MustParse("0"),
		corev1.ResourceServicesNodePorts:     resource.MustParse("0"),
		corev1.ResourceServicesLoadBalancers: resource.MustParse("0"),
	}
	store := &memStore{data: map[string][]byte{
		"/registry/resourcequotas/default/q": mustJSON(t, corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "default"},
			Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
				corev1.ResourceServices:              resource.MustParse("10"),
				corev1.ResourceServicesNodePorts:     resource.MustParse("1"),
				corev1.ResourceServicesLoadBalancers: resource.MustParse("1"),
			}},
			Status: corev1.ResourceQuotaStatus{Hard: corev1.ResourceList{
				corev1.ResourceServices:              resource.MustParse("10"),
				corev1.ResourceServicesNodePorts:     resource.MustParse("1"),
				corev1.ResourceServicesLoadBalancers: resource.MustParse("1"),
			}, Used: zero},
		}),
		"/registry/services/default/nodeport": mustJSON(t, corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "nodeport", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Ports: []corev1.ServicePort{{Port: 80}}},
		}),
	}}
	kine := httptest.NewServer(store)
	defer kine.Close()
	h := NewHandler(Config{Kine: rewriteClient(kine)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "validate",
		Name:      "lb",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "services"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Service"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]any{"name": "lb", "namespace": "default"},
			"spec": map[string]any{
				"type":                          "LoadBalancer",
				"allocateLoadBalancerNodePorts": true,
				"ports":                         []any{map[string]any{"port": 80}},
			},
		},
	})
	if out.Allowed {
		t.Fatal("expected nodeport quota deny despite stale zero usage")
	}
}

func TestResourceQuotaDeniesReplicaSetOverCount(t *testing.T) {
	store := &memStore{data: map[string][]byte{
		"/registry/resourcequotas/default/q": mustJSON(t, corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "default"},
			Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
				corev1.ResourceName("count/replicasets.apps"): resource.MustParse("1"),
			}},
		}),
		"/registry/replicasets/default/rs": mustJSON(t, map[string]any{
			"apiVersion": "apps/v1", "kind": "ReplicaSet",
			"metadata": map[string]any{"name": "rs", "namespace": "default"},
		}),
	}}
	kine := httptest.NewServer(store)
	defer kine.Close()
	h := NewHandler(Config{Kine: rewriteClient(kine)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "validate",
		Name:      "rs2",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"},
		Kind:      schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "apps/v1", "kind": "ReplicaSet",
			"metadata": map[string]any{"name": "rs2", "namespace": "default"},
		},
	})
	if out.Allowed {
		t.Fatal("expected replicaset count quota deny")
	}
}

func TestResourceQuotaDeniesIngressOverCount(t *testing.T) {
	store := &memStore{data: map[string][]byte{
		"/registry/resourcequotas/default/q": mustJSON(t, corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "default"},
			Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
				corev1.ResourceName("count/ingresses.networking.k8s.io"): resource.MustParse("1"),
			}},
		}),
		"/registry/ingresses/default/web": mustJSON(t, map[string]any{
			"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
			"metadata": map[string]any{"name": "web", "namespace": "default"},
		}),
	}}
	kine := httptest.NewServer(store)
	defer kine.Close()
	h := NewHandler(Config{Kine: rewriteClient(kine)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "validate",
		Name:      "web2",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		Kind:      schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
			"metadata": map[string]any{"name": "web2", "namespace": "default"},
		},
	})
	if out.Allowed {
		t.Fatal("expected ingress count quota deny")
	}
}

func TestResourceQuotaOfficialExceedMessage(t *testing.T) {
	store := &memStore{data: map[string][]byte{
		"/registry/resourcequotas/default/q": mustJSON(t, corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "default"},
			Spec:       corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{corev1.ResourceConfigMaps: resource.MustParse("1")}},
			Status:     corev1.ResourceQuotaStatus{Hard: corev1.ResourceList{corev1.ResourceConfigMaps: resource.MustParse("1")}},
		}),
		"/registry/configmaps/default/a": mustJSON(t, corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"}}),
	}}
	kine := httptest.NewServer(store)
	defer kine.Close()
	h := NewHandler(Config{Kine: rewriteClient(kine)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "validate",
		Name:      "b",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "configmaps"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]any{"name": "b", "namespace": "default"},
		},
	})
	if out.Allowed {
		t.Fatal("expected configmap quota deny")
	}
	want := "exceeded quota: q, requested: configmaps=1, used: configmaps=1, limited: configmaps=1"
	if !strings.Contains(out.Message, want) {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestResourceQuotaDeniesPodOverCPU(t *testing.T) {
	store := &memStore{data: map[string][]byte{
		"/registry/resourcequotas/default/test-quota": mustJSON(t, corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "test-quota", Namespace: "default"},
			Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
				corev1.ResourcePods:   resource.MustParse("5"),
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			}},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{
					corev1.ResourcePods:   resource.MustParse("5"),
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("500Mi"),
				},
				Used: corev1.ResourceList{
					corev1.ResourcePods:   resource.MustParse("1"),
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("252Mi"),
				},
			},
		}),
	}}
	kine := httptest.NewServer(store)
	defer kine.Close()
	h := NewHandler(Config{Kine: rewriteClient(kine)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "validate",
		Name:      "fail-pod",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "fail-pod", "namespace": "default"},
			"spec": map[string]any{
				"serviceAccountName": "default",
				"containers": []any{map[string]any{
					"name":  "pause",
					"image": "registry.k8s.io/pause:3.10",
					"resources": map[string]any{
						"requests": map[string]any{"cpu": "600m", "memory": "100Mi"},
					},
				}},
			},
		},
	})
	if out.Allowed {
		t.Fatal("expected pod cpu quota deny")
	}
	want := "exceeded quota: test-quota, requested: cpu=600m, used: cpu=500m, limited: cpu=1"
	if !strings.Contains(out.Message, want) {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestResourceQuotaReservesUsageBeforeTheObjectIsStored(t *testing.T) {
	store := &memStore{data: map[string][]byte{
		"/registry/resourcequotas/default/condition-test": mustJSON(t, corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "condition-test", Namespace: "default"},
			Spec:       corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("2")}},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("2")},
				Used: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("1")},
			},
		}),
		"/registry/pods/default/condition-test-1": mustJSON(t, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "condition-test-1", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "agnhost", Image: "agnhost"}}},
		}),
	}}
	kine := httptest.NewServer(store)
	defer kine.Close()
	h := NewHandler(Config{Kine: rewriteClient(kine)})
	create := func(name string) admit.Response {
		return postAdmit(t, h, admit.Request{
			Phase:     "validate",
			Name:      name,
			Namespace: "default",
			Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
			Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
			Operation: "CREATE",
			Object: map[string]any{
				"apiVersion": "v1", "kind": "Pod",
				"metadata": map[string]any{"name": name, "namespace": "default"},
				"spec": map[string]any{
					"serviceAccountName": "default",
					"containers":         []any{map[string]any{"name": "agnhost", "image": "agnhost"}},
				},
			},
		})
	}
	if out := create("condition-test-2"); !out.Allowed {
		t.Fatalf("second pod should fit: %+v", out)
	}
	var rq corev1.ResourceQuota
	if err := json.Unmarshal(store.data["/registry/resourcequotas/default/condition-test"], &rq); err != nil {
		t.Fatal(err)
	}
	if used := rq.Status.Used[corev1.ResourcePods]; used.Cmp(resource.MustParse("2")) != 0 {
		t.Fatalf("status.used.pods = %s after the admitted create, want 2", used.String())
	}
	if out := create("condition-test-3"); out.Allowed {
		t.Fatal("third pod was admitted while the second one was not stored yet")
	} else if want := "exceeded quota: condition-test, requested: pods=1, used: pods=2, limited: pods=2"; !strings.Contains(out.Message, want) {
		t.Fatalf("message = %q", out.Message)
	}
}

func quotaPodRequest(phase, name string, requests, limits map[string]any) admit.Request {
	return admit.Request{
		Phase:     phase,
		Name:      name,
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": name, "namespace": "default"},
			"spec": map[string]any{
				"serviceAccountName": "default",
				"containers": []any{map[string]any{
					"name": "pause", "image": "registry.k8s.io/pause:3.10",
					"resources": map[string]any{"requests": requests, "limits": limits},
				}},
			},
		},
	}
}

func TestResourceQuotaReservesOnlyInTheValidatePhase(t *testing.T) {
	hard := corev1.ResourceList{
		corev1.ResourcePods:                                resource.MustParse("5"),
		corev1.ResourceCPU:                                 resource.MustParse("1"),
		corev1.ResourceMemory:                              resource.MustParse("500Mi"),
		corev1.ResourceEphemeralStorage:                    resource.MustParse("50Gi"),
		corev1.ResourceName("requests.example.com/dongle"): resource.MustParse("3"),
		corev1.ResourceConfigMaps:                          resource.MustParse("2"),
	}
	store := &memStore{data: map[string][]byte{
		"/registry/resourcequotas/default/test-quota": mustJSON(t, corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "test-quota", Namespace: "default"},
			Spec:       corev1.ResourceQuotaSpec{Hard: hard},
			Status:     corev1.ResourceQuotaStatus{Hard: hard, Used: corev1.ResourceList{corev1.ResourceConfigMaps: resource.MustParse("1")}},
		}),
		"/registry/configmaps/default/kube-root-ca.crt": mustJSON(t, corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "kube-root-ca.crt", Namespace: "default"}}),
	}}
	kine := httptest.NewServer(store)
	defer kine.Close()
	h := NewHandler(Config{Kine: rewriteClient(kine)})
	requests := map[string]any{"cpu": "500m", "memory": "252Mi", "ephemeral-storage": "30Gi", "example.com/dongle": "2"}
	limits := map[string]any{"example.com/dongle": "2"}
	usedPods := func() corev1.ResourceList {
		var rq corev1.ResourceQuota
		if err := json.Unmarshal(store.data["/registry/resourcequotas/default/test-quota"], &rq); err != nil {
			t.Fatal(err)
		}
		return rq.Status.Used
	}
	if out := postAdmit(t, h, quotaPodRequest("check", "test-pod", requests, limits)); !out.Allowed {
		t.Fatalf("admit-time check denied the pod: %+v", out)
	}
	if used := usedPods(); len(used) != 1 {
		t.Fatalf("the admit-time check reserved usage: %v", used)
	}
	if out := postAdmit(t, h, quotaPodRequest("validate", "test-pod", requests, limits)); !out.Allowed {
		t.Fatalf("the pod that fits the quota was denied: %+v", out)
	}
	used := usedPods()
	for name, want := range map[corev1.ResourceName]string{corev1.ResourcePods: "1", corev1.ResourceCPU: "500m", corev1.ResourceMemory: "252Mi", corev1.ResourceEphemeralStorage: "30Gi", "requests.example.com/dongle": "2", corev1.ResourceConfigMaps: "1"} {
		if got := used[name]; got.Cmp(resource.MustParse(want)) != 0 {
			t.Fatalf("status.used[%s] = %s, want %s", name, got.String(), want)
		}
	}
	configMap := admit.Request{
		Phase:     "validate",
		Name:      "test-configmap",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "configmaps"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]any{"name": "test-configmap", "namespace": "default"},
		},
	}
	if out := postAdmit(t, h, configMap); !out.Allowed {
		t.Fatalf("the second configmap was denied next to kube-root-ca.crt: %+v", out)
	}
	if out := postAdmit(t, h, quotaPodRequest("validate", "fail-pod-for-extended-resource", requests, limits)); out.Allowed {
		t.Fatal("a second pod asking for 2 more dongles passed a limit of 3")
	}
}
