package batch

import (
	"context"
	"time"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	"github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
)

func init() {
	registry.Customizers["jobs"] = func(store *registry.Store, _ registry.Deps) { registry.PokeControllersOn(store) }
	registry.Customizers["cronjobs"] = func(store *registry.Store, _ registry.Deps) {
		registry.PokeControllersOn(store)
		store.BeginCreate = func(_ context.Context, obj runtime.Object, _ *metav1.CreateOptions) (genericregistry.FinishFunc, error) {
			return wakeAtNextSchedule(obj.(*batchv1.CronJob)), nil
		}
		store.BeginUpdate = func(_ context.Context, obj, _ runtime.Object, _ *metav1.UpdateOptions) (genericregistry.FinishFunc, error) {
			return wakeAtNextSchedule(obj.(*batchv1.CronJob)), nil
		}
	}
}

func wakeAtNextSchedule(cj *batchv1.CronJob) genericregistry.FinishFunc {
	return func(ctx context.Context, success bool) {
		if !success || registry.PokeControllers == nil {
			return
		}
		registry.PokeControllers(ctx)
		if registry.WakeControllers == nil || (cj.Spec.Suspend != nil && *cj.Spec.Suspend) {
			return
		}
		if next, ok := nextSchedule(cj, time.Now()); ok {
			registry.WakeControllers(ctx, time.Until(next))
		}
	}
}

func nextSchedule(cj *batchv1.CronJob, now time.Time) (time.Time, bool) {
	schedule := cj.Spec.Schedule
	if cj.Spec.TimeZone != nil && *cj.Spec.TimeZone != "" {
		schedule = "TZ=" + *cj.Spec.TimeZone + " " + schedule
	}
	parsed, err := cron.ParseStandard(schedule)
	if err != nil {
		return time.Time{}, false
	}
	next := parsed.Next(now)
	if next.IsZero() {
		return time.Time{}, false
	}
	return next, true
}
