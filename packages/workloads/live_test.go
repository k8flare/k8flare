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
	"k8s.io/apimachinery/pkg/types"
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
	feed := followWrites(ctx, all, []int64{10, 12}, 0)
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
	if feed := followWrites(context.Background(), all, []int64{1}, 0); feed != nil || feed.live() || feed.count() != 0 {
		t.Fatal("a feed without a dialer")
	}
	WatchDialer = func(context.Context, string) (<-chan []byte, func(), error) {
		return nil, nil, context.DeadlineExceeded
	}
	feed := followWrites(context.Background(), all, []int64{1}, 0)
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
	if drain(func(string) bool { return true }, 300*time.Millisecond, time.Millisecond, time.Time{}, func() bool { return true }, nil) {
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
	feed := followWrites(ctx, all, []int64{10}, 0)
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

func TestDrainAdoptsAnOfferedChangeOnlyWhileLive(t *testing.T) {
	work.reset(workloadQueue)
	defer work.reset(workloadQueue)
	work.of("replicationmanager").depth.Store(1)
	for _, live := range []bool{false, true} {
		adopted := make(chan []string, 1)
		drained := make(chan struct{})
		go func() {
			drain(workloadQueue, 300*time.Millisecond, time.Millisecond, time.Time{}, func() bool { return live }, func(a *adoption) { adopted <- a.changed })
			close(drained)
		}()
		var handed *adoption
		for deadline := time.Now().Add(250 * time.Millisecond); handed == nil && time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
			handed = offer([]string{"configmaps"})
		}
		<-drained
		if live != (handed != nil) {
			t.Fatalf("live=%v: offer accepted=%v", live, handed != nil)
		}
		select {
		case got := <-adopted:
			if !live || len(got) != 1 || got[0] != "configmaps" {
				t.Fatalf("live=%v: adopted %v", live, got)
			}
		default:
			if live {
				t.Fatal("a live pass accepted the offer without adopting it")
			}
		}
	}
}

func TestLiveFeedKeepsOnlyATerminalPhaseFromBeforeTheList(t *testing.T) {
	previous := WatchDialer
	defer func() { WatchDialer = previous }()
	msgs := make(chan []byte, 8)
	var dialed string
	WatchDialer = func(_ context.Context, rawURL string) (<-chan []byte, func(), error) {
		dialed = rawURL
		return msgs, func() {}, nil
	}
	pod := func(name string, uid types.UID, rev string, phase corev1.PodPhase) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: uid, ResourceVersion: rev}, Status: corev1.PodStatus{Phase: phase}}
	}
	pods := newSnapshotInformer(&corev1.Pod{})
	pods.fill([]runtime.Object{pod("reverted", "a", "12", corev1.PodRunning), pod("recreated", "b2", "12", corev1.PodRunning), pod("started", "c", "12", corev1.PodRunning)})
	events := &recordedEvents{}
	if _, err := pods.AddEventHandler(events); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	feed := followWrites(ctx, []loadedSource{{informer: pods}}, []int64{12}, 9)
	msgs <- storedEvent(t, 10, "modified", "/registry/pods/default/started", pod("started", "c", "10", corev1.PodPending))
	msgs <- storedEvent(t, 10, "modified", "/registry/pods/default/recreated", pod("recreated", "b1", "10", corev1.PodFailed))
	msgs <- storedEvent(t, 11, "modified", "/registry/pods/default/reverted", pod("reverted", "a", "11", corev1.PodFailed))
	msgs <- storedEvent(t, 12, "modified", "/registry/pods/default/reverted", pod("reverted", "a", "12", corev1.PodRunning))
	msgs <- storedEvent(t, 13, "modified", "/registry/pods/default/started", pod("started", "c", "13", corev1.PodRunning))
	deadline := time.Now().Add(5 * time.Second)
	for feed.reach() < 13 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if dialed != liveWatchBase+"9" {
		t.Fatalf("dialed %s, want the history from 9", dialed)
	}
	phase := func(name string) (corev1.PodPhase, string) {
		held, _, _ := pods.GetIndexer().GetByKey("default/" + name)
		return held.(*corev1.Pod).Status.Phase, held.(*corev1.Pod).ResourceVersion
	}
	if got, rev := phase("reverted"); got != corev1.PodFailed || rev != "11" {
		t.Fatalf("reverted pod held as %s at %s, want Failed at 11", got, rev)
	}
	if got, _ := phase("recreated"); got != corev1.PodRunning {
		t.Fatalf("a Failed pod of an earlier UID replaced the listed one: %s", got)
	}
	if got, rev := phase("started"); got != corev1.PodRunning || rev != "13" {
		t.Fatalf("started pod held as %s at %s, want the write after the list", got, rev)
	}
	if len(events.updated) != 2 || events.updated[0] != "reverted" || events.updated[1] != "started" {
		t.Fatalf("handler saw updates %v", events.updated)
	}
}

func TestLiveFeedFollowsFromTheListsWhenItsHistoryIsCompacted(t *testing.T) {
	previous := WatchDialer
	defer func() { WatchDialer = previous }()
	var dialed []string
	WatchDialer = func(_ context.Context, rawURL string) (<-chan []byte, func(), error) {
		dialed = append(dialed, rawURL)
		msgs := make(chan []byte, 1)
		if len(dialed) == 1 {
			compacted, _ := json.Marshal(liveEvent{Rev: 7, Type: "compacted"})
			msgs <- compacted
			close(msgs)
		}
		return msgs, func() {}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	feed := followWrites(ctx, []loadedSource{{informer: newSnapshotInformer(&corev1.Pod{})}}, []int64{10}, 5)
	deadline := time.Now().Add(5 * time.Second)
	for (len(dialed) < 2 || !feed.live()) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(dialed) != 2 || dialed[0] != liveWatchBase+"5" || dialed[1] != liveWatchBase+"10" {
		t.Fatalf("dialed %v, want the history from 5 and then the lists at 10", dialed)
	}
	if !feed.live() {
		t.Fatal("the feed did not stay live after falling back to the lists")
	}
}

func TestLiveFeedIsNotLiveOnceItStops(t *testing.T) {
	previous := WatchDialer
	defer func() { WatchDialer = previous }()
	compacted, _ := json.Marshal(liveEvent{Rev: 10, Type: "compacted"})
	for name, end := range map[string]func(chan []byte){
		"closed":              func(msgs chan []byte) { close(msgs) },
		"compacted at listed": func(msgs chan []byte) { msgs <- compacted },
	} {
		msgs := make(chan []byte, 1)
		WatchDialer = func(context.Context, string) (<-chan []byte, func(), error) {
			return msgs, func() {}, nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		feed := followWrites(ctx, []loadedSource{{informer: newSnapshotInformer(&corev1.Pod{})}}, []int64{10}, 0)
		for deadline := time.Now().Add(5 * time.Second); !feed.live() && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		}
		if !feed.live() {
			cancel()
			t.Fatalf("%s: the feed never reported itself live", name)
		}
		end(msgs)
		select {
		case <-feed.done:
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatalf("%s: the feed did not stop", name)
		}
		cancel()
		if feed.live() {
			t.Fatalf("%s: a stopped feed still reports itself live", name)
		}
	}
}
