package all

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	"github.com/k8flare/k8flare/packages/workloads"
)

type storedWrite struct {
	Rev   int64  `json:"rev"`
	Type  string `json:"type"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

func TestADaemonPodThatFailedAndCameBackBeforeThePassListedIsStillDeleted(t *testing.T) {
	labels := map[string]string{"app": "daemon"}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "daemon", Namespace: "spec", UID: "ds-uid"},
		Spec: appsv1.DaemonSetSpec{
			Selector:       &metav1.LabelSelector{MatchLabels: labels},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "app", Image: "app"}}},
			},
		},
	}
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status:     v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}}},
	}
	running := &v1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "daemon-x", Namespace: "spec", UID: "pod-uid", Labels: labels,
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: "daemon", UID: "ds-uid", Controller: ptr.To(true)}},
		},
		Spec:   v1.PodSpec{NodeName: "node-1", Containers: []v1.Container{{Name: "app", Image: "app"}}},
		Status: v1.PodStatus{Phase: v1.PodRunning},
	}
	client := fake.NewSimpleClientset(ds, node, running.DeepCopy())
	var listedAt atomic.Int64
	client.PrependReactor("list", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		handled, obj, err := k8stesting.ObjectReaction(client.Tracker())(action)
		if list, listErr := meta.ListAccessor(obj); listErr == nil {
			list.SetResourceVersion(strconv.FormatInt(listedAt.Load(), 10))
		}
		return handled, obj, err
	})
	failed := running.DeepCopy()
	failed.Status.Phase = v1.PodFailed
	var history []storedWrite
	writesAfterTheFirstPass := []storedWrite{
		{Rev: 11, Type: "modified", Key: "/registry/pods/spec/daemon-x", Value: encodeStored(t, failed)},
		{Rev: 12, Type: "modified", Key: "/registry/pods/spec/daemon-x", Value: encodeStored(t, running)},
	}
	var dialed []string
	previous := workloads.WatchDialer
	defer func() { workloads.WatchDialer = previous }()
	workloads.WatchDialer = func(_ context.Context, rawURL string) (<-chan []byte, func(), error) {
		dialed = append(dialed, rawURL)
		since, err := strconv.ParseInt(rawURL[strings.LastIndex(rawURL, "=")+1:], 10, 64)
		if err != nil {
			t.Fatalf("watch URL %s: %v", rawURL, err)
		}
		msgs := make(chan []byte, len(history))
		for _, write := range history {
			if write.Rev > since {
				msg, _ := json.Marshal(write)
				msgs <- msg
			}
		}
		return msgs, func() {}, nil
	}
	ctx := context.Background()

	listedAt.Store(10)
	if _, err := workloads.Sync(ctx, client, []byte("ca"), nil, nil, []string{"daemonsets"}); err != nil {
		t.Fatal(err)
	}
	if deletedPod(client) {
		t.Fatal("the first pass deleted a running daemon pod")
	}

	history = writesAfterTheFirstPass
	listedAt.Store(12)
	if _, err := workloads.Sync(ctx, client, []byte("ca"), nil, nil, []string{"pods"}); err != nil {
		t.Fatal(err)
	}
	if !deletedPod(client) {
		t.Fatalf("the DaemonSet controller never deleted the pod that was Failed between its passes (dialed %v)", dialed)
	}
}

func encodeStored(t *testing.T, obj runtime.Object) string {
	t.Helper()
	data, err := runtime.Encode(scheme.Codecs.LegacyCodec(v1.SchemeGroupVersion), obj)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func deletedPod(client *fake.Clientset) bool {
	for _, action := range client.Actions() {
		if action.GetVerb() == "delete" && action.GetResource().Resource == "pods" {
			return true
		}
	}
	return false
}
