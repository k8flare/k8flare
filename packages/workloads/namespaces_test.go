package workloads

import (
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	eventsv1 "k8s.io/api/events/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestNamespaceNames(t *testing.T) {
	got := NamespaceNames([]NamespaceMessage{
		{Kind: "change", Key: "/registry/namespaces/kube-system"},
		{Kind: "change", Key: "/registry/pods/default/a"},
		{Kind: "retry", Names: []string{"default", "kube-system"}},
	})
	if len(got) != 2 || got[0] != "kube-system" || got[1] != "default" {
		t.Fatal(got)
	}
}

func TestPickTerminatingCapsBatch(t *testing.T) {
	got, more := pickTerminating([]string{"a", "b", "c", "d"}, 3)
	if len(got) != 3 || !more {
		t.Fatalf("got %v more=%v", got, more)
	}
	got, more = pickTerminating([]string{"a"}, 3)
	if len(got) != 1 || more {
		t.Fatalf("got %v more=%v", got, more)
	}
}

func TestListTerminatingSkipsActive(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "alive"}},
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "also", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
	)
	got, err := listTerminating(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("terminating = %v", got)
	}
}

func TestListTerminatingNewestFirst(t *testing.T) {
	old := metav1.NewTime(metav1.Now().Add(-time.Hour))
	neu := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "stale", DeletionTimestamp: &old}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "fresh", DeletionTimestamp: &neu}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
	)
	got, err := listTerminating(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "fresh" || got[1] != "stale" {
		t.Fatalf("order = %v", got)
	}
}

func TestMergeTerminatingPrefersHinted(t *testing.T) {
	got := mergeTerminating([]string{"hint", ""}, []string{"also", "hint"})
	if len(got) != 2 || got[0] != "hint" || got[1] != "also" {
		t.Fatalf("got %v", got)
	}
}

func TestDeleteTerminatingHintedOnlyIgnoresStale(t *testing.T) {
	now := metav1.Now()
	old := metav1.NewTime(now.Add(-time.Hour))
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "stale", DeletionTimestamp: &old}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "fresh", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "fresh"}},
		&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "old-pod", Namespace: "stale"}},
	)
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"fresh"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Terminating != 1 || result.Deleted != 1 || result.NextMs <= 0 {
		t.Fatalf("result=%+v", result)
	}
	if _, err := client.CoreV1().Pods("fresh").Get(context.Background(), "test-pod", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("fresh pod still present: %v", err)
	}
	if _, err := client.CoreV1().Pods("stale").Get(context.Background(), "old-pod", metav1.GetOptions{}); err != nil {
		t.Fatalf("stale pod should remain: %v", err)
	}
}

func TestDeleteTerminatingRemovesPods(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "gone"}},
	)
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Terminating != 1 || result.Deleted != 1 {
		t.Fatalf("result=%+v", result)
	}
	_, err = client.CoreV1().Pods("gone").Get(context.Background(), "test-pod", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("pod still present: %v", err)
	}
}

func TestDeleteTerminatingFinalizesEmpty(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "alive"}},
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "kube-root-ca.crt", Namespace: "gone"}},
		&v1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "gone"}},
	)
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Terminating != 1 || result.Deleted != 1 {
		t.Fatalf("result=%+v", result)
	}
	_, err = client.CoreV1().ConfigMaps("gone").Get(context.Background(), "kube-root-ca.crt", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("configmap still present: %v", err)
	}
	got, err := client.CoreV1().Namespaces().Get(context.Background(), "gone", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Finalizers) != 0 {
		t.Fatalf("finalizers=%v", got.Spec.Finalizers)
	}
}

func TestDeleteTerminatingRetriesUnhinted(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "left", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
	)
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 || len(result.Names) != 1 || result.Names[0] != "left" {
		t.Fatalf("result=%+v", result)
	}
}

func TestDeleteTerminatingKeepsNamespaceWhilePDBRemains(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "gone"}},
	)
	client.PrependReactor("delete", "poddisruptionbudgets", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.Remaining == 0 {
		t.Fatalf("result=%+v", result)
	}
	got, err := client.CoreV1().Namespaces().Get(context.Background(), "gone", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Finalizers) == 0 {
		t.Fatal("finalized while pdb remains")
	}
}

func TestDeleteTerminatingKeepsNamespaceWhileEndpointSliceRemains(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "gone"}},
	)
	client.PrependReactor("delete", "endpointslices", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.Remaining == 0 {
		t.Fatalf("result=%+v", result)
	}
	got, err := client.CoreV1().Namespaces().Get(context.Background(), "gone", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Finalizers) == 0 {
		t.Fatal("finalized while endpointslice remains")
	}
}

func TestDeleteTerminatingKeepsNamespaceWhileEventRemains(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Event{ObjectMeta: metav1.ObjectMeta{Name: "left", Namespace: "gone"}},
	)
	client.PrependReactor("delete", "events", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.Remaining == 0 {
		t.Fatalf("result=%+v", result)
	}
	got, err := client.CoreV1().Namespaces().Get(context.Background(), "gone", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Finalizers) == 0 {
		t.Fatal("finalized while event remains")
	}
}

