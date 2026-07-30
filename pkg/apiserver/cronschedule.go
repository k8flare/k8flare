package apiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// A CronJob is the one workload whose next unit of work is due at a wall
// clock time rather than in response to a write, which puts it at odds with
// a control plane that parks everything when idle. The Controllers DO's
// hasUnconvergedWork() probe answers "is there work outstanding right now",
// and for a CronJob between fires the honest answer is no -- so the alarm
// parks, the dynamic workers are evicted, and the schedule passes with
// nothing running to notice.
//
// Confirmed by measurement 2026-07-30 (docs/platform-verification.md S27),
// and the measurement is worth describing because two earlier attempts said
// the opposite: a CronJob DOES fire from an apparently parked cluster as
// long as the KCM's isolate happens to still be alive, because the real
// cronjob controller's own timer is still running inside it. Only after
// restarting wrangler -- evicting every isolate, which is what eviction
// does in production -- did the schedule actually get missed.
//
// The fix is to give the alarm a wake time instead of a park: this endpoint
// answers "when is the earliest moment any CronJob in this cluster next
// needs attention", and the Controllers DO arms its alarm for exactly that
// instead of parking. Still event-armed, not polling -- the alarm exists
// only because a CronJob exists, is set once for a specific instant, and
// parks again when the last CronJob is deleted.
//
// Deliberately narrow: this computes only WHEN to wake. What to do on
// waking -- missed-schedule policy, startingDeadlineSeconds, the 100-missed
// giveup, concurrency policy -- stays entirely with the real upstream
// cronjob controller that runs when the alarm fires. Waking too eagerly
// costs one no-op sync; reimplementing that policy here would be a second
// source of truth that could diverge from it.

func cronJobsResourceStore(storage *Storage) *ResourceStore {
	return NewResourceStore(storage, batchv1.SchemeGroupVersion, "cronjobs", "cronjob", true,
		func() runtime.Object { return &batchv1.CronJob{} },
		func() runtime.Object { return &batchv1.CronJobList{} },
	)
}

type nextCronScheduleResponse struct {
	// NextScheduleTime is RFC3339, or empty when no CronJob needs a wake-up
	// (none exist, all suspended, or every schedule is unparseable).
	NextScheduleTime string `json:"nextScheduleTime,omitempty"`
	// Overdue reports that some CronJob has an occurrence at or before now
	// that it has not recorded running yet. The caller must treat this as
	// outstanding work rather than as a wake time: waking once at the right
	// instant is not enough, because the controller needs the pump window
	// held open long enough to actually create the Job. Measured 2026-07-30:
	// an alarm that fired exactly at the schedule and then re-armed for the
	// next occurrence dropped the fire entirely (S27).
	Overdue bool `json:"overdue"`
	// CronJobs counted, for the caller's logging.
	CronJobs int `json:"cronJobs"`
}

// scheduleExpr mirrors upstream's formatSchedule
// (.build/k8s-js-mirror/pkg/controller/cronjob/cronjob_controllerv2.go:775):
// spec.timeZone is expressed to the parser as a TZ= prefix.
func scheduleExpr(cj *batchv1.CronJob) string {
	if cj.Spec.TimeZone != nil && *cj.Spec.TimeZone != "" {
		return fmt.Sprintf("TZ=%s %s", *cj.Spec.TimeZone, cj.Spec.Schedule)
	}
	return cj.Spec.Schedule
}

// NextCronScheduleTime returns the earliest FUTURE fire time across every
// CronJob in the cluster (zero when there is none), and whether any CronJob
// has an occurrence at or before now that it has not run yet.
func NextCronScheduleTime(ctx context.Context, storage *Storage, now time.Time) (next time.Time, overdue bool, count int, err error) {
	listObj, err := cronJobsResourceStore(storage).List(ctx, "", "", "")
	if err != nil {
		return time.Time{}, false, 0, fmt.Errorf("list cronjobs: %w", err)
	}
	list, ok := listObj.(*batchv1.CronJobList)
	if !ok {
		return time.Time{}, false, 0, fmt.Errorf("list cronjobs: unexpected type %T", listObj)
	}

	var earliest time.Time
	for i := range list.Items {
		due, isOverdue, ok := cronJobDue(&list.Items[i], now)
		if !ok {
			continue
		}
		if isOverdue {
			overdue = true
			continue
		}
		if earliest.IsZero() || due.Before(earliest) {
			earliest = due
		}
	}
	return earliest, overdue, len(list.Items), nil
}

// cronJobDue answers, for one CronJob, when it next needs the control plane
// awake. ok is false when it needs nothing at all -- suspended, or a
// schedule the parser rejects (upstream records an event and stops
// scheduling it, so there is nothing to wake for; failing the whole probe
// instead would hold the alarm open on one bad object).
func cronJobDue(cj *batchv1.CronJob, now time.Time) (due time.Time, overdue bool, ok bool) {
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		return time.Time{}, false, false
	}
	sched, err := cron.ParseStandard(scheduleExpr(cj))
	if err != nil {
		return time.Time{}, false, false
	}
	// Measure from the last recorded fire, falling back to creation for a
	// CronJob that has never fired. If the occurrence that follows it is
	// already in the past, this CronJob is behind: the cluster was parked
	// or evicted across its slot.
	from := cj.CreationTimestamp.Time
	if cj.Status.LastScheduleTime != nil {
		from = cj.Status.LastScheduleTime.Time
	}
	due = sched.Next(from)
	return due, !due.After(now), true
}

// GET /internal/next-cron-schedule: called from the Controllers DO's alarm
// park decision (packages/k8flare-worker/src/controllers/index.ts).
func handleNextCronSchedule(storage *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next, overdue, count, err := NextCronScheduleTime(r.Context(), storage, time.Now())
		if err != nil {
			writeInternalError(w, err)
			return
		}
		resp := nextCronScheduleResponse{Overdue: overdue, CronJobs: count}
		if !next.IsZero() {
			resp.NextScheduleTime = next.UTC().Format(time.RFC3339)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
