package edgehost

import "testing"

func TestFollowUpSchedulerFailure(t *testing.T) {
	got := followUp(followIn{Target: "scheduler"})
	if len(got.Sends) != 1 || got.Sends[0].Queue != "sched" || got.Sends[0].DelaySeconds != 5 || got.Sends[0].Attempt == nil || *got.Sends[0].Attempt != 0 {
		t.Fatal(got)
	}
}

func TestFollowUpGCSettles(t *testing.T) {
	got := followUp(followIn{Target: "gc", HasResult: true, Deleted: 1})
	if len(got.Sends) != 1 || got.Sends[0].DelaySeconds != 2 {
		t.Fatal(got)
	}
	idle := followUp(followIn{Target: "gc", HasResult: true})
	if len(idle.Sends) != 0 {
		t.Fatal(idle)
	}
}

func TestFollowUpAccountsStopsWhileTerminating(t *testing.T) {
	got := followUp(followIn{Target: "accounts", HasResult: true, NextMs: 1500, Names: []string{"gone"}, Remaining: 1})
	if !got.Stop || len(got.Sends) != 1 || got.Sends[0].DelaySeconds != 2 || len(got.Sends[0].Names) != 1 {
		t.Fatal(got)
	}
}

func TestFollowUpAddonsRetriesUntilDeployed(t *testing.T) {
	failed := followUp(followIn{Target: "addons"})
	if len(failed.Sends) != 1 || failed.Sends[0].Queue != "addons" || failed.Sends[0].DelaySeconds != 5 {
		t.Fatal(failed)
	}
	done := followUp(followIn{Target: "addons", OK: true})
	if len(done.Sends) != 0 {
		t.Fatal(done)
	}
}

func TestFollowUpAddonsResumesAPendingStorageMigrationAlone(t *testing.T) {
	got := followUp(followIn{Target: "addons", OK: true, Pending: 3})
	if len(got.Sends) != 1 || got.Sends[0].Queue != "addons" || got.Sends[0].DelaySeconds != 5 || len(got.Sends[0].Names) != 1 || got.Sends[0].Names[0] != "storage-migrate" {
		t.Fatal(got)
	}
}

func TestFollowUpHPAReschedulesWhenHPAsExist(t *testing.T) {
	got := followUp(followIn{Target: "hpa", HasResult: true, HPAs: 2})
	if len(got.Sends) != 1 || got.Sends[0].Queue != "hpa" || got.Sends[0].DelaySeconds != 15 || !got.Sends[0].Once {
		t.Fatal(got)
	}
}

func TestFollowUpHPADoesNotRescheduleWhenZeroHPAs(t *testing.T) {
	got := followUp(followIn{Target: "hpa", HasResult: true, HPAs: 0})
	if len(got.Sends) != 0 {
		t.Fatal(got)
	}
}

func TestFollowUpHPARetriesOnFailure(t *testing.T) {
	got := followUp(followIn{Target: "hpa", HasResult: false})
	if len(got.Sends) != 1 || got.Sends[0].Queue != "hpa" || got.Sends[0].DelaySeconds != 5 || got.Sends[0].Once {
		t.Fatal(got)
	}
}

func TestFollowUpWorkloadsProvisionAndSync(t *testing.T) {
	got := followUp(followIn{Target: "workloads", PlanOK: true, ProvisionFailed: true, Changed: []string{"services"}, HasResult: true, Drained: true, NextMs: 1000})
	if len(got.Sends) != 2 || got.Sends[0].DelaySeconds != 5 || got.Sends[1].DelaySeconds != 1 {
		t.Fatal(got)
	}
}