func TestDeleteTerminatingKeepsNamespaceWhileEventsV1Remains(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&eventsv1.Event{ObjectMeta: metav1.ObjectMeta{Name: "note", Namespace: "gone"}},
	)
	client.PrependReactor("delete", "events", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.Remaining == 0 {
		t.Fatalf("result=%+v", result)
	}
	got, err := client.CoreV1().Namespaces().Get(context.Background(), "gone", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Finalizers) == 0 {
		t.Fatal("finalized while events.k8s.io event remains")
	}
}

func TestDeleteTerminatingKeepsNamespaceWhileNetworkPolicyRemains(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "deny", Namespace: "gone"}},
	)
	client.PrependReactor("delete", "networkpolicies", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.Remaining == 0 {
		t.Fatalf("result=%+v", result)
	}
	got, err := client.CoreV1().Namespaces().Get(context.Background(), "gone", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Finalizers) == 0 {
		t.Fatal("finalized while networkpolicy remains")
	}
}

func TestDeleteTerminatingKeepsNamespaceWhileServiceRemains(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "gone"}},
	)
	client.PrependReactor("delete", "services", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.Remaining == 0 {
		t.Fatalf("result=%+v", result)
	}
	got, err := client.CoreV1().Namespaces().Get(context.Background(), "gone", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Finalizers) == 0 {
		t.Fatal("finalized while service remains")
	}
}

func TestDeleteTerminatingKeepsNamespaceWhileResourceClaimRemains(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&resourcev1.ResourceClaim{ObjectMeta: metav1.ObjectMeta{Name: "claim", Namespace: "gone"}},
	)
	client.PrependReactor("delete", "resourceclaims", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.Remaining == 0 {
		t.Fatalf("result=%+v", result)
	}
	got, err := client.CoreV1().Namespaces().Get(context.Background(), "gone", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Finalizers) == 0 {
		t.Fatal("finalized while resourceclaim remains")
	}
}

func TestDeleteTerminatingKeepsNamespaceWhilePodTemplateRemains(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.PodTemplate{ObjectMeta: metav1.ObjectMeta{Name: "tmpl", Namespace: "gone"}},
	)
	client.PrependReactor("delete", "podtemplates", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.Remaining == 0 {
		t.Fatalf("result=%+v", result)
	}
	got, err := client.CoreV1().Namespaces().Get(context.Background(), "gone", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Finalizers) == 0 {
		t.Fatal("finalized while podtemplate remains")
	}
}

func TestDeleteTerminatingKeepsNamespaceWhileReplicationControllerRemains(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.ReplicationController{ObjectMeta: metav1.ObjectMeta{Name: "rc", Namespace: "gone"}},
	)
	client.PrependReactor("delete", "replicationcontrollers", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.Remaining == 0 {
		t.Fatalf("result=%+v", result)
	}
	got, err := client.CoreV1().Namespaces().Get(context.Background(), "gone", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Finalizers) == 0 {
		t.Fatal("finalized while replicationcontroller remains")
	}
}

func TestDeleteTerminatingKeepsNamespaceWhileLeaseRemains(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: "holder", Namespace: "gone"}},
	)
	client.PrependReactor("delete", "leases", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.Remaining == 0 {
		t.Fatalf("result=%+v", result)
	}
	got, err := client.CoreV1().Namespaces().Get(context.Background(), "gone", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Finalizers) == 0 {
		t.Fatal("finalized while lease remains")
	}
}

func TestDeleteTerminatingReportsContentConditionsWhilePodRemains(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "ns", Finalizers: []string{"e2e.example.com/finalizer"}}},
		&v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test-configmap", Namespace: "ns"}},
	)
	client.PrependReactor("delete", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	d := NewDeleter(context.Background(), client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"ns"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Remaining != 1 {
		t.Fatalf("result=%+v", result)
	}
	got, err := client.CoreV1().Namespaces().Get(context.Background(), "ns", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, condition := range got.Status.Conditions {
		if condition.Type == v1.NamespaceDeletionContentFailure {
			found = true
		}
	}
	if !found {
		t.Fatalf("conditions=%+v", got.Status.Conditions)
	}
	if _, err := client.CoreV1().ConfigMaps("ns").Get(context.Background(), "test-configmap", metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestClearCoreKeepsConfigMapWhilePodRemains(t *testing.T) {
	client := fake.NewSimpleClientset(
		&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "ns"}},
		&v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test-configmap", Namespace: "ns"}},
	)
	client.PrependReactor("delete", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	if err := clearCore(context.Background(), client, "ns"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().ConfigMaps("ns").Get(context.Background(), "test-configmap", metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestClearCoreRemovesEventsInOneRequestPerAPI(t *testing.T) {
	var objects []runtime.Object
	for _, name := range []string{"a", "b", "c"} {
		objects = append(objects,
			&v1.Event{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"}},
			&eventsv1.Event{ObjectMeta: metav1.ObjectMeta{Name: "v1-" + name, Namespace: "ns"}},
		)
	}
	client := fake.NewSimpleClientset(objects...)
	if err := clearCore(context.Background(), client, "ns"); err != nil {
		t.Fatal(err)
	}
	perItem, collections := 0, map[string]int{}
	for _, action := range client.Actions() {
		if action.GetResource().Resource != "events" {
			continue
		}
		switch action.GetVerb() {
		case "delete":
			perItem++
		case "delete-collection":
			collections[action.GetResource().Group]++
		}
	}
	if perItem != 0 || collections[""] != 1 || collections["events.k8s.io"] != 1 {
		t.Fatalf("per-item deletes=%d collections=%v, want one delete-collection per events API", perItem, collections)
	}
}
