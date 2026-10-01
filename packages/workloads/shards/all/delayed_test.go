package all

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	"github.com/k8flare/k8flare/packages/workloads"
)

func TestSyncBooksTheDeploymentProgressDeadlineWithoutFurtherWrites(t *testing.T) {
	labels := map[string]string{"app": "web"}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "d1"},
		Spec: appsv1.DeploymentSpec{
			Replicas:                ptr.To[int32](1),
			ProgressDeadlineSeconds: ptr.To[int32](3),
			Strategy:                appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector:                &metav1.LabelSelector{MatchLabels: labels},
			Template:                v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}}},
		},
	}
	client := fake.NewSimpleClientset(d)
	assignGeneratedNames(client)
	var next time.Duration
	for i := 0; i < 2; i++ {
		result, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		next = time.Duration(result.NextMs) * time.Millisecond
	}
	if next <= 0 || next > 4*time.Second {
		t.Fatalf("next pass in %s, want within the progress deadline", next)
	}
	time.Sleep(next)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err := client.AppsV1().Deployments("default").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing && c.Status == v1.ConditionFalse && c.Reason == "ProgressDeadlineExceeded" {
			return
		}
	}
	t.Fatalf("conditions = %v", got.Status.Conditions)
}

func assignResourceVersions(client *fake.Clientset) {
	var version atomic.Int64
	stamp := func(action k8stesting.Action) (bool, runtime.Object, error) {
		if written, ok := action.(interface{ GetObject() runtime.Object }); ok {
			if obj, err := meta.Accessor(written.GetObject()); err == nil {
				obj.SetResourceVersion(strconv.FormatInt(version.Add(1), 10))
			}
		}
		return false, nil, nil
	}
	client.PrependReactor("create", "*", stamp)
	client.PrependReactor("update", "*", stamp)
}

func TestSyncDoesNotRebookAPodChangeWhenNothingIsDue(t *testing.T) {
	labels := map[string]string{"app": "web"}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "d1"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}}},
		},
	}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "s1"},
		Spec:       v1.ServiceSpec{Selector: labels, ClusterIP: "10.43.0.20", Ports: []v1.ServicePort{{Port: 80}}},
	}
	client := fake.NewSimpleClientset(d, svc)
	assignGeneratedNames(client)
	assignResourceVersions(client)
	changed := []string{"endpointslices", "controllerrevisions", "endpoints", "replicasets", "daemonsets", "deployments", "pods"}
	var next time.Duration
	for i := 0; i < 3; i++ {
		result, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, changed)
		if err != nil {
			t.Fatal(err)
		}
		next = time.Duration(result.NextMs) * time.Millisecond
	}
	pods, err := client.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
	if err != nil || len(pods.Items) != 1 {
		t.Fatalf("pods = %d, %v, want the one replica", len(pods.Items), err)
	}
	slices, err := client.DiscoveryV1().EndpointSlices("default").List(context.Background(), metav1.ListOptions{})
	if err != nil || len(slices.Items) != 1 {
		t.Fatalf("endpointslices = %d, %v, want the one the controller manages", len(slices.Items), err)
	}
	if next > 0 && next < time.Minute {
		t.Fatalf("next pass in %s with one pending pod and no deadline near, want none before the progress deadline", next)
	}
}

func TestSyncRebooksAPodChangeWhileAStatefulSetIsNotSettled(t *testing.T) {
	labels := map[string]string{"app": "db"}
	set := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default", UID: "s1", Generation: 1},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    ptr.To[int32](2),
			ServiceName: "db",
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template:    v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}}},
		},
	}
	client := fake.NewSimpleClientset(set)
	assignGeneratedNames(client)
	result, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"statefulsets", "pods"})
	if err != nil {
		t.Fatal(err)
	}
	if next := time.Duration(result.NextMs) * time.Millisecond; next <= 0 || next > 2*time.Second {
		t.Fatalf("next pass in %s while the StatefulSet has unready replicas, want within 2s", next)
	}
}

func TestSyncDoesNotRebookAFinishedJobPodChange(t *testing.T) {
	jobLabels := map[string]string{
		"batch.kubernetes.io/job-name":       "job1",
		"batch.kubernetes.io/controller-uid": "j1",
	}
	now := metav1.Now()
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default", UID: "j1"},
		Spec: batchv1.JobSpec{
			Parallelism: ptr.To[int32](1),
			Completions: ptr.To[int32](1),
			Selector:    &metav1.LabelSelector{MatchLabels: jobLabels},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: jobLabels},
				Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}},
			},
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{
					Type:               batchv1.JobComplete,
					Status:             v1.ConditionTrue,
					LastTransitionTime: now,
				},
			},
			Succeeded:      1,
			CompletionTime: &now,
			StartTime:      &now,
		},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job1-pod1",
			Namespace: "default",
			UID:       "p1",
			Labels:    jobLabels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "batch/v1",
					Kind:       "Job",
					Name:       "job1",
					UID:        "j1",
					Controller: ptr.To(true),
				},
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{Name: "c", Image: "i"}},
		},
		Status: v1.PodStatus{
			Phase: v1.PodSucceeded,
		},
	}
	client := fake.NewSimpleClientset(j, pod)
	assignGeneratedNames(client)
	assignResourceVersions(client)
	var next time.Duration
	for i := 0; i < 3; i++ {
		result, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"jobs", "pods"})
		if err != nil {
			t.Fatal(err)
		}
		next = time.Duration(result.NextMs) * time.Millisecond
	}
	if next != 0 {
		t.Fatalf("next pass in %s for a finished job with succeeded pod, want 0", next)
	}
}

func TestSyncPreservesJobActiveDeadlineSeconds(t *testing.T) {
	jobLabels := map[string]string{
		"batch.kubernetes.io/job-name":       "job2",
		"batch.kubernetes.io/controller-uid": "j2",
	}
	now := metav1.Now()
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job2", Namespace: "default", UID: "j2"},
		Spec: batchv1.JobSpec{
			Parallelism:           ptr.To[int32](1),
			Completions:           ptr.To[int32](1),
			BackoffLimit:          ptr.To[int32](6),
			ActiveDeadlineSeconds: ptr.To[int64](30),
			Selector:              &metav1.LabelSelector{MatchLabels: jobLabels},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: jobLabels},
				Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}},
			},
		},
		Status: batchv1.JobStatus{
			StartTime: &now,
		},
	}
	client := fake.NewSimpleClientset(j)
	assignGeneratedNames(client)
	assignResourceVersions(client)
	result, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"jobs", "pods"})
	if err != nil {
		t.Fatal(err)
	}
	next := time.Duration(result.NextMs) * time.Millisecond
	if next <= 0 || next > 30*time.Second {
		t.Fatalf("next pass in %s for job with activeDeadlineSeconds 30s, want 0 < next <= 30s", next)
	}
}
