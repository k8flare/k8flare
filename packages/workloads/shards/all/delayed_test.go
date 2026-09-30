package all

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"

	"github.com/k8flare/k8flare/packages/workloads"
)

func TestSyncBooksTheDeploymentProgressDeadlineWithoutFurtherWrites(t *testing.T) {
	labels := map[string]string{"app": "web"}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "d1"},
		Spec: appsv1.DeploymentSpec{
			Replicas:                ptr.To[int32](1),
			ProgressDeadlineSeconds: ptr.To[int32](3),
			Strategy:                appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector:                &metav1.LabelSelector{MatchLabels: labels},
			Template:                v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}}},
		},
	}
	client := fake.NewSimpleClientset(d)
	assignGeneratedNames(client)
	var next time.Duration
	for i := 0; i < 2; i++ {
		result, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		next = time.Duration(result.NextMs) * time.Millisecond
	}
	if next <= 0 || next > 4*time.Second {
		t.Fatalf("next pass in %s, want within the progress deadline", next)
	}
	time.Sleep(next)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err := client.AppsV1().Deployments("default").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing && c.Status == v1.ConditionFalse && c.Reason == "ProgressDeadlineExceeded" {
			return
		}
	}
	t.Fatalf("conditions = %v", got.Status.Conditions)
}
