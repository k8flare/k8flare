package workloads

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func TestAdoptCronChildrenRecordsUnfinishedJob(t *testing.T) {
	yes := true
	cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "forbid", UID: "cj"}}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "forbid-1",
			UID:       "job",
			OwnerReferences: []metav1.OwnerReference{{
				Kind:       "CronJob",
				Name:       "forbid",
				Controller: &yes,
				UID:        "cj",
			}},
		},
	}
	done := job.DeepCopy()
	done.Name = "forbid-0"
	done.UID = "done"
	done.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: v1.ConditionTrue}}
	changed := adoptCronChildren([]runtime.Object{cj}, []runtime.Object{job, done})
	if len(changed) != 1 || len(cj.Status.Active) != 1 || cj.Status.Active[0].UID != types.UID("job") {
		t.Fatalf("active = %#v changed = %d", cj.Status.Active, len(changed))
	}
	if again := adoptCronChildren([]runtime.Object{cj}, []runtime.Object{job}); len(again) != 0 {
		t.Fatalf("adopted twice: %d", len(again))
	}
}
