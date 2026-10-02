package workloads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
)

func never() bool { return false }

func storedEvent(t *testing.T, rev int64, kind, key string, obj runtime.Object) []byte {
	t.Helper()
	data, err := runtime.Encode(scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion, discoveryv1.SchemeGroupVersion), obj)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := json.Marshal(liveEvent{Rev: rev, Type: kind, Key: key, Value: base64.StdEncoding.EncodeToString(data)})
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

type recordedEvents struct {
	added, updated, deleted []string
}

func (r *recordedEvents) OnAdd(obj interface{}, _ bool) {
	r.added = append(r.added, obj.(metav1.Object).GetName())
}

func (r *recordedEvents) OnUpdate(_, obj interface{}) {
	r.updated = append(r.updated, obj.(metav1.Object).GetName())
}

func (r *recordedEvents) OnDelete(obj interface{}) {
	r.deleted = append(r.deleted, obj.(metav1.Object).GetName())
}

func TestLiveFeedNotesWritesAfterTheListIntoTheSnapshot(t *testing.T) {
	previous := WatchDialer
	defer func() { WatchDialer = previous }()
	msgs := make(chan []byte, 8)
	var dialed string
	WatchDialer = func(_ context.Context, rawURL string) (<-chan []byte, func(), error) {
		dialed = rawURL
		return msgs, func() {}, nil
	}
	endpoints := newSnapshotInformer(&corev1.Endpoints{})
	slices := newSnapshotInformer(&discoveryv1.EndpointSlice{})
	listed := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "listed", Namespace: "default", ResourceVersion: "9"}}
	endpoints.fill([]runtime.Object{listed})
	events := &recordedEvents{}
	if _, err := endpoints.AddEventHandler(events); err != nil {
		t.Fatal(err)
	}
	all := []loadedSource{{informer: endpoints, objs: []runtime.Object{listed}}, {informer: slices}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	feed := followWrites(ctx, all, []int64{10, 12})
	if feed == nil {
		t.Fatal("no feed with a dialer installed")
	}
	msgs <- storedEvent(t, 10, "modified", "/registry/endpoints/default/listed", &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "listed", Namespace: "default", ResourceVersion: "10"}})
	msgs <- storedEvent(t, 11, "created", "/registry/endpoints/default/custom", &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "custom", Namespace: "default", ResourceVersion: "11"}})
	msgs <- storedEvent(t, 12, "modified", "/registry/endpoints/default/custom", &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "custom", Namespace: "default", ResourceVersion: "12", Labels: map[string]string{"seen": "1"}}})
	msgs <- storedEvent(t, 13, "created", "/registry/endpointslices/default/other", &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "default", ResourceVersion: "13"}, AddressType: discoveryv1.AddressTypeIPv4})
	msgs <- storedEvent(t, 14, "created", "/registry/events/default/noise", &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "noise", Namespace: "default", ResourceVersion: "14"}})
	msgs <- storedEvent(t, 15, "created", "/registry/configmaps/default/untracked", &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "untracked", Namespace: "default", ResourceVersion: "15"}})
	msgs <- storedEvent(t, 16, "deleted", "/registry/endpoints/default/listed", listed)
	deadline := time.Now().Add(5 * time.Second)
	for feed.count() < 4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := feed.count(); got != 4 {
		t.Fatalf("applied %d events, want 4", got)
	}
	if dialed != liveWatchBase+"10" {
		t.Fatalf("dialed %s", dialed)
	}
	if !feed.live() {
		t.Fatal("feed did not report itself live")
	}
	custom, exists, err := endpoints.GetIndexer().GetByKey("default/custom")
	if err != nil || !exists {
		t.Fatalf("custom endpoints missing: exists=%v err=%v", exists, err)
	}
	if custom.(metav1.Object).GetLabels()["seen"] != "1" {
		t.Fatalf("snapshot kept an older copy: %v", custom.(metav1.Object).GetLabels())
	}
	if _, exists, _ := endpoints.GetIndexer().GetByKey("default/listed"); exists {
		t.Fatal("deleted endpoints stayed in the snapshot")
	}
	if _, exists, _ := slices.GetIndexer().GetByKey("default/other"); !exists {
		t.Fatal("endpointslice create was not noted")
	}
	if len(events.added) != 1 || events.added[0] != "custom" || len(events.updated) != 1 || len(events.deleted) != 1 {
		t.Fatalf("handler saw added=%v updated=%v deleted=%v", events.added, events.updated, events.deleted)
	}
	close(msgs)
	select {
	case <-feed.done:
	case <-time.After(5 * time.Second):
		t.Fatal("feed did not stop when the socket closed")
	}
}

func TestLiveFeedIsAbsentWithoutADialerOrWhenTheDialFails(t *testing.T) {
	previous := WatchDialer
	defer func() { WatchDialer = previous }()
	WatchDialer = nil
	all := []loadedSource{{informer: newSnapshotInformer(&corev1.Pod{})}}
	if feed := followWrites(context.Background(), all, []int64{1}); feed != nil || feed.live() || feed.count() != 0 {
		t.Fatal("a feed without a dialer")
	}
	WatchDialer = func(context.Context, string) (<-chan []byte, func(), error) {
		return nil, nil, context.DeadlineExceeded
	}
	feed := followWrites(context.Background(), all, []int64{1})
	select {
	case <-feed.done:
	case <-time.After(5 * time.Second):
		t.Fatal("feed did not give up after a failed dial")
	}
	if feed.live() {
		t.Fatal("a failed dial reported live")
	}
}

func TestDrainIgnoresYieldWhileLive(t *testing.T) {
	work.reset(func(string) bool { return true })
	work.of("replicationmanager").depth.Store(1)
	yieldRequested.Store(true)
	defer yieldRequested.Store(false)
	started := time.Now()
	if drain(func(string) bool { return true }, 300*time.Millisecond, time.Millisecond, time.Time{}, func() bool { return true }) {
		t.Fatal("queued work reported drained")
	}
	if time.Since(started) < 250*time.Millisecond {
		t.Fatalf("a live pass yielded after %s", time.Since(started))
	}
}

var _ cache.ResourceEventHandler = (*recordedEvents)(nil)

func TestLiveFeedGivesAnObjectTheRevisionItWasStoredAt(t *testing.T) {
	previous := WatchDialer
	defer func() { WatchDialer = previous }()
	msgs := make(chan []byte, 2)
	WatchDialer = func(context.Context, string) (<-chan []byte, func(), error) {
		return msgs, func() {}, nil
	}
	endpoints := newSnapshotInformer(&corev1.Endpoints{})
	all := []loadedSource{{informer: endpoints}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	feed := followWrites(ctx, all, []int64{10})
	msgs <- storedEvent(t, 11, "created", "/registry/endpoints/default/stored", &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "stored", Namespace: "default"}})
	deadline := time.Now().Add(5 * time.Second)
	for feed.count() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	held, exists, err := endpoints.GetIndexer().GetByKey("default/stored")
	if err != nil || !exists {
		t.Fatalf("the stored object is not in the snapshot: %v", err)
	}
	if got := held.(*corev1.Endpoints).ResourceVersion; got != "11" {
		t.Fatalf("resourceVersion %q, want the stored revision 11", got)
	}
}
