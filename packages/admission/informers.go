package admission

import (
	"context"
	"fmt"
	"sort"
	"time"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/client-go/informers"
	informerscore "k8s.io/client-go/informers/core"
	"k8s.io/client-go/informers/internalinterfaces"
	informersscheduling "k8s.io/client-go/informers/scheduling"
	informersstorage "k8s.io/client-go/informers/storage"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
)

type dummyClient struct {
	kubernetes.Interface
}

type limitRangeIndexerClient struct {
	kubernetes.Interface
	indexer cache.Indexer
}

func (c *limitRangeIndexerClient) CoreV1() corev1client.CoreV1Interface {
	return &limitRangeIndexerCoreV1{indexer: c.indexer}
}

type limitRangeIndexerCoreV1 struct {
	corev1client.CoreV1Interface
	indexer cache.Indexer
}

func (c *limitRangeIndexerCoreV1) LimitRanges(ns string) corev1client.LimitRangeInterface {
	return &limitRangeIndexerNamespaced{indexer: c.indexer, namespace: ns}
}

type limitRangeIndexerNamespaced struct {
	corev1client.LimitRangeInterface
	indexer   cache.Indexer
	namespace string
}

func (n *limitRangeIndexerNamespaced) List(_ context.Context, _ metav1.ListOptions) (*corev1.LimitRangeList, error) {
	items, err := n.indexer.ByIndex(cache.NamespaceIndex, n.namespace)
	if err != nil {
		return nil, err
	}
	result := &corev1.LimitRangeList{}
	for _, item := range items {
		lr := item.(*corev1.LimitRange)
		result.Items = append(result.Items, *lr)
	}
	return result, nil
}

type storeInformer struct {
	indexer cache.Indexer
}

func (s storeInformer) AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}
func (s storeInformer) AddEventHandlerWithResyncPeriod(handler cache.ResourceEventHandler, resyncPeriod time.Duration) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}
func (s storeInformer) AddEventHandlerWithOptions(handler cache.ResourceEventHandler, options cache.HandlerOptions) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}
func (s storeInformer) RemoveEventHandler(handle cache.ResourceEventHandlerRegistration) error {
	return nil
}
func (s storeInformer) GetStore() cache.Store {
	return s.indexer
}
func (s storeInformer) GetController() cache.Controller {
	return nil
}
func (s storeInformer) Run(stopCh <-chan struct{})         {}
func (s storeInformer) RunWithContext(ctx context.Context) {}
func (s storeInformer) HasSynced() bool {
	return true
}

type doneChecker struct{}

func (doneChecker) Name() string { return "store" }
func (doneChecker) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (s storeInformer) HasSyncedChecker() cache.DoneChecker {
	return doneChecker{}
}
func (s storeInformer) LastSyncResourceVersion() string {
	return ""
}
func (s storeInformer) SetWatchErrorHandler(handler cache.WatchErrorHandler) error {
	return nil
}
func (s storeInformer) SetWatchErrorHandlerWithContext(handler cache.WatchErrorHandlerWithContext) error {
	return nil
}
func (s storeInformer) SetTransform(handler cache.TransformFunc) error {
	return nil
}
func (s storeInformer) IsStopped() bool {
	return false
}
func (s storeInformer) AddIndexers(indexers cache.Indexers) error {
	return s.indexer.AddIndexers(indexers)
}
func (s storeInformer) GetIndexer() cache.Indexer {
	return s.indexer
}

type storeIndexer[T runtime.Object] struct {
	ctx         context.Context
	fail        func(error)
	store       *store
	prefix      string
	namespaced  bool
	items       map[string]T
	byNamespace map[string][]T
	allLoaded   bool
}

func newStoreIndexer[T runtime.Object](ctx context.Context, s *store, prefix string, namespaced bool, fail func(error)) *storeIndexer[T] {
	return &storeIndexer[T]{
		ctx:         ctx,
		fail:        fail,
		store:       s,
		prefix:      prefix,
		namespaced:  namespaced,
		items:       make(map[string]T),
		byNamespace: make(map[string][]T),
	}
}

