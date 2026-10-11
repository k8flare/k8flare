package edgehost

import (
	"slices"
	"testing"
)

func TestFollowUpSchedulerFailure(t *testing.T) {
	got := followUp(followIn{Target: "scheduler"})
	if len(got.Sends) != 1 || got.Sends[0].Queue != "sched" || got.Sends[0].DelaySeconds != 5 || got.Sends[0].Attempt == nil || *got.Sends[0].Attempt != 0 {
		t.Fatal(got)
	}
}

func TestFollowUpSchedulerBooksOnlyTheUnschedulableFlush(t *testing.T) {
	idle := followUp(followIn{Target: "scheduler", OK: true})
	if len(idle.Sends) != 0 {
		t.Fatal(idle)
	}
	waiting := followUp(followIn{Target: "scheduler", OK: true, Attempt: 2, RetryAfterS: 300})
	if len(waiting.Sends) != 1 || waiting.Sends[0].Queue != "sched" || waiting.Sends[0].DelaySeconds != 300 {
		t.Fatal(waiting)
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

func TestFollowUpAddonsBooksOneWakeAtTheHelmHookDeadline(t *testing.T) {
	got := followUp(followIn{Target: "addons", OK: true, NextMs: 299_001})
	if len(got.Sends) != 1 || got.Sends[0].Queue != "addons" || got.Sends[0].DelaySeconds != 300 || len(got.Sends[0].Names) != 0 {
		t.Fatal(got)
	}
}

func TestFollowUpAddonsClearsTheHelmHookWakeOnlyAfterAHelmPassWithNoHookPending(t *testing.T) {
	for _, tc := range []struct {
		in   followIn
		want []string
	}{
		{followIn{Target: "addons", OK: true}, []string{"addons"}},
		{followIn{Target: "addons", OK: true, Pending: 3}, []string{"addons"}},
		{followIn{Target: "addons", OK: true, NextMs: 299_001}, nil},
		{followIn{Target: "addons", OK: true, Skip: true}, nil},
		{followIn{Target: "addons"}, nil},
	} {
		if got := followUp(tc.in); !slices.Equal(got.Cancel, tc.want) {
			t.Errorf("%+v: cancel = %v, want %v", tc.in, got.Cancel, tc.want)
		}
	}
}

func TestFollowUpHPAReschedulesWhenHPAsExist(t *testing.T) {
	got := followUp(followIn{Target: "hpa", HasResult: true, HPAs: 2})
	if len(got.Sends) != 1 || got.Sends[0].Queue != "hpa" || got.Sends[0].DelaySeconds != 15 {
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
	if len(got.Sends) != 1 || got.Sends[0].Queue != "hpa" || got.Sends[0].DelaySeconds != 5 {
		t.Fatal(got)
	}
}

func TestFollowUpWorkloadsProvisionAndSync(t *testing.T) {
	got := followUp(followIn{Target: "workloads", PlanOK: true, ProvisionFailed: true, Changed: []string{"services"}, HasResult: true, Drained: true, NextMs: 1000})
	if len(got.Sends) != 2 || got.Sends[0].DelaySeconds != 5 || got.Sends[1].DelaySeconds != 1 {
		t.Fatal(got)
	}
}

func TestFollowUpBacksOffAFailureRetryFrom5sTo1000s(t *testing.T) {
	failing := []followIn{
		{Target: "scheduler"},
		{Target: "workloads"},
		{Target: "workloads", PlanOK: true, Changed: []string{"pods"}},
		{Target: "workloads", PlanOK: true, Changed: []string{"pods"}, HasResult: true},
		{Target: "crds", Status: 500},
		{Target: "crds", Status: 200, Pending: 1},
		{Target: "gc"},
		{Target: "accounts", Phase: "sync"},
		{Target: "accounts", Phase: "sync", HasResult: true},
		{Target: "accounts", Names: []string{"gone"}},
		{Target: "extensions"},
		{Target: "addons"},
		{Target: "addons", OK: true, MigrateFailed: true},
		{Target: "leases", Node: "n1"},
		{Target: "hpa"},
	}
	want := []int{5, 10, 20, 40, 80, 160, 320, 640, 1000, 1000, 1000}
	for _, in := range failing {
		for failures, delay := range want {
			in.Failures = failures
			got := followUp(in)
			if len(got.Sends) != 1 || got.Sends[0].DelaySeconds != delay || got.Sends[0].Failures != failures+1 {
				t.Fatalf("%+v: sends = %+v, want one retry in %ds counting %d failures", in, got.Sends, delay, failures+1)
			}
		}
	}
}

func TestFollowUpClearsTheFailureCountOnSuccess(t *testing.T) {
	for _, in := range []followIn{
		{Target: "workloads", PlanOK: true, Changed: []string{"jobs"}, HasResult: true, Drained: true, NextMs: 4000, Failures: 6},
		{Target: "accounts", HasResult: true, NextMs: 2000, Names: []string{"gone"}, Failures: 6},
		{Target: "leases", Node: "n1", HasResult: true, NextMs: 30_000, Failures: 6},
		{Target: "addons", OK: true, NextMs: 299_001, Failures: 6},
		{Target: "hpa", HasResult: true, HPAs: 1, Failures: 6},
	} {
		got := followUp(in)
		if len(got.Sends) != 1 || got.Sends[0].Failures != 0 {
			t.Fatalf("%+v: sends = %+v, want the deadline booked with no failure count", in, got.Sends)
		}
	}
	for _, in := range []followIn{
		{Target: "workloads", PlanOK: true, Changed: []string{"jobs"}, HasResult: true, Drained: true, Failures: 6},
		{Target: "crds", Status: 200, Failures: 6},
		{Target: "extensions", OK: true, Failures: 6},
		{Target: "scheduler", OK: true, Failures: 6},
	} {
		if got := followUp(in); len(got.Sends) != 0 {
			t.Fatalf("%+v: sends = %+v, want nothing booked", in, got.Sends)
		}
	}
}

func TestFollowUpRetriesABusySyncIn5sWithoutCountingAFailure(t *testing.T) {
	for _, in := range []followIn{
		{Target: "workloads", PlanOK: true, Changed: []string{"pods"}, HasResult: true, Busy: true, NextMs: 2000, Failures: 3},
		{Target: "accounts", Phase: "sync", HasResult: true, Busy: true, Failures: 3},
	} {
		got := followUp(in)
		if len(got.Sends) != 1 || got.Sends[0].DelaySeconds != 5 || got.Sends[0].Failures != 3 {
			t.Fatalf("%+v: sends = %+v, want one retry in 5s keeping 3 failures", in, got.Sends)
		}
	}
}

func TestFollowUpResumesAStorageMigrationIn5sWithoutCountingAFailure(t *testing.T) {
	got := followUp(followIn{Target: "addons", OK: true, Pending: 2, Failures: 4})
	if len(got.Sends) != 1 || got.Sends[0].DelaySeconds != 5 || got.Sends[0].Failures != 0 || !slices.Equal(got.Sends[0].Names, []string{"storage-migrate"}) {
		t.Fatal(got)
	}
}
