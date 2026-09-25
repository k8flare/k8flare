package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

func TestLegacyBindingAssignsNode(t *testing.T) {
	data := map[string][]byte{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path != "/kv" && r.URL.Path != "/list" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/list" {
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kvs": []any{}})
			return
		}
		if r.Method == http.MethodGet {
			k := r.URL.Query().Get("key")
			if v, ok := data[k]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kv": map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 1}})
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
			return
		}
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		raw, _ := base64.StdEncoding.DecodeString(body.Value)
		data[body.Key] = raw
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 2})
	}))
	t.Cleanup(srv.Close)
	client := &kine.Client{HTTP: &http.Client{Transport: rewrite{base: srv.URL, next: srv.Client().Transport}}}
	pods, err := registry.NewStore(client, schema.GroupVersion{Version: "v1"}, metav1.APIResource{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod"})
	if err != nil {
		t.Fatal(err)
	}
	registry.Customizers["pods"](pods, registry.Deps{})
	ctx := genericapirequest.WithNamespace(context.Background(), "ns")
	if _, err := pods.Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "pause"}}},
		Status:     corev1.PodStatus{NominatedNodeName: "other"},
	}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	prev := bindingPods.store
	bindingPods.store = pods
	t.Cleanup(func() { bindingPods.store = prev })
	if _, err := (legacyBindingREST{}).Create(ctx, &corev1.Binding{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns", Labels: map[string]string{corev1.LabelTopologyZone: "zone-a"}},
		Target:     corev1.ObjectReference{Kind: "Node", Name: "n1"},
	}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := pods.Get(ctx, "p", &metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pod := got.(*corev1.Pod)
	if pod.Spec.NodeName != "n1" {
		t.Fatalf("node %q", pod.Spec.NodeName)
	}
	if pod.Generation != 1 {
		t.Fatalf("generation=%d after binding", pod.Generation)
	}
	if pod.Labels[corev1.LabelTopologyZone] != "zone-a" {
		t.Fatalf("labels = %v", pod.Labels)
	}
	if pod.Status.NominatedNodeName != "" {
		t.Fatalf("nominated %q", pod.Status.NominatedNodeName)
	}
}