func (idx *storeIndexer[T]) Add(obj any) error    { return nil }
func (idx *storeIndexer[T]) Update(obj any) error { return nil }
func (idx *storeIndexer[T]) Delete(obj any) error { return nil }
func (idx *storeIndexer[T]) Replace(list []any, rv string) error {
	return nil
}
func (idx *storeIndexer[T]) Resync() error { return nil }

func (idx *storeIndexer[T]) LastStoreSyncResourceVersion() string { return "" }
func (idx *storeIndexer[T]) Bookmark(rv string)                   {}

func (idx *storeIndexer[T]) Get(obj any) (any, bool, error) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		return nil, false, err
	}
	return idx.GetByKey(key)
}

func (idx *storeIndexer[T]) GetByKey(key string) (any, bool, error) {
	if item, ok := idx.items[key]; ok {
		return item, true, nil
	}
	if idx.allLoaded {
		return nil, false, nil
	}
	if idx.store == nil || idx.store.client == nil {
		return nil, false, nil
	}
	obj, exists, err := getJSON[T](idx.ctx, idx.store.client, idx.prefix+key)
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, nil
	}
	idx.items[key] = obj
	return obj, true, nil
}

func (idx *storeIndexer[T]) List() []any {
	if !idx.allLoaded {
		if idx.store != nil && idx.store.client != nil {
			list, err := listPrefix[T](idx.ctx, idx.store.client, idx.prefix)
			if err != nil {
				idx.fail(err)
				return nil
			}
			for _, item := range list {
				if accessor, err := meta.Accessor(item); err == nil {
					idx.items[idx.keyOf(accessor)] = item
				}
			}
		}
		idx.allLoaded = true
	}
	return idx.sortedItems(func(T) bool { return true })
}

func (idx *storeIndexer[T]) keyOf(accessor metav1.Object) string {
	if idx.namespaced && accessor.GetNamespace() != "" {
		return accessor.GetNamespace() + "/" + accessor.GetName()
	}
	return accessor.GetName()
}

func (idx *storeIndexer[T]) sortedItems(keep func(T) bool) []any {
	keys := make([]string, 0, len(idx.items))
	for key, item := range idx.items {
		if keep(item) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make([]any, len(keys))
	for i, key := range keys {
		out[i] = idx.items[key]
	}
	return out
}

func (idx *storeIndexer[T]) ListKeys() []string {
	items := idx.List()
	keys := make([]string, len(items))
	for i, item := range items {
		accessor, _ := meta.Accessor(item)
		if idx.namespaced && accessor.GetNamespace() != "" {
			keys[i] = accessor.GetNamespace() + "/" + accessor.GetName()
		} else {
			keys[i] = accessor.GetName()
		}
	}
	return keys
}

func (idx *storeIndexer[T]) ByIndex(indexName, indexedValue string) ([]any, error) {
	if indexName != cache.NamespaceIndex {
		return nil, fmt.Errorf("storeIndexer: unsupported index %q", indexName)
	}
	if !idx.namespaced {
		return idx.List(), nil
	}
	if cached, ok := idx.byNamespace[indexedValue]; ok {
		out := make([]any, len(cached))
		for i, v := range cached {
			out[i] = v
		}
		return out, nil
	}
	if idx.allLoaded {
		return idx.sortedItems(func(item T) bool {
			accessor, err := meta.Accessor(item)
			return err == nil && accessor.GetNamespace() == indexedValue
		}), nil
	}
	if idx.store == nil || idx.store.client == nil {
		return nil, nil
	}
	list, err := listPrefix[T](idx.ctx, idx.store.client, idx.prefix+indexedValue+"/")
	if err != nil {
		return nil, err
	}
	idx.byNamespace[indexedValue] = list
	for _, item := range list {
		accessor, err := meta.Accessor(item)
		if err == nil {
			idx.items[indexedValue+"/"+accessor.GetName()] = item
		}
	}
	out := make([]any, len(list))
	for i, v := range list {
		out[i] = v
	}
	return out, nil
}

func (idx *storeIndexer[T]) Index(indexName string, obj any) ([]any, error) {
	if indexName == cache.NamespaceIndex {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			return nil, err
		}
		return idx.ByIndex(indexName, accessor.GetNamespace())
	}
	return nil, fmt.Errorf("storeIndexer: unsupported index %q", indexName)
}

