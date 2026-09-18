package workloads

import (
	"context"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func creates(client *fake.Clientset, resource string) int {
	n := 0
	for _, a := range client.Actions() {
		if a.GetVerb() == "create" && a.GetResource().Resource == resource {
			n++
		}
	}
	return n
}

func TestSyncCreatesReplicaSetThenPods(t *testing.T) {
	labels := map[string]string{"app": "web"}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "d1"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](2),
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}}},
		},
	}
	client := fake.NewSimpleClientset(d)
	uids := 0
	client.PrependReactor("create", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if obj, err := meta.Accessor(action.(k8stesting.CreateAction).GetObject()); err == nil && obj.GetUID() == "" {
			uids++
			obj.SetUID(types.UID(fmt.Sprintf("uid-%d", uids)))
			if obj.GetName() == "" {
				obj.SetName(fmt.Sprintf("%s%d", obj.GetGenerateName(), uids))
			}
		}
		return false, nil, nil
	})
	rsCreates, podCreates := 0, 0
	for i := 0; i < 4; i++ {
		client.ClearActions()
		if _, err := Sync(context.Background(), client, []byte("ca"), nil); err != nil {
			t.Fatal(err)
		}
		rsCreates += creates(client, "replicasets")
		podCreates += creates(client, "pods")
	}
	if rsCreates != 1 || podCreates != 2 {
		t.Fatalf("replicaset creates = %d, pod creates = %d", rsCreates, podCreates)
	}
	client.ClearActions()
	if _, err := Sync(context.Background(), client, []byte("ca"), nil); err != nil {
		t.Fatal(err)
	}
	if got := creates(client, "pods") + creates(client, "replicasets"); got != 0 {
		t.Fatalf("creates on settled state = %d", got)
	}
}

func TestWantedSelectsControllersForChangedResources(t *testing.T) {
	controllers, needed := wanted([]string{"deployments"})
	if !controllers["deployment"] || controllers["job"] {
		t.Fatalf("controllers = %v", controllers)
	}
	for _, want := range []string{"pods", "replicasets", "deployments"} {
		if !needed[want] {
			t.Fatalf("needed is missing %s: %v", want, needed)
		}
	}
	if needed["cronjobs"] {
		t.Fatalf("needed should not include cronjobs: %v", needed)
	}
	all, none := wanted(nil)
	if len(all) != len(controllerNeeds) || none != nil {
		t.Fatalf("an empty batch must run everything: %d %v", len(all), none)
	}
}
