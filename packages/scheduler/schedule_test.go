package scheduler

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func node(name, cpu string) *v1.Node {
	res := v1.ResourceList{v1.ResourceCPU: resource.MustParse(cpu), v1.ResourceMemory: resource.MustParse("1Gi"), v1.ResourcePods: resource.MustParse("110")}
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"kubernetes.io/hostname": name}},
		Status:     v1.NodeStatus{Capacity: res, Allocatable: res, Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}}},
	}
}

func pod(name, uid, cpu string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID("uid-" + uid)},
		Spec:       v1.PodSpec{SchedulerName: "default-scheduler", Containers: []v1.Container{{Name: "c", Image: "i", Resources: v1.ResourceRequirements{Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse(cpu)}}}}},
		Status:     v1.PodStatus{Phase: v1.PodPending},
	}
}

func TestScheduleBindsAndReportsUnschedulable(t *testing.T) {
	client := fake.NewSimpleClientset(node("n1", "1"), pod("fits", "a", "500m"), pod("huge", "b", "4"))
	result, err := Schedule(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Bound != 1 {
		t.Fatalf("bound = %d, want 1 (%+v)", result.Bound, result)
	}
	if len(result.Unschedulable) != 1 || result.Unschedulable[0].Name != "huge" {
		t.Fatalf("unschedulable = %+v", result.Unschedulable)
	}
	var bindings int
	for _, a := range client.Actions() {
		if a.GetSubresource() == "binding" {
			bindings++
		}
	}
	if bindings != 1 {
		t.Fatalf("bindings = %d", bindings)
	}
}
