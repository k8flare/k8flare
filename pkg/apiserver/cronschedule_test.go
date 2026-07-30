package apiserver

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// TestCronJobDue covers the decision that keeps a CronJob alive on a control
// plane that parks when idle: when to wake, and when the slot has already
// been missed. The missed case is the one that matters -- a cluster whose
// isolates were evicted across a schedule has to notice on the way back up,
// and an earlier version that only ever armed for the NEXT occurrence
// silently dropped the fire (docs/platform-verification.md S27).
func TestCronJobDue(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 30, 0, 0, time.UTC)
	at := func(h, m int) metav1.Time {
		return metav1.NewTime(time.Date(2026, 7, 30, h, m, 0, 0, time.UTC))
	}

	tests := []struct {
		name        string
		cj          batchv1.CronJob
		wantOK      bool
		wantOverdue bool
		wantDue     time.Time
	}{{
		name: "never fired, next slot ahead",
		cj: batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{CreationTimestamp: at(12, 29)},
			Spec:       batchv1.CronJobSpec{Schedule: "45 * * * *"},
		},
		wantOK:  true,
		wantDue: time.Date(2026, 7, 30, 12, 45, 0, 0, time.UTC),
	}, {
		// The S27 case: parked across the slot. Must come back overdue, not
		// as a wake-up an hour out.
		name: "slot passed while the cluster was down",
		cj: batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{CreationTimestamp: at(10, 0)},
			Spec:       batchv1.CronJobSpec{Schedule: "15 * * * *"},
			Status:     batchv1.CronJobStatus{LastScheduleTime: ptr.To(at(11, 15))},
		},
		wantOK:      true,
		wantOverdue: true,
	}, {
		name: "already ran this slot, next one ahead",
		cj: batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{CreationTimestamp: at(10, 0)},
			Spec:       batchv1.CronJobSpec{Schedule: "15 * * * *"},
			Status:     batchv1.CronJobStatus{LastScheduleTime: ptr.To(at(12, 15))},
		},
		wantOK:  true,
		wantDue: time.Date(2026, 7, 30, 13, 15, 0, 0, time.UTC),
	}, {
		name: "suspended needs nothing",
		cj: batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{CreationTimestamp: at(10, 0)},
			Spec:       batchv1.CronJobSpec{Schedule: "* * * * *", Suspend: ptr.To(true)},
		},
		wantOK: false,
	}, {
		// Upstream events it and stops scheduling. Waking for it forever
		// would be an alarm chain no one can clear.
		name: "unparseable schedule needs nothing",
		cj: batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{CreationTimestamp: at(10, 0)},
			Spec:       batchv1.CronJobSpec{Schedule: "not a schedule"},
		},
		wantOK: false,
	}, {
		// spec.timeZone must reach the parser, or the wake time is computed
		// in the wrong zone and can be hours off.
		name: "timeZone is honoured",
		cj: batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{CreationTimestamp: at(12, 0)},
			Spec: batchv1.CronJobSpec{
				Schedule: "0 22 * * *", // 22:00 in Tokyo == 13:00 UTC
				TimeZone: ptr.To("Asia/Tokyo"),
			},
		},
		wantOK:  true,
		wantDue: time.Date(2026, 7, 30, 13, 0, 0, 0, time.UTC),
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			due, overdue, ok := cronJobDue(&tc.cj, now)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if overdue != tc.wantOverdue {
				t.Errorf("overdue = %v, want %v (due %s)", overdue, tc.wantOverdue, due)
			}
			if !tc.wantOverdue && !due.Equal(tc.wantDue) {
				t.Errorf("due = %s, want %s", due.UTC(), tc.wantDue)
			}
		})
	}
}

// A CronJob whose timeZone does not exist must not be treated as needing a
// wake-up: upstream refuses to schedule it, so an alarm armed for it would
// never be cleared by anything.
func TestCronJobDueRejectsUnknownTimeZone(t *testing.T) {
	cj := batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.Now()},
		Spec: batchv1.CronJobSpec{
			Schedule: "* * * * *",
			TimeZone: ptr.To("Mars/Olympus_Mons"),
		},
	}
	if _, _, ok := cronJobDue(&cj, time.Now()); ok {
		t.Error("an unknown timeZone must not produce a wake-up")
	}
}
