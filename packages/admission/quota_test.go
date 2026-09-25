package admission

import (
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
	store := memStore{data: map[string][]byte{
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
	store := memStore{data: map[string][]byte{
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
	store := memStore{data: map[string][]byte{
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
	store := memStore{data: map[string][]byte{
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
	store := memStore{data: map[string][]byte{
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
	store := memStore{data: map[string][]byte{
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