func (idx *storeIndexer[T]) IndexKeys(indexName, indexedValue string) ([]string, error) {
	items, err := idx.ByIndex(indexName, indexedValue)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(items))
	for i, item := range items {
		accessor, err := meta.Accessor(item)
		if err != nil {
			return nil, err
		}
		if idx.namespaced && accessor.GetNamespace() != "" {
			keys[i] = accessor.GetNamespace() + "/" + accessor.GetName()
		} else {
			keys[i] = accessor.GetName()
		}
	}
	return keys, nil
}

func (idx *storeIndexer[T]) ListIndexFuncValues(indexName string) []string {
	idx.fail(fmt.Errorf("storeIndexer: unsupported ListIndexFuncValues for %q", indexName))
	return nil
}

func (idx *storeIndexer[T]) AddIndexers(newIndexers cache.Indexers) error {
	for name := range newIndexers {
		if name != cache.NamespaceIndex {
			return fmt.Errorf("storeIndexer: unsupported indexer %q", name)
		}
	}
	return nil
}

func (idx *storeIndexer[T]) GetIndexers() cache.Indexers {
	return cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}
}

type storeInformerFactory struct {
	informers.SharedInformerFactory
	ctx           context.Context
	store         *store
	priorityClass cache.Indexer
	limitRange    cache.Indexer
	storageClass  cache.Indexer
	failure       error
}

func newStoreInformerFactory(ctx context.Context, s *store) *storeInformerFactory {
	f := &storeInformerFactory{
		SharedInformerFactory: informers.NewSharedInformerFactory(nil, 0),
		ctx:                   ctx,
		store:                 s,
	}
	f.priorityClass = newStoreIndexer[*schedulingv1.PriorityClass](ctx, s, "/registry/priorityclasses/", false, f.fail)
	f.limitRange = newStoreIndexer[*corev1.LimitRange](ctx, s, "/registry/limitranges/", true, f.fail)
	f.storageClass = newStoreIndexer[*storagev1.StorageClass](ctx, s, "/registry/storageclasses/", false, f.fail)
	return f
}

func (f *storeInformerFactory) fail(err error) {
	if f.failure == nil {
		f.failure = err
	}
}

func runWithFactory(ctx context.Context, plugin admission.Interface, f *storeInformerFactory, req *admit.Request) error {
	err := runUpstreamPlugin(ctx, plugin, req)
	if f.failure != nil {
		return apierrors.NewInternalError(f.failure)
	}
	return err
}

func (f *storeInformerFactory) Scheduling() informersscheduling.Interface {
	return informersscheduling.New(f, metav1.NamespaceAll, nil)
}

func (f *storeInformerFactory) Core() informerscore.Interface {
	return informerscore.New(f, metav1.NamespaceAll, nil)
}

func (f *storeInformerFactory) Storage() informersstorage.Interface {
	return informersstorage.New(f, metav1.NamespaceAll, nil)
}

func (f *storeInformerFactory) InformerName() *cache.InformerName { return nil }

func (f *storeInformerFactory) InformerFor(obj runtime.Object, newFunc internalinterfaces.NewInformerFunc) cache.SharedIndexInformer {
	switch obj.(type) {
	case *schedulingv1.PriorityClass:
		return storeInformer{indexer: f.priorityClass}
	case *corev1.LimitRange:
		return storeInformer{indexer: f.limitRange}
	case *storagev1.StorageClass:
		return storeInformer{indexer: f.storageClass}
	default:
		return f.SharedInformerFactory.InformerFor(obj, newFunc)
	}
}
