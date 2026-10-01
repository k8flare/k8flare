package all

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/k8flare/k8flare/packages/workloads"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func storeDeleted(t *testing.T, key string, obj runtime.Object) []byte {
	t.Helper()
	value, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	event, err := json.Marshal(map[string]any{"rev": 9000, "type": "deleted", "key": key, "value": base64.StdEncoding.EncodeToString(value)})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func TestSyncStopsCreatingPodsForAReplicaSetDeletedDuringThePass(t *testing.T) {
	labels := map[string]string{"app": "marker"}
	rs := &appsv1.ReplicaSet{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "ReplicaSet"},
		ObjectMeta: metav1.ObjectMeta{Name: "marker-deployment-5d4f", Namespace: "default", UID: "rs1"},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: ptr.To[int32](1337),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}}},
		},
	}
	client := fake.NewSimpleClientset(rs)
	assignGeneratedNames(client)

	events := make(chan []byte, 1)
	previous := workloads.WatchDialer
	workloads.WatchDialer = func(context.Context, string) (<-chan []byte, func(), error) { return events, func() {}, nil }
	defer func() { workloads.WatchDialer = previous }()

	var creates atomic.Int32
	var collected sync.Once
	client.PrependReactor("create", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		if creates.Add(1) == 10 {
			collected.Do(func() {
				_ = client.Tracker().Delete(appsv1.SchemeGroupVersion.WithResource("replicasets"), rs.Namespace, rs.Name)
				events <- storeDeleted(t, "/registry/replicasets/default/"+rs.Name, rs)
			})
		}
		return false, nil, nil
	})

	started := time.Now()
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"replicasets"}); err != nil {
		t.Fatal(err)
	}
	if got := creates.Load(); got >= 100 {
		t.Fatalf("pod creates = %d after the ReplicaSet was deleted at the 10th, want a small number", got)
	}
	if took := time.Since(started); took > 30*time.Second {
		t.Fatalf("the pass kept running for %s after the ReplicaSet was deleted", took)
	}
}

func TestSyncStopsCreatingPodsOnceTheDeploymentBehindTheReplicaSetIsDeleted(t *testing.T) {
	labels := map[string]string{"app": "marker"}
	d := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "marker-deployment", Namespace: "default", UID: "d1"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1337),
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}}},
		},
	}
	client := fake.NewSimpleClientset(d)
	assignGeneratedNames(client)

	events := make(chan []byte, 1)
	previous := workloads.WatchDialer
	workloads.WatchDialer = func(context.Context, string) (<-chan []byte, func(), error) { return events, func() {}, nil }
	defer func() { workloads.WatchDialer = previous }()

	var creates atomic.Int32
	var deleted sync.Once
	client.PrependReactor("create", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		if creates.Add(1) == 10 {
			deleted.Do(func() {
				_ = client.Tracker().Delete(appsv1.SchemeGroupVersion.WithResource("deployments"), d.Namespace, d.Name)
				events <- storeDeleted(t, "/registry/deployments/default/"+d.Name, d)
			})
		}
		return false, nil, nil
	})

	started := time.Now()
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"deployments"}); err != nil {
		t.Fatal(err)
	}
	if got := creates.Load(); got < 10 || got >= 100 {
		t.Fatalf("pod creates = %d with the Deployment deleted at the 10th, want a small number", got)
	}
	if took := time.Since(started); took > 30*time.Second {
		t.Fatalf("the pass kept running for %s after the Deployment was deleted", took)
	}
}
