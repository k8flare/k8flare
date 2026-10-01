package scheduler

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/kubernetes/pkg/scheduler/backend/queue"
	"k8s.io/utils/ptr"
)

func node(name, cpu string) *v1.Node {
	res := v1.ResourceList{v1.ResourceCPU: resource.MustParse(cpu), v1.ResourceMemory: resource.MustParse("1Gi"), v1.ResourcePods: resource.MustParse("110")}
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"kubernetes.io/hostname": name}},
		Status:     v1.NodeStatus{Capacity: res, Allocatable: res, Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}}},
	}
}

func nodeWithZone(name, cpu, zone string) *v1.Node {
	res := v1.ResourceList{v1.ResourceCPU: resource.MustParse(cpu), v1.ResourceMemory: resource.MustParse("1Gi"), v1.ResourcePods: resource.MustParse("110")}
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"kubernetes.io/hostname": name, "topology.kubernetes.io/zone": zone}},
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

func TestSchedulePreemptsLowerPriority(t *testing.T) {
	lowPrio, highPrio := int32(1), int32(1000)
	low := pod("low", "l", "1")
	low.Spec.Priority = &lowPrio
	low.Spec.NodeName = "n1"
	low.Status.Phase = v1.PodRunning
	high := pod("high", "h", "1")
	high.Spec.Priority = &highPrio
	client := fake.NewSimpleClientset(node("n1", "1"), low, high)
	result, err := Schedule(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	var deleted bool
	for _, a := range client.Actions() {
		if a.GetVerb() == "delete" && a.GetResource().Resource == "pods" {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("expected victim delete, result=%+v actions=%d", result, len(client.Actions()))
	}
}

func TestQueueAttempt(t *testing.T) {
	if _, ok := QueueAttempt(nil); ok {
		t.Fatal("empty")
	}
	n := 2
	attempt, ok := QueueAttempt([]QueueMessage{{Kind: "retry", Attempt: &n}, {Kind: "change"}})
	if !ok || attempt != 0 {
		t.Fatal(attempt, ok)
	}
	attempt, ok = QueueAttempt([]QueueMessage{{Kind: "retry", Attempt: &n}})
	if !ok || attempt != 3 {
		t.Fatal(attempt, ok)
	}
}

func TestKeepUnboundRetriesWhenBindDoesNotReport(t *testing.T) {
	queued := []*v1.Pod{pod("fits", "a", "1"), pod("other", "b", "1")}
	if got := keepUnbound(queued, 1, nil); len(got) != 2 {
		t.Fatalf("unreported = %+v", got)
	}
	if got := keepUnbound(queued, 2, nil); len(got) != 0 {
		t.Fatalf("bound = %+v", got)
	}
	found := []PodRef{{Name: "other"}}
	if got := keepUnbound(queued, 0, found); len(got) != 1 || got[0].Name != "other" {
		t.Fatalf("existing = %+v", got)
	}
}

func TestUnschedulablePodsAreRetriedAtTheUpstreamFlushPeriodOnly(t *testing.T) {
	flush := int(queue.DefaultPodMaxInUnschedulablePodsDuration / time.Second)
	if RetryDelaySeconds(0) != 0 || RetryDelaySeconds(1) != flush || RetryDelaySeconds(2) != flush {
		t.Fatal(RetryDelaySeconds(0), RetryDelaySeconds(1), RetryDelaySeconds(2), flush)
	}
}

func TestTopologySpreadWithReplicaSetOwner(t *testing.T) {
	n1 := nodeWithZone("n1", "3", "zone-a")
	n2 := nodeWithZone("n2", "3", "zone-b")

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "rs1", Namespace: "default"},
		Spec: appsv1.ReplicaSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			Replicas: ptr.To(int32(2)),
		},
	}

	existing := pod("rs-pod-1", "1", "100m")
	existing.Labels = map[string]string{"app": "test"}
	existing.Spec.NodeName = "n2"
	existing.Status.Phase = v1.PodRunning
	existing.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "rs1",
		UID:        "rs-uid-1",
	}}

	filler := pod("other-pod", "2", "100m")
	filler.Labels = map[string]string{"app": "other"}
	filler.Spec.NodeName = "n1"
	filler.Status.Phase = v1.PodRunning

	newPod := pod("rs-pod-2", "3", "100m")
	newPod.Labels = map[string]string{"app": "test"}
	newPod.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "rs1",
		UID:        "rs-uid-1",
	}}

	client := fake.NewSimpleClientset(n1, n2, rs, existing, filler, newPod)
	result, err := Schedule(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}

	if result.Bound != 1 {
		t.Fatalf("expected 1 bound pod, got %d: %+v", result.Bound, result)
	}

	var boundNode string
	for _, a := range client.Actions() {
		if a.GetSubresource() == "binding" && a.GetResource().Resource == "pods" {
			obj := a.(clienttesting.CreateAction)
			if binding, ok := obj.GetObject().(*v1.Binding); ok {
				boundNode = binding.Target.Name
				break
			}
		}
	}

	if boundNode == "" {
		t.Fatal("no binding found")
	}

	if boundNode != "n1" {
		t.Fatalf("pod bound to %q, expected n1 (different zone from existing ReplicaSet pod on n2). With ReplicaSet lister, PodTopologySpread default constraints should spread pods across zones", boundNode)
	}
}
