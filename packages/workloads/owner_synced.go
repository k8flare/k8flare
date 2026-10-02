package workloads

import (
	"time"

	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type ownerSyncedPodInformer struct {
	coreinformers.PodInformer
}

func OwnerSyncedPodInformer(inner coreinformers.PodInformer) coreinformers.PodInformer {
	return ownerSyncedPodInformer{inner}
}

func (p ownerSyncedPodInformer) Informer() cache.SharedIndexInformer {
	return ownerSyncedSharedIndexInformer{p.PodInformer.Informer()}
}

type ownerSyncedSharedIndexInformer struct {
	cache.SharedIndexInformer
}

func (s ownerSyncedSharedIndexInformer) AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	return s.SharedIndexInformer.AddEventHandler(ownerSyncedEventHandler{handler})
}

func (s ownerSyncedSharedIndexInformer) AddEventHandlerWithResyncPeriod(handler cache.ResourceEventHandler, resyncPeriod time.Duration) (cache.ResourceEventHandlerRegistration, error) {
	return s.SharedIndexInformer.AddEventHandlerWithResyncPeriod(ownerSyncedEventHandler{handler}, resyncPeriod)
}

func (s ownerSyncedSharedIndexInformer) AddEventHandlerWithOptions(handler cache.ResourceEventHandler, options cache.HandlerOptions) (cache.ResourceEventHandlerRegistration, error) {
	return s.SharedIndexInformer.AddEventHandlerWithOptions(ownerSyncedEventHandler{handler}, options)
}

type ownerSyncedEventHandler struct {
	cache.ResourceEventHandler
}

func (h ownerSyncedEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	pending.withoutBooking(func() {
		h.ResourceEventHandler.OnAdd(obj, isInInitialList)
	})
}

func (h ownerSyncedEventHandler) OnUpdate(oldObj, newObj interface{}) {
	pending.withoutBooking(func() {
		h.ResourceEventHandler.OnUpdate(oldObj, newObj)
	})
}

func (h ownerSyncedEventHandler) OnDelete(obj interface{}) {
	pending.withoutBooking(func() {
		h.ResourceEventHandler.OnDelete(obj)
	})
}
