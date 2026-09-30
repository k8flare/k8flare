package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
)

func TestEvictionDeletesTerminalPod(t *testing.T) {
	h := newEvictionHarness(t, runningPod("p", corev1.PodSucceeded, false), nil)
	out, err := h.Create(h.ctx, "p", &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if st, ok := out.(*metav1.Status); !ok || st.Status != metav1.StatusSuccess {
		t.Fatalf("got %#v", out)
	}
	if !h.evicted() {
		t.Fatal("expected delete")
	}
}

func TestEvictionDeniedWhenPDBHasNoDisruptions(t *testing.T) {
	h := newEvictionHarness(t, runningPod("p", corev1.PodRunning, true), &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb", Namespace: "ns", Generation: 1},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "p"}}},
		Status:     policyv1.PodDisruptionBudgetStatus{ObservedGeneration: 1, DisruptionsAllowed: 0, DesiredHealthy: 1, CurrentHealthy: 1},
	})
	_, err := h.Create(h.ctx, "p", &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if !apierrors.IsTooManyRequests(err) {
		t.Fatalf("got %v", err)
	}
	if h.evicted() {
		t.Fatal("must not delete")
	}
}

func TestEvictionDecrementsPDBAndDeletes(t *testing.T) {
	h := newEvictionHarness(t, runningPod("p", corev1.PodRunning, true), &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb", Namespace: "ns", Generation: 1},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "p"}}},
		Status:     policyv1.PodDisruptionBudgetStatus{ObservedGeneration: 1, DisruptionsAllowed: 1, DesiredHealthy: 1, CurrentHealthy: 2},
	})
	if _, err := h.Create(h.ctx, "p", &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if !h.evicted() {
		t.Fatal("expected delete")
	}
	got := h.pdb("pdb")
	if got.Status.DisruptionsAllowed != 0 {
		t.Fatalf("allowed %d", got.Status.DisruptionsAllowed)
	}
	if _, ok := got.Status.DisruptedPods["p"]; !ok {
		t.Fatal("missing disrupted pod")
	}
}

func TestEvictionRejectsStaleResourceVersion(t *testing.T) {
	h := newEvictionHarness(t, runningPod("p", corev1.PodSucceeded, false), nil)
	stale := "0"
	_, err := h.Create(h.ctx, "p", &policyv1.Eviction{
		ObjectMeta:    metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		DeleteOptions: &metav1.DeleteOptions{Preconditions: &metav1.Preconditions{ResourceVersion: &stale}},
	}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if !apierrors.IsConflict(err) {
		t.Fatalf("got %v", err)
	}
	if h.evicted() {
		t.Fatal("must not delete")
	}
}

func TestEvictionDeletesUnreadyPodWithoutDecrement(t *testing.T) {
	h := newEvictionHarness(t, runningPod("p", corev1.PodRunning, false), &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb", Namespace: "ns", Generation: 1},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "p"}}},
		Status:     policyv1.PodDisruptionBudgetStatus{ObservedGeneration: 1, DisruptionsAllowed: 0, DesiredHealthy: 1, CurrentHealthy: 1},
	})
	if _, err := h.Create(h.ctx, "p", &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if !h.evicted() {
		t.Fatal("expected delete")
	}
	if got := h.pdb("pdb"); got.Status.DisruptionsAllowed != 0 {
		t.Fatalf("allowed %d", got.Status.DisruptionsAllowed)
	}
}

func TestEvictionDeletesPendingPodWithoutDecrement(t *testing.T) {
	h := newEvictionHarness(t, runningPod("p", corev1.PodPending, false), &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb", Namespace: "ns", Generation: 1},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "p"}}},
		Status:     policyv1.PodDisruptionBudgetStatus{ObservedGeneration: 1, DisruptionsAllowed: 0, DesiredHealthy: 1, CurrentHealthy: 1},
	})
	if _, err := h.Create(h.ctx, "p", &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if !h.evicted() {
		t.Fatal("expected delete")
	}
	if got := h.pdb("pdb"); got.Status.DisruptionsAllowed != 0 {
		t.Fatalf("allowed %d", got.Status.DisruptionsAllowed)
	}
}

func TestEvictionRejectsOversizedDisruptedPods(t *testing.T) {
	disrupted := map[string]metav1.Time{}
	for i := 0; i < 2001; i++ {
		disrupted[fmt.Sprintf("p%d", i)] = metav1.Now()
	}
	h := newEvictionHarness(t, runningPod("p", corev1.PodRunning, true), &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb", Namespace: "ns", Generation: 1},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "p"}}},
		Status:     policyv1.PodDisruptionBudgetStatus{ObservedGeneration: 1, DisruptionsAllowed: 1, DisruptedPods: disrupted},
	})
	_, err := h.Create(h.ctx, "p", &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if !apierrors.IsForbidden(err) {
		t.Fatalf("got %v", err)
	}
	if h.evicted() {
		t.Fatal("must not delete")
	}
}

func TestEvictionSyncFailedMessage(t *testing.T) {
	msg := disruptionDeniedMessage(policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb"},
		Status: policyv1.PodDisruptionBudgetStatus{
			DesiredHealthy: 1,
			CurrentHealthy: 1,
			Conditions: []metav1.Condition{{
				Type:    policyv1.DisruptionAllowedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  policyv1.SyncFailedReason,
				Message: "selector boom",
			}},
		},
	})
	if !strings.Contains(msg, "failed sync: selector boom") {
		t.Fatalf("msg %q", msg)
	}
}

func TestEvictionRetriesPDBConflict(t *testing.T) {
	h := newEvictionHarness(t, runningPod("p", corev1.PodRunning, true), &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb", Namespace: "ns", Generation: 1},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "p"}}},
		Status:     policyv1.PodDisruptionBudgetStatus{ObservedGeneration: 1, DisruptionsAllowed: 1, DesiredHealthy: 1, CurrentHealthy: 2},
	})
	h.failPDBPuts = 1
	if _, err := h.Create(h.ctx, "p", &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if !h.evicted() {
		t.Fatal("expected delete")
	}
	if got := h.pdb("pdb"); got.Status.DisruptionsAllowed != 0 {
		t.Fatalf("allowed %d", got.Status.DisruptionsAllowed)
	}
}

func TestEvictionDryRunLeavesPodAndBudget(t *testing.T) {
	h := newEvictionHarness(t, runningPod("p", corev1.PodRunning, true), &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb", Namespace: "ns", Generation: 1},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "p"}}},
		Status:     policyv1.PodDisruptionBudgetStatus{ObservedGeneration: 1, DisruptionsAllowed: 1, DesiredHealthy: 1, CurrentHealthy: 2},
	})
	_, err := h.Create(h.ctx, "p", &policyv1.Eviction{
		ObjectMeta:    metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		DeleteOptions: &metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}},
	}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if h.evicted() {
		t.Fatal("dry-run must not delete")
	}
	if got := h.pdb("pdb"); got.Status.DisruptionsAllowed != 1 {
		t.Fatalf("allowed %d", got.Status.DisruptionsAllowed)
	}
}

