package quota

import (
	"context"
	"github.com/k8flare/k8flare/packages/workloads"
	_ "github.com/k8flare/k8flare/packages/workloads/shards/all"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSyncResourceQuotaCalculatesStatus(t *testing.T) {
	rq := &v1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "default"},
		Spec: v1.ResourceQuotaSpec{
			Hard: v1.ResourceList{v1.ResourceQuotas: resource.MustParse("2")},
		},
		Status: v1.ResourceQuotaStatus{
			Hard: v1.ResourceList{v1.ResourceQuotas: resource.MustParse("9")},
		},
	}
	client := fake.NewSimpleClientset(rq)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"resourcequotas"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().ResourceQuotas("default").Get(context.Background(), "q", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	hard := got.Status.Hard[v1.ResourceQuotas]
	used := got.Status.Used[v1.ResourceQuotas]
	if hard.Cmp(resource.MustParse("2")) != 0 {
		t.Fatalf("status.hard resourcequotas = %s", hard.String())
	}
	if used.Cmp(resource.MustParse("1")) != 0 {
		t.Fatalf("status.used resourcequotas = %s", used.String())
	}
}

func TestSyncResourceQuotaCountsReplicaSets(t *testing.T) {
	zero := int32(0)
	rq := &v1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "default"},
		Spec: v1.ResourceQuotaSpec{
			Hard: v1.ResourceList{v1.ResourceName("count/replicasets.apps"): resource.MustParse("2")},
		},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "rs", Namespace: "default"},
		Spec:       appsv1.ReplicaSetSpec{Replicas: &zero},
	}
	client := fake.NewSimpleClientset(rq, rs)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"resourcequotas", "replicasets"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().ResourceQuotas("default").Get(context.Background(), "q", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	used := got.Status.Used[v1.ResourceName("count/replicasets.apps")]
	if used.Cmp(resource.MustParse("1")) != 0 {
		t.Fatalf("status.used count/replicasets.apps = %s status=%v", used.String(), got.Status.Used)
	}
}
