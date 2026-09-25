package batch

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDefaultsJobAndCronJob(t *testing.T) {
	job := &batchv1.Job{}
	scheme.Scheme.Default(job)
	if job.Spec.Completions == nil || *job.Spec.Completions != 1 || job.Spec.Parallelism == nil || *job.Spec.Parallelism != 1 {
		t.Fatalf("completions=%v parallelism=%v", job.Spec.Completions, job.Spec.Parallelism)
	}
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 6 {
		t.Fatalf("backoffLimit=%v", job.Spec.BackoffLimit)
	}
	if job.Spec.CompletionMode == nil || *job.Spec.CompletionMode != batchv1.NonIndexedCompletion {
		t.Fatalf("completionMode=%v", job.Spec.CompletionMode)
	}
	if job.Spec.Suspend == nil || *job.Spec.Suspend {
		t.Fatalf("suspend=%v", job.Spec.Suspend)
	}
	cron := &batchv1.CronJob{}
	scheme.Scheme.Default(cron)
	if cron.Spec.ConcurrencyPolicy != batchv1.AllowConcurrent {
		t.Fatalf("concurrency=%q", cron.Spec.ConcurrencyPolicy)
	}
	if cron.Spec.SuccessfulJobsHistoryLimit == nil || *cron.Spec.SuccessfulJobsHistoryLimit != 3 {
		t.Fatalf("successful=%v", cron.Spec.SuccessfulJobsHistoryLimit)
	}
	if cron.Spec.FailedJobsHistoryLimit == nil || *cron.Spec.FailedJobsHistoryLimit != 1 {
		t.Fatalf("failed=%v", cron.Spec.FailedJobsHistoryLimit)
	}
}
