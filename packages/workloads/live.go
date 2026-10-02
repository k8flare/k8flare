package workloads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

var WatchDialer func(ctx context.Context, rawURL string) (<-chan []byte, func(), error)

const liveWatchBase = "http://cluster.internal/watch?prefix=/registry/&since="

var liveSkippedPrefixes = []string{"/registry/events/", "/registry/leases/", "/registry/secrets/"}

type liveEvent struct {
	Rev   int64  `json:"rev"`
	Type  string `json:"type"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type liveFeed struct {
	informers map[reflect.Type]*snapshotInformer
	listed    map[reflect.Type]int64
	connected atomic.Bool
	applied   atomic.Int64
	done      chan struct{}
}

func (f *liveFeed) live() bool { return f != nil && f.connected.Load() }

func (f *liveFeed) count() int64 {
	if f == nil {
		return 0
	}
	return f.applied.Load()
}

func followWrites(ctx context.Context, all []loadedSource, revisions []int64) *liveFeed {
	if WatchDialer == nil || len(all) == 0 {
		return nil
	}
	f := &liveFeed{informers: map[reflect.Type]*snapshotInformer{}, listed: map[reflect.Type]int64{}, done: make(chan struct{})}
	since := int64(0)
	for i, l := range all {
		t := reflect.TypeOf(l.informer.example)
		f.informers[t] = l.informer
		f.listed[t] = revisions[i]
		if since == 0 || (revisions[i] > 0 && revisions[i] < since) {
			since = revisions[i]
		}
	}
	go f.run(ctx, since)
	return f
}

func (f *liveFeed) run(ctx context.Context, since int64) {
	defer close(f.done)
	msgs, closeFn, err := WatchDialer(ctx, liveWatchBase+strconv.FormatInt(since, 10))
	if err != nil {
		println("workloads: live feed dial failed:", err.Error())
		return
	}
	defer closeFn()
	f.connected.Store(true)
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				println("workloads: live feed closed")
				return
			}
			var ev liveEvent
			if err := json.Unmarshal(msg, &ev); err != nil {
				continue
			}
			if ev.Type == "compacted" {
				println("workloads: live feed compacted at", strconv.FormatInt(ev.Rev, 10))
				return
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
	obj, err := decodeStored(ev.Value)
	if err != nil {
		return false
	}
	informer := f.informers[reflect.TypeOf(obj)]
	if informer == nil || ev.Rev <= f.listed[reflect.TypeOf(obj)] {
		return false
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
