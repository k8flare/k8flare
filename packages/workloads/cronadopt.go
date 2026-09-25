package workloads

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

func loadedNamed(src []source, loaded [][]runtime.Object, name string) []runtime.Object {
	for i, s := range src {
		if s.name == name {
			return loaded[i]
		}
	}
	return nil
}

func recordCronChildren(ctx context.Context, client kubernetes.Interface, cronjobs, jobs []runtime.Object) error {
	changed := adoptCronChildren(cronjobs, jobs)
	var err error
	for _, cj := range changed {
		if _, updateErr := client.BatchV1().CronJobs(cj.Namespace).UpdateStatus(ctx, cj, metav1.UpdateOptions{}); updateErr != nil && err == nil {
			err = updateErr
		}
	}
	return err
}

func adoptCronChildren(cronjobs, jobs []runtime.Object) []*batchv1.CronJob {
	byName := map[string]*batchv1.CronJob{}
	for _, o := range cronjobs {
		cj := o.(*batchv1.CronJob)
		byName[cj.Namespace+"/"+cj.Name] = cj
	}
	var changed []*batchv1.CronJob
	seen := map[types.UID]bool{}
	for _, o := range jobs {
		j := o.(*batchv1.Job)
		if jobFinished(j) || j.DeletionTimestamp != nil {
			continue
		}
		ns, name, ok := cronOwner(j)
		if !ok {
			continue
		}
		cj := byName[ns+"/"+name]
		if cj == nil || cronActive(cj, j.UID) {
			continue
		}
		cj.Status.Active = append(cj.Status.Active, v1.ObjectReference{
			APIVersion: "batch/v1",
			Kind:       "Job",
			Namespace:  j.Namespace,
			Name:       j.Name,
			UID:        j.UID,
		})
		if !seen[cj.UID] {
			seen[cj.UID] = true
			changed = append(changed, cj)
		}
	}
	return changed
}

func cronActive(cj *batchv1.CronJob, uid types.UID) bool {
	for _, ref := range cj.Status.Active {
		if ref.UID == uid {
			return true
		}
	}
	return false
}

func cronOwner(j *batchv1.Job) (string, string, bool) {
	for _, ref := range j.OwnerReferences {
		if ref.Kind == "CronJob" && ref.Controller != nil && *ref.Controller {
			return j.Namespace, ref.Name, true
		}
	}
	return "", "", false
}

func jobFinished(j *batchv1.Job) bool {
	for _, c := range j.Status.Conditions {
		if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}
