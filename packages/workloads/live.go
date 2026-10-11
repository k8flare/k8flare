package workloads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

var WatchDialer func(ctx context.Context, rawURL string) (<-chan []byte, func(), error)

const liveWatchBase = "http://cluster.internal/watch?prefix=/registry/&since="

var liveSkippedPrefixes = []string{"/registry/events/", "/registry/leases/", "/registry/secrets/"}

const livePodPrefix = "/registry/pods/"

type liveEvent struct {
	Rev   int64  `json:"rev"`
	Type  string `json:"type"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type liveFeed struct {
	mu        sync.Mutex
	informers map[reflect.Type]*snapshotInformer
	listed    map[reflect.Type]int64
	floor     int64
	connected atomic.Bool
	applied   atomic.Int64
	reached   atomic.Int64
	done      chan struct{}
}

func (f *liveFeed) live() bool { return f != nil && f.connected.Load() }

func (f *liveFeed) count() int64 {
	if f == nil {
		return 0
	}
	return f.applied.Load()
}

func followWrites(ctx context.Context, all []loadedSource, revisions []int64, from int64) *liveFeed {
	if WatchDialer == nil || len(all) == 0 {
		return nil
	}
	f := &liveFeed{informers: map[reflect.Type]*snapshotInformer{}, listed: map[reflect.Type]int64{}, floor: listedFloor(revisions), done: make(chan struct{})}
	for i, l := range all {
		t := reflect.TypeOf(l.informer.example)
		f.informers[t] = l.informer
		f.listed[t] = revisions[i]
	}
	since := f.floor
	if from > 0 && from < since {
		since = from
	}
	go f.run(ctx, since)
	return f
}

func listedFloor(revisions []int64) int64 {
	floor := int64(0)
	for _, revision := range revisions {
		if floor == 0 || (revision > 0 && revision < floor) {
			floor = revision
		}
	}
	return floor
}

func (f *liveFeed) reach() int64 {
	if f == nil {
		return 0
	}
	return max(f.floor, f.reached.Load())
}

func (f *liveFeed) follow(informer *snapshotInformer, listed int64) {
	t := reflect.TypeOf(informer.example)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.informers[t] = informer
	f.listed[t] = listed
}

func (f *liveFeed) run(ctx context.Context, since int64) {
	defer close(f.done)
	if f.watch(ctx, since) && since < f.floor {
		println("workloads: live feed history compacted, following from", strconv.FormatInt(f.floor, 10))
		f.watch(ctx, f.floor)
	}
}

func (f *liveFeed) watch(ctx context.Context, since int64) bool {
	msgs, closeFn, err := WatchDialer(ctx, liveWatchBase+strconv.FormatInt(since, 10))
	if err != nil {
		println("workloads: live feed dial failed:", err.Error())
		return false
	}
	defer closeFn()
	f.connected.Store(true)
	for {
		select {
		case <-ctx.Done():
			return false
		case msg, ok := <-msgs:
			if !ok {
				println("workloads: live feed closed")
				return false
			}
			var ev liveEvent
			if err := json.Unmarshal(msg, &ev); err != nil {
				continue
			}
			if ev.Type == "compacted" {
				println("workloads: live feed compacted at", strconv.FormatInt(ev.Rev, 10))
				return true
			}
			if ev.Rev > f.reached.Load() {
				f.reached.Store(ev.Rev)
			}
			if f.apply(ev) {
				f.applied.Add(1)
			}
		}
	}
}

func (f *liveFeed) apply(ev liveEvent) bool {
	if ev.Key == "" || ev.Value == "" {
		return false
	}
	for _, prefix := range liveSkippedPrefixes {
		if strings.HasPrefix(ev.Key, prefix) {
			return false
		}
	}
	if ev.Rev <= f.floor && !strings.HasPrefix(ev.Key, livePodPrefix) {
		return false
	}
	obj, err := decodeStored(ev.Value)
	if err != nil {
		return false
	}
	f.mu.Lock()
	informer, listed := f.informers[reflect.TypeOf(obj)], f.listed[reflect.TypeOf(obj)]
	f.mu.Unlock()
	if informer == nil {
		return false
	}
	if ev.Rev <= listed {
		return keepTerminal(informer, obj, ev)
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return false
	}
	accessor.SetResourceVersion(strconv.FormatInt(ev.Rev, 10))
	if ev.Type == "deleted" {
		informer.forget(accessor.GetNamespace(), accessor.GetName())
		return true
	}
	if held := heldRevision(informer, accessor.GetNamespace(), accessor.GetName()); held >= ev.Rev {
		return false
	}
	informer.note(obj)
	return true
}

func keepTerminal(informer *snapshotInformer, obj runtime.Object, ev liveEvent) bool {
	pod, ok := obj.(*corev1.Pod)
	if !ok || ev.Type == "deleted" || !podTerminal(pod) {
		return false
	}
	held, exists, _ := informer.SharedIndexInformer.GetIndexer().GetByKey(pod.Namespace + "/" + pod.Name)
	if !exists || held.(*corev1.Pod).UID != pod.UID || podTerminal(held.(*corev1.Pod)) {
		return false
	}
	pod.ResourceVersion = strconv.FormatInt(ev.Rev, 10)
	println("workloads: live feed keeps", string(pod.Status.Phase), "of pod", pod.Namespace+"/"+pod.Name, "from", strconv.FormatInt(ev.Rev, 10))
	informer.note(pod)
	return true
}

func podTerminal(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded
}

func heldRevision(informer *snapshotInformer, namespace, name string) int64 {
	key := name
	if namespace != "" {
		key = namespace + "/" + name
	}
	previous, exists, _ := informer.SharedIndexInformer.GetIndexer().GetByKey(key)
	if !exists {
		return 0
	}
	accessor, err := meta.Accessor(previous)
	if err != nil {
		return 0
	}
	rev, _ := strconv.ParseInt(accessor.GetResourceVersion(), 10, 64)
	return rev
}

func decodeStored(b64 string) (runtime.Object, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(data, nil, nil)
	return obj, err
}
