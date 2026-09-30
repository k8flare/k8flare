package all

import (
	"context"
	"errors"
	"math"
	"strconv"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	"github.com/k8flare/k8flare/packages/workloads"
)

func TestSyncRunsEachFailingIndexOnce(t *testing.T) {
	labels := map[string]string{"job": "indexed"}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "indexed", Namespace: "default", UID: "j1", ResourceVersion: "1"},
		Spec: batchv1.JobSpec{
			Completions:          ptr.To[int32](3),
			Parallelism:          ptr.To[int32](1),
			CompletionMode:       ptr.To(batchv1.IndexedCompletion),
			BackoffLimit:         ptr.To[int32](math.MaxInt32),
			BackoffLimitPerIndex: ptr.To[int32](0),
			ManualSelector:       ptr.To(true),
			Selector:             &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       v1.PodSpec{RestartPolicy: v1.RestartPolicyNever, Containers: []v1.Container{{Name: "c", Image: "i"}}},
			},
		},
	}
	client := fake.NewSimpleClientset(job)
	assignGeneratedNames(client)
	version, conflicts := 1, 0
	client.PrependReactor("update", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		sent := action.(k8stesting.UpdateAction).GetObject().(*batchv1.Job)
		stored, err := client.Tracker().Get(action.GetResource(), sent.Namespace, sent.Name)
		if err != nil {
			return true, nil, err
		}
		if stored.(*batchv1.Job).ResourceVersion != sent.ResourceVersion {
			conflicts++
			return true, nil, apierrors.NewConflict(action.GetResource().GroupResource(), sent.Name, errors.New("the object has been modified"))
		}
		version++
		sent.ResourceVersion = strconv.Itoa(version)
		return false, nil, nil
	})
	client.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		pod := action.(k8stesting.CreateAction).GetObject().(*v1.Pod)
		if pod.Annotations[batchv1.JobCompletionIndexAnnotation] == "1" {
			pod.Status.Phase = v1.PodSucceeded
		} else {
			pod.Status.Phase = v1.PodFailed
		}
		return false, nil, nil
	})
	for range 2 {
		if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if conflicts != 0 {
		t.Fatalf("job writes conflicted %d times", conflicts)
	}
	if n := creates(client, "pods"); n != 3 {
		t.Fatalf("created %d pods, want 3", n)
	}
	got, err := client.BatchV1().Jobs("default").Get(context.Background(), "indexed", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.CompletedIndexes != "1" || ptr.Deref(got.Status.FailedIndexes, "") != "0,2" {
		t.Fatalf("completed=%q failed=%q status=%+v", got.Status.CompletedIndexes, ptr.Deref(got.Status.FailedIndexes, ""), got.Status)
	}
}