func TestEvictionDryRunRejectsStaleResourceVersion(t *testing.T) {
	h := newEvictionHarness(t, runningPod("p", corev1.PodSucceeded, false), nil)
	stale := "0"
	_, err := h.Create(h.ctx, "p", &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		DeleteOptions: &metav1.DeleteOptions{
			DryRun:        []string{metav1.DryRunAll},
			Preconditions: &metav1.Preconditions{ResourceVersion: &stale},
		},
	}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if !apierrors.IsConflict(err) {
		t.Fatalf("got %v", err)
	}
	if h.evicted() {
		t.Fatal("dry-run must not delete")
	}
}

func TestEvictionNameMismatch(t *testing.T) {
	h := newEvictionHarness(t, runningPod("p", corev1.PodRunning, true), nil)
	_, err := h.Create(h.ctx, "p", &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: "other"}}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err == nil {
		t.Fatal("expected name mismatch")
	}
}

type evictionHarness struct {
	*evictionREST
	ctx         context.Context
	data        map[string][]byte
	mu          sync.Mutex
	failPDBPuts int
}

func (h *evictionHarness) evicted() bool {
	got, err := h.pods.Get(h.ctx, "p", &metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return true
	}
	if err != nil {
		return false
	}
	pod := got.(*corev1.Pod)
	if pod.DeletionTimestamp != nil {
		return true
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.DisruptionTarget && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (h *evictionHarness) pdb(name string) policyv1.PodDisruptionBudget {
	h.mu.Lock()
	defer h.mu.Unlock()
	var pdb policyv1.PodDisruptionBudget
	if err := json.Unmarshal(h.data["/registry/poddisruptionbudgets/ns/"+name], &pdb); err != nil {
		return policyv1.PodDisruptionBudget{}
	}
	return pdb
}

func newEvictionHarness(t *testing.T, pod *corev1.Pod, pdb *policyv1.PodDisruptionBudget) *evictionHarness {
	t.Helper()
	h := &evictionHarness{data: map[string][]byte{}, ctx: genericapirequest.WithNamespace(context.Background(), "ns")}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		if r.URL.Path != "/kv" && r.URL.Path != "/list" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/list" {
			prefix := r.URL.Query().Get("prefix")
			var kvs []map[string]any
			for k, v := range h.data {
				if strings.HasPrefix(k, prefix) {
					kvs = append(kvs, map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 1})
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kvs": kvs})
			return
		}
		if r.Method == http.MethodDelete {
			var body struct {
				Key string `json:"key"`
			}
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			delete(h.data, body.Key)
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 3})
			return
		}
		if r.Method == http.MethodGet {
			k := r.URL.Query().Get("key")
			if v, ok := h.data[k]; ok {
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
		if strings.Contains(body.Key, "poddisruptionbudgets") && h.failPDBPuts > 0 {
			h.failPDBPuts--
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
			return
		}
		raw, _ := base64.StdEncoding.DecodeString(body.Value)
		h.data[body.Key] = raw
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 2})
	}))
	t.Cleanup(srv.Close)
	client := &kine.Client{HTTP: &http.Client{Transport: rewrite{base: srv.URL, next: srv.Client().Transport}}}
	if pdb != nil {
		raw, err := json.Marshal(pdb)
		if err != nil {
			t.Fatal(err)
		}
		h.data["/registry/poddisruptionbudgets/ns/"+pdb.Name] = raw
	}
	pods, err := registry.NewStore(client, schema.GroupVersion{Version: "v1"}, metav1.APIResource{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod"})
	if err != nil {
		t.Fatal(err)
	}
	wantStatus := *pod.Status.DeepCopy()
	if _, err := pods.Create(h.ctx, pod, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	h.restorePodStatus(t, pod.Namespace, pod.Name, wantStatus)
	h.evictionREST = newEvictionREST(pods, client).(*evictionREST)
	return h
}

func (h *evictionHarness) restorePodStatus(t *testing.T, namespace, name string, want corev1.PodStatus) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	key := "/registry/pods/" + namespace + "/" + name
	raw, ok := h.data[key]
	if !ok {
		t.Fatalf("pod not stored at %s", key)
	}
	var stored corev1.Pod
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	stored.Status = want
	out, err := json.Marshal(&stored)
	if err != nil {
		t.Fatal(err)
	}
	h.data[key] = out
}

func runningPod(name string, phase corev1.PodPhase, ready bool) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns", Labels: map[string]string{"app": "p"}, UID: "uid-1"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "pause"}}},
		Status:     corev1.PodStatus{Phase: phase},
	}
	if ready {
		pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	}
	scheme.Scheme.Default(pod)
	return pod
}

