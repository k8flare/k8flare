package workloads

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
)

type snapshotInformer struct {
	cache.SharedIndexInformer
	example  runtime.Object
	mu       sync.Mutex
	handlers []cache.ResourceEventHandler
	stale    atomic.Bool
	list     func(context.Context) ([]runtime.Object, error)
	passCtx  context.Context

	ownersSyncedInPass bool
}

func (s *snapshotInformer) deliver(notify func()) {
	if s.ownersSyncedInPass {
		pending.withoutBooking(notify)
		return
	}
	notify()
}

func newSnapshotInformer(example runtime.Object) *snapshotInformer {
	return &snapshotInformer{
		example:             example,
		SharedIndexInformer: cache.NewSharedIndexInformer(nil, example, 0, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}),
	}
}

func (s *snapshotInformer) HasSynced() bool { return true }

func (s *snapshotInformer) GetIndexer() cache.Indexer {
	return catchupIndexer{Indexer: s.SharedIndexInformer.GetIndexer(), snap: s}
}

func (s *snapshotInformer) markStale() {
	if s != nil {
		s.stale.Store(true)
	}
}

func (s *snapshotInformer) HasSyncedChecker() cache.DoneChecker { return synced{} }

func (s *snapshotInformer) AddEventHandler(h cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers = append(s.handlers, h)
	return synced{}, nil
}

func (s *snapshotInformer) AddEventHandlerWithResyncPeriod(h cache.ResourceEventHandler, _ time.Duration) (cache.ResourceEventHandlerRegistration, error) {
	return s.AddEventHandler(h)
}

func (s *snapshotInformer) AddEventHandlerWithOptions(h cache.ResourceEventHandler, _ cache.HandlerOptions) (cache.ResourceEventHandlerRegistration, error) {
	return s.AddEventHandler(h)
}

func (s *snapshotInformer) fill(objs []runtime.Object) {
	for _, o := range objs {
		copied := o.DeepCopyObject()
		scheme.Scheme.Default(copied)
		s.GetIndexer().Add(copied)
	}
}

func (s *snapshotInformer) replay(objs []runtime.Object) {
	s.mu.Lock()
	handlers := append([]cache.ResourceEventHandler(nil), s.handlers...)
	s.mu.Unlock()
	s.deliver(func() {
		for _, o := range objs {
			for _, h := range handlers {
				h.OnAdd(o, true)
			}
		}
	})
}

func (s *snapshotInformer) note(obj runtime.Object) {
	if s == nil || obj == nil {
		return
	}
	copied := obj.DeepCopyObject()
	scheme.Scheme.Default(copied)
	key, err := cache.MetaNamespaceKeyFunc(copied)
	if err != nil {
		return
	}
	previous, exists, _ := s.GetIndexer().GetByKey(key)
	var old runtime.Object
	if exists {
		old = previous.(runtime.Object).DeepCopyObject()
		_ = s.GetIndexer().Update(copied)
	} else {
		_ = s.GetIndexer().Add(copied)
	}
	s.mu.Lock()
	handlers := append([]cache.ResourceEventHandler(nil), s.handlers...)
	s.mu.Unlock()
	s.deliver(func() {
		for _, h := range handlers {
			if exists {
				h.OnUpdate(old, copied)
			} else {
				h.OnAdd(copied, false)
			}
		}
	})
}

func (s *snapshotInformer) forget(namespace, name string) {
	if s == nil {
		return
	}
	key := name
	if namespace != "" {
		key = namespace + "/" + name
	}
	previous, exists, _ := s.GetIndexer().GetByKey(key)
	if !exists {
		return
	}
	old := previous.(runtime.Object).DeepCopyObject()
	_ = s.GetIndexer().Delete(old)
	s.mu.Lock()
	handlers := append([]cache.ResourceEventHandler(nil), s.handlers...)
	s.mu.Unlock()
	s.deliver(func() {
		for _, h := range handlers {
			h.OnDelete(old)
		}
	})
}

func (s *snapshotInformer) catchUp() {
	if s == nil || s.list == nil || !s.stale.CompareAndSwap(true, false) {
		return
	}
	ctx := s.passCtx
	if ctx == nil {
		ctx = context.Background()
	}
	items, err := s.list(ctx)
	if err != nil {
		s.stale.Store(true)
		return
	}
	objs := make([]interface{}, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		copied := item.DeepCopyObject()
		scheme.Scheme.Default(copied)
		objs = append(objs, copied)
	}
	_ = s.SharedIndexInformer.GetIndexer().Replace(objs, "catchup")
}

type catchupIndexer struct {
	cache.Indexer
	snap *snapshotInformer
}

func (c catchupIndexer) ByIndex(indexName, indexKey string) ([]interface{}, error) {
	c.snap.catchUp()
	return c.Indexer.ByIndex(indexName, indexKey)
}

func (c catchupIndexer) List() []interface{} {
	c.snap.catchUp()
	return c.Indexer.List()
}

var closed = func() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()

type synced struct{}

func (synced) HasSynced() bool                     { return true }
func (synced) HasSyncedChecker() cache.DoneChecker { return synced{} }
func (synced) Name() string                        { return "snapshot" }
func (synced) Done() <-chan struct{}               { return closed }
