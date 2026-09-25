package workloads

import (
	"sync"
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
}

func newSnapshotInformer(example runtime.Object) *snapshotInformer {
	return &snapshotInformer{
		example:             example,
		SharedIndexInformer: cache.NewSharedIndexInformer(nil, example, 0, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}),
	}
}

func (s *snapshotInformer) HasSynced() bool { return true }

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
	for _, o := range objs {
		for _, h := range handlers {
			h.OnAdd(o, true)
		}
	}
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