func TestCanIgnorePDB(t *testing.T) {
	if !canIgnorePDB(&corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}}) {
		t.Fatal("pending")
	}
	if canIgnorePDB(&corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}) {
		t.Fatal("running")
	}
	if !canIgnorePDB(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: ptr.To(metav1.Now())}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}) {
		t.Fatal("deleting")
	}
}

func TestDisruptionTargetKeepsTransitionTime(t *testing.T) {
	at := metav1.NewTime(time.Now().Add(-time.Hour))
	pod := &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{
		Type:               corev1.DisruptionTarget,
		Status:             corev1.ConditionTrue,
		Reason:             "EvictionByEvictionAPI",
		Message:            "Eviction API: evicting",
		LastTransitionTime: at,
	}}}}
	setDisruptionTarget(pod)
	if !pod.Status.Conditions[0].LastTransitionTime.Equal(&at) {
		t.Fatalf("transition %+v", pod.Status.Conditions[0])
	}
	other := pod.DeepCopy()
	other.Status.Conditions[0].Reason = "Other"
	setDisruptionTarget(other)
	if !other.Status.Conditions[0].LastTransitionTime.Equal(&at) {
		t.Fatalf("kept %+v", other.Status.Conditions[0])
	}
	if other.Status.Conditions[0].Reason != "EvictionByEvictionAPI" {
		t.Fatalf("reason %s", other.Status.Conditions[0].Reason)
	}
}
