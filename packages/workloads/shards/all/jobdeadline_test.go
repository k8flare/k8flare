package all

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	"github.com/k8flare/k8flare/packages/workloads"
)

var jobChanges = []string{"jobs", "pods"}

func oneShotJob(name string) *batchv1.Job {
	labels := map[string]string{"job": name}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID("job-" + name)},
		Spec: batchv1.JobSpec{
			Completions:    ptr.To[int32](1),
			Parallelism:    ptr.To[int32](1),
			BackoffLimit:   ptr.To[int32](6),
			CompletionMode: ptr.To(batchv1.NonIndexedCompletion),
			ManualSelector: ptr.To(true),
			Selector:       &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       v1.PodSpec{RestartPolicy: v1.RestartPolicyNever, Containers: []v1.Container{{Name: "c", Image: "i"}}},
			},
		},
	}
}

func createPodsIn(client *fake.Clientset, phase v1.PodPhase) {
	client.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		pod := action.(k8stesting.CreateAction).GetObject().(*v1.Pod)
		now := metav1.Now()
		pod.CreationTimestamp = now
		pod.Status.Phase = phase
		if phase == v1.PodFailed {
			pod.Status.ContainerStatuses = []v1.ContainerStatus{{Name: "c", State: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{ExitCode: 1, FinishedAt: now}}}}
		}
		return false, nil, nil
	})
}

func syncJobs(t *testing.T, client *fake.Clientset, passes int) time.Duration {
	t.Helper()
	var next time.Duration
	for range passes {
		result, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, jobChanges)
		if err != nil {
			t.Fatal(err)
		}
		next = time.Duration(result.NextMs) * time.Millisecond
	}
	return next
}

func TestSyncBooksNothingForARunningJobWithNoDeadline(t *testing.T) {
	client := fake.NewSimpleClientset(oneShotJob("idle"))
	assignGeneratedNames(client)
	assignResourceVersions(client)
	createPodsIn(client, v1.PodRunning)
	next := syncJobs(t, client, 3)
	if n := creates(client, "pods"); n != 1 {
		t.Fatalf("created %d pods, want 1", n)
	}
	if next != 0 {
		t.Fatalf("next pass in %s for a running Job with no deadline, want none", next)
	}
}

func TestSyncBooksTheJobActiveDeadlineAndFailsTheJobWhenItPasses(t *testing.T) {
	job := oneShotJob("deadline")
	job.Spec.ActiveDeadlineSeconds = ptr.To[int64](3)
	client := fake.NewSimpleClientset(job)
	assignGeneratedNames(client)
	assignResourceVersions(client)
	createPodsIn(client, v1.PodRunning)
	next := syncJobs(t, client, 2)
	if next <= 0 || next > 3*time.Second {
		t.Fatalf("next pass in %s, want within the active deadline", next)
	}
	time.Sleep(next)
	syncJobs(t, client, 1)
	got, err := client.BatchV1().Jobs("default").Get(context.Background(), "deadline", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == v1.ConditionTrue && c.Reason == batchv1.JobReasonDeadlineExceeded {
			return
		}
	}
	t.Fatalf("conditions = %v", got.Status.Conditions)
}

func TestSyncBooksThePodFailureBackoffOfAJob(t *testing.T) {
	client := fake.NewSimpleClientset(oneShotJob("backoff"))
	assignGeneratedNames(client)
	assignResourceVersions(client)
	createPodsIn(client, v1.PodFailed)
	next := syncJobs(t, client, 1)
	if n := creates(client, "pods"); n != 1 {
		t.Fatalf("created %d pods, want 1 until the backoff passes", n)
	}
	if next <= 0 || next > 10*time.Second {
		t.Fatalf("next pass in %s, want within the 10s pod failure backoff", next)
	}
}
