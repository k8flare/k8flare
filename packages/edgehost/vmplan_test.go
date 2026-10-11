package edgehost

import "testing"

func TestPlanVMsBootsNewPod(t *testing.T) {
	pod := containersPod("default", "web", "uid-1", "node-a", "small")
	plan := PlanVMs(vmPlanIn{NowMs: 1_000, Pending: []vmPod{pod}})
	if len(plan.Boots) != 1 || plan.Boots[0].Claimed || plan.Boots[0].NodeName != "node-a" || plan.Boots[0].Tier != "small" {
		t.Fatalf("%+v", plan.Boots)
	}
	if len(plan.Teardowns) != 0 || plan.NextMs != nodeReadyTimeoutMs {
		t.Fatalf("%+v", plan)
	}
}

func TestPlanVMsReapsFailedAndReady(t *testing.T) {
	failed := containersPod("default", "old", "uid-old", "node-old", "small")
	failed.Status.Phase = "Failed"
	live := containersPod("default", "live", "uid-live", "node-live", "medium")
	slow := containersPod("default", "slow", "uid-slow", "node-slow", "large")
	node := vmPod{}
	node.Status.Conditions = []struct {
		Type   string `json:"type"`
		Status string `json:"status"`
	}{{Type: "Ready", Status: "True"}}
	plan := PlanVMs(vmPlanIn{
		NowMs: 400_000,
		Tracked: map[string]vmTrack{
			"uid-old":  {Namespace: "default", PodName: "old", PodUID: "uid-old", NodeName: "node-old", Tier: "small", BootedAt: 1},
			"uid-live": {Namespace: "default", PodName: "live", PodUID: "uid-live", NodeName: "node-live", Tier: "medium", BootedAt: 1},
			"uid-slow": {Namespace: "default", PodName: "slow", PodUID: "uid-slow", NodeName: "node-slow", Tier: "large", BootedAt: 1},
		},
		Pods: map[string]*vmPod{
			"default/old":  &failed,
			"default/live": &live,
			"default/slow": &slow,
		},
		Nodes: map[string]*vmPod{"node-live": &node},
	})
	if len(plan.Teardowns) != 2 || len(plan.MarkBound) != 1 || plan.MarkBound[0] != "uid-live" || plan.NextMs != 0 {
		t.Fatalf("%+v", plan)
	}
}

func containersPod(ns, name, uid, node, tier string) vmPod {
	var pod vmPod
	pod.Metadata.Namespace = ns
	pod.Metadata.Name = name
	pod.Metadata.UID = uid
	pod.Metadata.Annotations = map[string]string{"k8flare.com/nodevm-tier": tier}
	pod.Spec.NodeSelector = map[string]string{
		"k8flare.com/backend":    "containers",
		"kubernetes.io/hostname": node,
	}
	pod.Status.Phase = "Pending"
	return pod
}

func TestPlanVMsBooksTheNodeReadyTimeoutOfTheOldestUnboundVMOnly(t *testing.T) {
	waiting := containersPod("default", "waiting", "uid-waiting", "node-waiting", "small")
	later := containersPod("default", "later", "uid-later", "node-later", "small")
	running := containersPod("default", "running", "uid-running", "node-running", "small")
	running.Status.Phase = "Running"
	plan := PlanVMs(vmPlanIn{
		NowMs: 100_000,
		Tracked: map[string]vmTrack{
			"uid-waiting": {Namespace: "default", PodName: "waiting", PodUID: "uid-waiting", NodeName: "node-waiting", Tier: "small", Started: true, BootedAt: 40_000},
			"uid-later":   {Namespace: "default", PodName: "later", PodUID: "uid-later", NodeName: "node-later", Tier: "small", Started: true, BootedAt: 90_000},
			"uid-running": {Namespace: "default", PodName: "running", PodUID: "uid-running", NodeName: "node-running", Tier: "small", Started: true, Bound: true, BootedAt: 1},
		},
		Pending: []vmPod{waiting, later},
		Pods:    map[string]*vmPod{"default/waiting": &waiting, "default/later": &later, "default/running": &running},
	})
	if len(plan.Boots) != 0 || len(plan.Teardowns) != 0 || plan.NextMs != 240_000 {
		t.Fatalf("%+v, want nothing to do until the waiting VM's node-ready timeout in 240s", plan)
	}
	idle := PlanVMs(vmPlanIn{
		NowMs:   100_000,
		Tracked: map[string]vmTrack{"uid-running": {Namespace: "default", PodName: "running", PodUID: "uid-running", NodeName: "node-running", Tier: "small", Started: true, Bound: true, BootedAt: 1}},
		Pods:    map[string]*vmPod{"default/running": &running},
	})
	if idle.NextMs != 0 {
		t.Fatalf("%+v, want nothing booked for a running pod: its deletion is a write that wakes the scheduler", idle)
	}
}
