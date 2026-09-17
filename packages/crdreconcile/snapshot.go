package crdreconcile

import (
	"sync"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/tools/cache"
)

type snapshotInformer struct {
	cache.SharedIndexInformer
	mu       sync.Mutex
	handlers []cache.ResourceEventHandler
}

func (s *snapshotInformer) HasSynced() bool                     { return true }
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

func (s *snapshotInformer) replay(crds []*apiextensionsv1.CustomResourceDefinition) {
	s.mu.Lock()
	handlers := append([]cache.ResourceEventHandler(nil), s.handlers...)
	s.mu.Unlock()
	for _, crd := range crds {
		for _, h := range handlers {
			h.OnAdd(crd, true)
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
