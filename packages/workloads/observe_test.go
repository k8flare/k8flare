package workloads

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func TestStatefulSetStatusWriteEntersSnapshot(t *testing.T) {
	set := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "ss", Namespace: "default", ResourceVersion: "1"}}
	sets := newSnapshotInformer(&appsv1.StatefulSet{})
	client := observeWrites(fake.NewSimpleClientset(set), []loadedSource{{informer: sets}})
	update := set.DeepCopy()
	update.Status.Replicas = 1
	if _, err := client.AppsV1().StatefulSets("default").UpdateStatus(context.Background(), update, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	stored, exists, err := sets.GetIndexer().GetByKey("default/ss")
	if err != nil || !exists || stored.(*appsv1.StatefulSet).Status.Replicas != 1 {
		t.Fatalf("exists=%v err=%v stored=%#v", exists, err, stored)
	}
}

func TestPodCreateEntersSnapshot(t *testing.T) {
	pods := newSnapshotInformer(&corev1.Pod{})
	var created *corev1.Pod
	if _, err := pods.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) { created = obj.(*corev1.Pod) },
	}); err != nil {
		t.Fatal(err)
	}
	client := observeWrites(fake.NewSimpleClientset(), []loadedSource{{informer: pods}})
	got, err := client.CoreV1().Pods("default").Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pause", Namespace: "default"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if created == nil || created.Name != "pause" {
		t.Fatalf("handler saw %#v", created)
	}
	stored, exists, err := pods.GetIndexer().GetByKey("default/pause")
	if err != nil || !exists || stored.(*corev1.Pod).Name != got.Name {
		t.Fatalf("indexer exists=%v err=%v stored=%#v", exists, err, stored)
	}
}

func TestPodCreatePublishesRCStatusWhenTheSpecIsMet(t *testing.T) {
	uid := types.UID("rc-uid")
	controlled := true
	replicas := int32(1)
	rc := &corev1.ReplicationController{
		ObjectMeta: metav1.ObjectMeta{Name: "pause", Namespace: "default", UID: uid, ResourceVersion: "3"},
		Spec:       corev1.ReplicationControllerSpec{Replicas: &replicas},
	}
	pods := newSnapshotInformer(&corev1.Pod{})
	rcs := newSnapshotInformer(&corev1.ReplicationController{})
	client := observeWrites(fake.NewSimpleClientset(rc), []loadedSource{{informer: pods}, {informer: rcs}})
	_, err := client.CoreV1().Pods("default").Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pause",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				Controller: &controlled, Kind: "ReplicationController", Name: "pause", UID: uid,
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().ReplicationControllers("default").Get(context.Background(), "pause", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.Replicas != 1 {
		t.Fatalf("status.replicas=%d", got.Status.Replicas)
	}
}

func TestRCStatusCountsPodsCreatedDuringSync(t *testing.T) {
	uid := types.UID("rc-uid")
	controlled := true
	replicas := int32(3)
	rc := &corev1.ReplicationController{
		ObjectMeta: metav1.ObjectMeta{Name: "pause", Namespace: "default", UID: uid, ResourceVersion: "9"},
		Spec:       corev1.ReplicationControllerSpec{Replicas: &replicas},
	}
	pods := newSnapshotInformer(&corev1.Pod{})
	owner := []metav1.OwnerReference{{Controller: &controlled, Kind: "ReplicationController", Name: "pause", UID: uid}}
	for _, pod := range []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default", OwnerReferences: owner}},
		{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "default", OwnerReferences: owner}},
		{ObjectMeta: metav1.ObjectMeta{Name: "done", Namespace: "default", OwnerReferences: owner}, Status: corev1.PodStatus{Phase: corev1.PodSucceeded}},
	} {
		if err := pods.GetIndexer().Add(pod); err != nil {
			t.Fatal(err)
		}
	}
	client := observeWrites(fake.NewSimpleClientset(rc), []loadedSource{{informer: pods}})
	stale := rc.DeepCopy()
	stale.ResourceVersion = "1"
	got, err := client.CoreV1().ReplicationControllers("default").UpdateStatus(context.Background(), stale, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.Replicas != 2 || got.Status.FullyLabeledReplicas != 2 {
		t.Fatalf("status=%#v", got.Status)
	}
	if got.ResourceVersion == "1" {
		t.Fatal("status write kept the stale resourceVersion")
	}
}

func TestRCStatusListsPodsMissingFromTheSnapshot(t *testing.T) {
	uid := types.UID("rc-uid")
	controlled := true
	replicas := int32(2)
	owner := []metav1.OwnerReference{{Controller: &controlled, Kind: "ReplicationController", Name: "pause", UID: uid}}
	rc := &corev1.ReplicationController{
		ObjectMeta: metav1.ObjectMeta{Name: "pause", Namespace: "default", UID: uid, ResourceVersion: "4"},
		Spec:       corev1.ReplicationControllerSpec{Replicas: &replicas},
	}
	serverPods := make([]runtime.Object, 0, 2)
	for _, name := range []string{"a", "b"} {
		serverPods = append(serverPods, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", OwnerReferences: owner}})
	}
	pods := newSnapshotInformer(&corev1.Pod{})
	client := observeWrites(fake.NewSimpleClientset(append(serverPods, rc)...), []loadedSource{{informer: pods}})
	got, err := client.CoreV1().ReplicationControllers("default").UpdateStatus(context.Background(), rc.DeepCopy(), metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.Replicas != 2 {
		t.Fatalf("status.replicas=%d", got.Status.Replicas)
	}
	stored, exists, err := pods.GetIndexer().GetByKey("default/b")
	if err != nil || !exists || stored.(*corev1.Pod).Name != "b" {
		t.Fatalf("snapshot missed the listed pod exists=%v err=%v", exists, err)
	}
}

func TestStalePodListReplacesTheSnapshot(t *testing.T) {
	pods := newSnapshotInformer(&corev1.Pod{})
	pods.list = func(context.Context) ([]runtime.Object, error) {
		return []runtime.Object{&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "late", Namespace: "default"}}}, nil
	}
	pods.markStale()
	got, err := pods.GetIndexer().ByIndex(cache.NamespaceIndex, "default")
	if err != nil || len(got) != 1 || got[0].(*corev1.Pod).Name != "late" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
