package all

import (
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"

	"github.com/k8flare/k8flare/packages/workloads"
)

func TestSyncBooksTheTolerationSecondsOfAUserNoExecuteTaint(t *testing.T) {
	added := metav1.Now()
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Spec:       v1.NodeSpec{Taints: []v1.Taint{{Key: "maintenance", Effect: v1.TaintEffectNoExecute, TimeAdded: &added}}},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default", UID: "p1"},
		Spec: v1.PodSpec{
			NodeName:    "n1",
			Tolerations: []v1.Toleration{{Key: "maintenance", Operator: v1.TolerationOpExists, Effect: v1.TaintEffectNoExecute, TolerationSeconds: ptr.To[int64](30)}},
		},
	}
	client := fake.NewSimpleClientset(node, pod)
	result, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	next := time.Duration(result.NextMs) * time.Millisecond
	if next <= 0 || next > 30*time.Second {
		t.Fatalf("next pass in %s, want within the toleration", next)
	}
}
