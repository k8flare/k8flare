package workloads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

type fakeKine struct {
	mu   sync.Mutex
	data map[string][]byte
	revs map[string]int64
}

func (f *fakeKine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/list":
		prefix := r.URL.Query().Get("prefix")
		kvs := []map[string]any{}
		for k, v := range f.data {
			if strings.HasPrefix(k, prefix) {
				kvs = append(kvs, map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": f.revs[k]})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kvs": kvs})
	case "/kv":
		var body struct {
			Key      string `json:"key"`
			Value    string `json:"value"`
			Revision int64  `json:"revision"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.Revision != 0 && f.revs[body.Key] != body.Revision {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "error": "conflict"})
			return
		}
		if r.Method == http.MethodDelete {
			delete(f.data, body.Key)
			delete(f.revs, body.Key)
		} else {
			value, _ := base64.StdEncoding.DecodeString(body.Value)
			f.data[body.Key] = value
			f.revs[body.Key]++
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": f.revs[body.Key]})
	default:
		http.NotFound(w, r)
	}
}

func useFakeTaintStore(t *testing.T) *fakeKine {
	store := &fakeKine{data: map[string][]byte{}, revs: map[string]int64{}}
	srv := httptest.NewServer(store)
	t.Cleanup(srv.Close)
	previous := Store
	Store = &kine.Client{HTTP: &http.Client{Transport: rewriteHost(nil, srv.URL)}}
	t.Cleanup(func() { Store = previous })
	return store
}

func taintedNode(taints ...v1.Taint) *v1.Node {
	return &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Spec: v1.NodeSpec{Taints: taints}}
}

func evictTaint() v1.Taint {
	return v1.Taint{Key: "kubernetes.io/e2e-evict-taint-key", Value: "evictTaintVal", Effect: v1.TaintEffectNoExecute}
}

func podOnNode(name string, tolerationSeconds *int64) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID("uid-" + name)},
		Spec:       v1.PodSpec{NodeName: "n1"},
	}
	if tolerationSeconds != nil {
		pod.Spec.Tolerations = []v1.Toleration{{Key: "kubernetes.io/e2e-evict-taint-key", Operator: v1.TolerationOpEqual, Value: "evictTaintVal", Effect: v1.TaintEffectNoExecute, TolerationSeconds: tolerationSeconds}}
	}
	return pod
}

func TestTaintEvictionDeletesAnIntolerantPodAtOnce(t *testing.T) {
	useFakeTaintStore(t)
	pod := podOnNode("b0", nil)
	client := fake.NewSimpleClientset(pod)
	pending.reset()
	if err := evictTaintedPods(context.Background(), client, []*v1.Node{taintedNode(evictTaint())}, []*v1.Pod{pod}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().Pods("default").Get(context.Background(), "b0", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("pod still present: %v", err)
	}
	var patched bool
	for _, action := range client.Actions() {
		if action.GetVerb() == "patch" && action.GetSubresource() == "status" {
			patched = true
		}
	}
	if !patched {
		t.Fatal("the DisruptionTarget condition was not written before the delete")
	}
}

func TestTaintEvictionKeepsTheTolerationStartAcrossPasses(t *testing.T) {
	store := useFakeTaintStore(t)
	pod := podOnNode("b1", ptr.To[int64](5))
	client := fake.NewSimpleClientset(pod)
	nodes := []*v1.Node{taintedNode(evictTaint())}
	start := time.Now()
	pending.reset()
	if err := evictTaintedPods(context.Background(), client, nodes, []*v1.Pod{pod}, start); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().Pods("default").Get(context.Background(), "b1", metav1.GetOptions{}); err != nil {
		t.Fatalf("pod evicted before its toleration elapsed: %v", err)
	}
	if next, ok := pending.next(); !ok || next > 5*time.Second {
		t.Fatalf("next pass booked in %s, %v", next, ok)
	}
	if len(store.data) != 1 {
		t.Fatalf("records = %v", store.data)
	}
	pending.reset()
	if err := evictTaintedPods(context.Background(), client, nodes, []*v1.Pod{pod}, start.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().Pods("default").Get(context.Background(), "b1", metav1.GetOptions{}); err != nil {
		t.Fatalf("pod evicted after 3 of 5 seconds: %v", err)
	}
	if next, ok := pending.next(); !ok || next > 2*time.Second {
		t.Fatalf("second pass booked the next one in %s, want the remaining 2s", next)
	}
	if err := evictTaintedPods(context.Background(), client, nodes, []*v1.Pod{pod}, start.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().Pods("default").Get(context.Background(), "b1", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("pod not evicted when the toleration elapsed: %v", err)
	}
	if len(store.data) != 0 {
		t.Fatalf("record kept after the eviction: %v", store.data)
	}
}

func TestTaintEvictionForgetsAPodWhoseTaintWentAway(t *testing.T) {
	store := useFakeTaintStore(t)
	pod := podOnNode("b2", ptr.To[int64](25))
	client := fake.NewSimpleClientset(pod)
	start := time.Now()
	if err := evictTaintedPods(context.Background(), client, []*v1.Node{taintedNode(evictTaint())}, []*v1.Pod{pod}, start); err != nil {
		t.Fatal(err)
	}
	if len(store.data) != 1 {
		t.Fatalf("records = %v", store.data)
	}
	if err := evictTaintedPods(context.Background(), client, []*v1.Node{taintedNode()}, []*v1.Pod{pod}, start.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(store.data) != 0 {
		t.Fatalf("record kept after the taint was removed: %v", store.data)
	}
	if err := evictTaintedPods(context.Background(), client, []*v1.Node{taintedNode(evictTaint())}, []*v1.Pod{pod}, start.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().Pods("default").Get(context.Background(), "b2", metav1.GetOptions{}); err != nil {
		t.Fatalf("a re-added taint counted from the old start: %v", err)
	}
}
