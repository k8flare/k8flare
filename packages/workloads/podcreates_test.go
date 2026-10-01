package workloads

import (
	"context"
	"strconv"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func freshPodCreateLimiter(t *testing.T) {
	t.Helper()
	previous := podCreates
	podCreates = newPodCreateLimiter()
	t.Cleanup(func() { podCreates = previous })
}

func numberedPod(i int, owner *metav1.OwnerReference) *corev1.Pod {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p" + strconv.Itoa(i), Namespace: "default"}}
	if owner != nil {
		pod.OwnerReferences = []metav1.OwnerReference{*owner}
	}
	return pod
}

func podCreatesSeenBy(client *fake.Clientset) int {
	n := 0
	for _, a := range client.Actions() {
		if a.GetVerb() == "create" && a.GetResource().Resource == "pods" {
			n++
		}
	}
	return n
}

func TestPodCreatesAreSpacedOnceTheBurstIsSpent(t *testing.T) {
	freshPodCreateLimiter(t)
	client := fake.NewSimpleClientset()
	pods := (&observeClient{Interface: client}).CoreV1().Pods("default")
	for i := 0; i < podCreateBurst; i++ {
		if _, err := pods.Create(context.Background(), numberedPod(i, nil), metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	const extra = 4
	started := time.Now()
	for i := 0; i < extra; i++ {
		if _, err := pods.Create(context.Background(), numberedPod(podCreateBurst+i, nil), metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	atLeast := time.Duration(float64(extra-1) / podCreatesPerSecond * float64(time.Second))
	if took := time.Since(started); took < atLeast {
		t.Fatalf("%d creates past the burst took %s, want at least %s at %v per second", extra, took, atLeast, podCreatesPerSecond)
	}
	if got := podCreatesSeenBy(client); got != podCreateBurst+extra {
		t.Fatalf("creates = %d", got)
	}
}

func TestPodCreateGivesUpWhenThePassIsCancelledWhileItWaits(t *testing.T) {
	freshPodCreateLimiter(t)
	client := fake.NewSimpleClientset()
	pods := (&observeClient{Interface: client}).CoreV1().Pods("default")
	for i := 0; i < podCreateBurst; i++ {
		if _, err := pods.Create(context.Background(), numberedPod(i, nil), metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() {
		_, err := pods.Create(ctx, numberedPod(podCreateBurst, nil), metav1.CreateOptions{})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a create issued for a cancelled pass went through")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a create for a cancelled pass kept waiting for the limiter")
	}
	if got := podCreatesSeenBy(client); got != podCreateBurst {
		t.Fatalf("creates = %d, want %d", got, podCreateBurst)
	}
}

func TestPodCreateIsRefusedOnceItsControllerLeftTheSnapshot(t *testing.T) {
	freshPodCreateLimiter(t)
	client := fake.NewSimpleClientset()
	replicaSets := newSnapshotInformer(&appsv1.ReplicaSet{})
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "rs1"}}
	replicaSets.fill([]runtime.Object{rs})
	pods := (&observeClient{Interface: client, replicaSets: replicaSets}).CoreV1().Pods("default")
	owned := func(uid types.UID) *metav1.OwnerReference {
		return &metav1.OwnerReference{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "web", UID: uid, Controller: ptr.To(true)}
	}
	job := &metav1.OwnerReference{APIVersion: "batch/v1", Kind: "Job", Name: "once", UID: "j1", Controller: ptr.To(true)}

	if _, err := pods.Create(context.Background(), numberedPod(0, owned("rs1")), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create for a ReplicaSet in the snapshot: %v", err)
	}
	if _, err := pods.Create(context.Background(), numberedPod(1, job), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create for a kind this pass did not list: %v", err)
	}
	if _, err := pods.Create(context.Background(), numberedPod(2, owned("recreated")), metav1.CreateOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("create for a ReplicaSet replaced under the same name: %v", err)
	}
	replicaSets.forget("default", "web")
	if _, err := pods.Create(context.Background(), numberedPod(3, owned("rs1")), metav1.CreateOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("create for a ReplicaSet that left the snapshot: %v", err)
	}
	if got := podCreatesSeenBy(client); got != 2 {
		t.Fatalf("creates = %d, want 2", got)
	}
}
