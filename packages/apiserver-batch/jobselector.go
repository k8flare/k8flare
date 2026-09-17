package batch

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

type jobCreateStrategy struct {
	rest.RESTCreateStrategy
}

func (s jobCreateStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	s.RESTCreateStrategy.PrepareForCreate(ctx, obj)
	job := obj.(*batchv1.Job)
	if job.Spec.ManualSelector != nil && *job.Spec.ManualSelector {
		return
	}
	if job.Spec.Template.Labels == nil {
		job.Spec.Template.Labels = map[string]string{}
	}
	for _, key := range []string{"job-name", batchv1.JobNameLabel} {
		if _, ok := job.Spec.Template.Labels[key]; !ok {
			job.Spec.Template.Labels[key] = job.Name
		}
	}
	for _, key := range []string{"controller-uid", batchv1.ControllerUidLabel} {
		if _, ok := job.Spec.Template.Labels[key]; !ok {
			job.Spec.Template.Labels[key] = string(job.UID)
		}
	}
	if job.Spec.Selector == nil {
		job.Spec.Selector = &metav1.LabelSelector{}
	}
	if job.Spec.Selector.MatchLabels == nil {
		job.Spec.Selector.MatchLabels = map[string]string{}
	}
	if _, ok := job.Spec.Selector.MatchLabels[batchv1.ControllerUidLabel]; !ok {
		job.Spec.Selector.MatchLabels[batchv1.ControllerUidLabel] = string(job.UID)
	}
}
