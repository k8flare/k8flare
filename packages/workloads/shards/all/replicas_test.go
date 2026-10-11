package all

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"

	"github.com/k8flare/k8flare/packages/workloads"
)

func storeWrites(client *fake.Clientset) int {
	n := 0
	for _, a := range client.Actions() {
		switch a.GetVerb() {
		case "create", "update", "patch", "delete":
			if a.GetResource().Resource != "events" {
				n++
			}
		}
	}
	return n
}

func syncUntilQuiet(t *testing.T, client *fake.Clientset, changed []string) *workloads.Result {
	t.Helper()
	for range 8 {
		client.ClearActions()
		result, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, changed)
		if err != nil {
			t.Fatal(err)
		}
		if storeWrites(client) == 0 {
			return result
		}
	}
	t.Fatal("passes kept writing after 8 rounds")
	return nil
}

func markReady(t *testing.T, client *fake.Clientset, selector string) {
	t.Helper()
	pods, err := client.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		t.Fatal(err)
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase == v1.PodRunning {
			continue
		}
		pod.Status.Phase = v1.PodRunning
		pod.Status.Conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue, LastTransitionTime: metav1.Now()}}
		if _, err := client.CoreV1().Pods("default").UpdateStatus(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAReplicationControllerScalingUpConvergesOnItsOwnWritesAndBooksNothing(t *testing.T) {
	labels := map[string]string{"app": "burst"}
	rc := &v1.ReplicationController{
		ObjectMeta: metav1.ObjectMeta{Name: "burst", Namespace: "default", UID: "rc1"},
		Spec: v1.ReplicationControllerSpec{
			Replicas: ptr.To[int32](40),
			Selector: labels,
			Template: &v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}}},
		},
	}
	client := fake.NewSimpleClientset(rc)
	assignGeneratedNames(client)
	assignResourceVersions(client)
	result := syncUntilQuiet(t, client, []string{"replicationcontrollers", "pods"})
	got, err := client.CoreV1().ReplicationControllers("default").Get(context.Background(), "burst", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.Replicas != 40 {
		t.Fatalf("status.replicas = %d once the passes stopped writing, want 40", got.Status.Replicas)
	}
	if result.NextMs != 0 {
		t.Fatalf("next pass in %dms with 40 unready replicas and nothing due, want none", result.NextMs)
	}
}

func TestARolloutWaitingForANewPodBooksNothingBeforeItsProgressDeadlineAndFinishesOnThePodsReadyWrite(t *testing.T) {
	labels := map[string]string{"app": "web"}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "d1", Generation: 1},
		Spec: appsv1.DeploymentSpec{
			Replicas:                ptr.To[int32](1),
			ProgressDeadlineSeconds: ptr.To[int32](600),
			RevisionHistoryLimit:    ptr.To[int32](10),
			Strategy: appsv1.DeploymentStrategy{
				Type:          appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{MaxSurge: ptr.To(intstr.FromInt32(1)), MaxUnavailable: ptr.To(intstr.FromInt32(0))},
			},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "v1"}}}},
		},
	}
	client := fake.NewSimpleClientset(d)
	assignGeneratedNames(client)
	assignResourceVersions(client)
	changed := []string{"deployments", "replicasets", "pods"}
	syncUntilQuiet(t, client, changed)
	markReady(t, client, "app=web")
	syncUntilQuiet(t, client, changed)

	current, err := client.AppsV1().Deployments("default").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	current.Spec.Template.Spec.Containers[0].Image = "v2"
	current.Generation = 2
	if _, err := client.AppsV1().Deployments("default").Update(context.Background(), current, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	waiting := syncUntilQuiet(t, client, changed)
	rolling, err := client.AppsV1().Deployments("default").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rolling.Status.Replicas != 2 || rolling.Status.UpdatedReplicas != 1 {
		t.Fatalf("status = %+v, want the old and the unready new pod", rolling.Status)
	}
	if next := time.Duration(waiting.NextMs) * time.Millisecond; next > 0 && next < 9*time.Minute {
		t.Fatalf("next pass in %s while the rollout waits on an unready pod, want none before the 600s progress deadline", next)
	}

	markReady(t, client, "app=web")
	syncUntilQuiet(t, client, changed)
	done, err := client.AppsV1().Deployments("default").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if done.Status.Replicas != 1 || done.Status.UpdatedReplicas != 1 || done.Status.AvailableReplicas != 1 {
		t.Fatalf("status = %+v after the new pod's Ready write, want the rollout finished", done.Status)
	}
}
