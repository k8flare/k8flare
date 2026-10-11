package workloads

import (
	"context"
	"strconv"
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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/metadata"
	metadatafake "k8s.io/client-go/metadata/fake"
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

func TestDeleteTerminatingTakesHintedThenListed(t *testing.T) {
	now := metav1.Now()
	old := metav1.NewTime(now.Add(-time.Hour))
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "stale", DeletionTimestamp: &old}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "fresh", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "fresh"}},
		&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "old-pod", Namespace: "stale"}},
	)
	d := NewDeleter(client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"fresh"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Terminating != 2 || result.Deleted != 2 || len(result.Names) != 0 {
		t.Fatalf("result=%+v", result)
	}
	for ns, pod := range map[string]string{"fresh": "test-pod", "stale": "old-pod"} {
		if _, err := client.CoreV1().Pods(ns).Get(context.Background(), pod, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Fatalf("%s pod still present: %v", ns, err)
		}
	}
}

func TestDeleteTerminatingRemovesPods(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "gone"}},
	)
	d := NewDeleter(client, nil)
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
	d := NewDeleter(client, nil)
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

func TestDeleteTerminatingClearsUnhintedInTheSameCall(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "left", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
	)
	d := NewDeleter(client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 2 || len(result.Names) != 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestDeleteTerminatingStopsAtTheBudgetAndReportsTheRest(t *testing.T) {
	saved := namespaceBudget
	namespaceBudget = 0
	defer func() { namespaceBudget = saved }()
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "a", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "b", DeletionTimestamp: &now}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
	)
	d := NewDeleter(client, nil)
	result, err := d.DeleteTerminating(context.Background(), client, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.Remaining != 2 || len(result.Names) != 2 || result.NextMs <= 0 {
		t.Fatalf("result=%+v", result)
	}
	for _, ns := range []string{"a", "b"} {
		if _, err := client.CoreV1().Namespaces().Get(context.Background(), ns, metav1.GetOptions{}); err != nil {
			t.Fatalf("namespace %s should be untouched: %v", ns, err)
		}
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
	d := NewDeleter(client, nil)
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

func stuckNamespaceRetry(t *testing.T, deletedAt time.Time) time.Duration {
	t.Helper()
	at := metav1.NewTime(deletedAt)
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "stuck", DeletionTimestamp: &at}, Spec: v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}}},
		&v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "held", Namespace: "stuck"}},
	)
	client.PrependReactor("delete", "services", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	result, err := NewDeleter(client, nil).DeleteTerminating(context.Background(), client, []string{"stuck"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.Remaining != 1 {
		t.Fatalf("result=%+v", result)
	}
	return time.Duration(result.NextMs) * time.Millisecond
}

func TestDeleteTerminatingRetriesAJustDeletedNamespaceIn2s(t *testing.T) {
	if got := stuckNamespaceRetry(t, time.Now()); got != namespaceRetry {
		t.Fatalf("retry in %s, want %s", got, namespaceRetry)
	}
}

func TestDeleteTerminatingBacksOffANamespaceThatCannotFinishToOncePerMinute(t *testing.T) {
	deletedAt := time.Now().Add(-20 * time.Second)
	earliest := time.Since(deletedAt).Truncate(time.Millisecond)
	got := stuckNamespaceRetry(t, deletedAt)
	if latest := time.Since(deletedAt); got < earliest || got > latest {
		t.Fatalf("retry in %s after 20s of trying, want the time since the deletion, %s to %s", got, earliest, latest)
	}
	if got := stuckNamespaceRetry(t, time.Now().Add(-10*time.Minute)); got != time.Minute {
		t.Fatalf("retry in %s after 10m of trying, want 1m", got)
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
	d := NewDeleter(client, nil)
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
	d := NewDeleter(client, nil)
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
	d := NewDeleter(client, nil)
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
	d := NewDeleter(client, nil)
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
	d := NewDeleter(client, nil)
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
	d := NewDeleter(client, nil)
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
	d := NewDeleter(client, nil)
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
	d := NewDeleter(client, nil)
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
	d := NewDeleter(client, nil)
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
	d := NewDeleter(client, nil)
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

type preferredDiscovery struct{ *fakediscovery.FakeDiscovery }

func (d preferredDiscovery) ServerPreferredNamespacedResources() ([]*metav1.APIResourceList, error) {
	return discovery.ServerPreferredNamespacedResources(d.FakeDiscovery)
}

type discoveringClientset struct{ *fake.Clientset }

func (c discoveringClientset) Discovery() discovery.DiscoveryInterface {
	return preferredDiscovery{c.Clientset.Discovery().(*fakediscovery.FakeDiscovery)}
}

type slowMetadata struct {
	metadata.Interface
	delay time.Duration
}

func (m slowMetadata) Resource(gvr schema.GroupVersionResource) metadata.Getter {
	return slowGetter{m.Interface.Resource(gvr), m.delay}
}

type slowGetter struct {
	metadata.Getter
	delay time.Duration
}

func (g slowGetter) Namespace(ns string) metadata.ResourceInterface {
	return slowResource{g.Getter.Namespace(ns), g.delay}
}

type slowResource struct {
	metadata.ResourceInterface
	delay time.Duration
}

func (r slowResource) wait(ctx context.Context) error {
	select {
	case <-time.After(r.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r slowResource) List(ctx context.Context, opts metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
	if err := r.wait(ctx); err != nil {
		return nil, err
	}
	return r.ResourceInterface.List(ctx, opts)
}

func (r slowResource) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	if err := r.wait(ctx); err != nil {
		return err
	}
	return r.ResourceInterface.DeleteCollection(ctx, opts, listOpts)
}

func namespacedResources(groupVersion string, names ...string) *metav1.APIResourceList {
	list := &metav1.APIResourceList{GroupVersion: groupVersion}
	for _, name := range names {
		list.APIResources = append(list.APIResources, metav1.APIResource{Name: name, Namespaced: true, Verbs: metav1.Verbs{"create", "delete", "deletecollection", "get", "list"}})
	}
	return list
}

func customResourceName(i int) string {
	return "kind" + strconv.Itoa(i) + "s"
}

func slowCluster(t *testing.T, delay time.Duration, customTypes int, objects ...runtime.Object) (discoveringClientset, *metadatafake.FakeMetadataClient, metadata.Interface) {
	t.Helper()
	now := metav1.Now()
	client := discoveringClientset{fake.NewSimpleClientset(&v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "gone", DeletionTimestamp: &now},
		Spec:       v1.NamespaceSpec{Finalizers: []v1.FinalizerName{v1.FinalizerKubernetes}},
		Status:     v1.NamespaceStatus{Phase: v1.NamespaceTerminating},
	})}
	client.PrependReactor("*", "*", func(clienttesting.Action) (bool, runtime.Object, error) {
		time.Sleep(delay)
		return false, nil, nil
	})
	var custom []string
	for i := range customTypes {
		custom = append(custom, customResourceName(i))
	}
	client.Resources = []*metav1.APIResourceList{
		namespacedResources("v1", "pods", "configmaps", "secrets", "serviceaccounts", "services", "endpoints", "events", "podtemplates", "replicationcontrollers", "persistentvolumeclaims", "resourcequotas", "limitranges"),
		namespacedResources("apps/v1", "deployments", "replicasets", "statefulsets", "daemonsets", "controllerrevisions"),
		namespacedResources("batch/v1", "jobs", "cronjobs"),
		namespacedResources("rbac.authorization.k8s.io/v1", "roles", "rolebindings"),
		namespacedResources("coordination.k8s.io/v1", "leases"),
		namespacedResources("discovery.k8s.io/v1", "endpointslices"),
		namespacedResources("networking.k8s.io/v1", "networkpolicies", "ingresses"),
		namespacedResources("policy/v1", "poddisruptionbudgets"),
		namespacedResources("autoscaling/v1", "horizontalpodautoscalers"),
		namespacedResources("events.k8s.io/v1", "events"),
		namespacedResources("resource.k8s.io/v1", "resourceclaims", "resourceclaimtemplates"),
		namespacedResources("storage.k8s.io/v1", "csistoragecapacities"),
		namespacedResources("example.com/v1", custom...),
	}
	scheme := metadatafake.NewTestScheme()
	if err := metav1.AddMetaToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	meta := metadatafake.NewSimpleMetadataClient(scheme, objects...)
	meta.PrependReactor("delete-collection", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		gvr, ns := action.GetResource(), action.GetNamespace()
		list, err := meta.Tracker().List(gvr, schema.GroupVersionKind{Group: "fake-metadata-client-group", Version: "v1"}, ns)
		if err != nil {
			return true, nil, err
		}
		for _, item := range list.(*metav1.List).Items {
			if err := meta.Tracker().Delete(gvr, ns, item.Object.(*metav1.PartialObjectMetadata).Name); err != nil {
				return true, nil, err
			}
		}
		return true, nil, nil
	})
	return client, meta, slowMetadata{meta, delay}
}

func TestDeleteTerminatingFinishesWithinTheBudgetWhateverTheNumberOfEmptyCustomResourceTypes(t *testing.T) {
	saved := namespaceBudget
	namespaceBudget = 2 * time.Second
	defer func() { namespaceBudget = saved }()
	for _, customTypes := range []int{0, 40} {
		client, _, meta := slowCluster(t, 10*time.Millisecond, customTypes)
		start := time.Now()
		result, err := NewDeleter(client, meta).DeleteTerminating(context.Background(), client, []string{"gone"})
		elapsed := time.Since(start)
		t.Logf("%d empty custom resource types: one deletion took %s", customTypes, elapsed.Round(time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		if result.Deleted != 1 {
			t.Fatalf("%d empty custom resource types: result=%+v after %s, budget %s", customTypes, result, elapsed, namespaceBudget)
		}
	}
}

func TestDeleteTerminatingDoesNotSweepACustomResourceTypeAnEarlierPassEmptied(t *testing.T) {
	saved := namespaceBudget
	namespaceBudget = 1200 * time.Millisecond
	defer func() { namespaceBudget = saved }()
	var objects []runtime.Object
	for i := range 40 {
		objects = append(objects, &metav1.PartialObjectMetadata{
			TypeMeta:   metav1.TypeMeta{APIVersion: "example.com/v1", Kind: "Kind" + strconv.Itoa(i)},
			ObjectMeta: metav1.ObjectMeta{Name: "item", Namespace: "gone"},
		})
	}
	client, fakeMeta, meta := slowCluster(t, 10*time.Millisecond, 40, objects...)
	d := NewDeleter(client, meta)
	passes := 0
	for passes < 10 {
		passes++
		result, err := d.DeleteTerminating(context.Background(), client, []string{"gone"})
		if err != nil {
			t.Fatal(err)
		}
		if result.Deleted == 1 {
			break
		}
	}
	if passes < 2 {
		t.Fatalf("finished in %d pass, the budget should have cut the first one", passes)
	}
	sweeps := map[string]int{}
	for _, action := range fakeMeta.Actions() {
		if action.GetVerb() == "delete-collection" && action.GetResource().Group == "example.com" {
			sweeps[action.GetResource().Resource]++
		}
	}
	for i := range 40 {
		if n := sweeps[customResourceName(i)]; n != 1 {
			t.Fatalf("%s swept %d times over %d passes, want once: %v", customResourceName(i), n, passes, sweeps)
		}
	}
}
