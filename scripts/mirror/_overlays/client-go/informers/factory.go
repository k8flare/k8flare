//go:build js

package informers

import (
	context "context"
	fmt "fmt"
	reflect "reflect"
	sync "sync"
	time "time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	wait "k8s.io/apimachinery/pkg/util/wait"
	apps "k8s.io/client-go/informers/apps"
	autoscaling "k8s.io/client-go/informers/autoscaling"
	batch "k8s.io/client-go/informers/batch"
	admissionregistration "k8s.io/client-go/informers/admissionregistration"
	coordination "k8s.io/client-go/informers/coordination"
	core "k8s.io/client-go/informers/core"
	discovery "k8s.io/client-go/informers/discovery"
	internalinterfaces "k8s.io/client-go/informers/internalinterfaces"
	policy "k8s.io/client-go/informers/policy"
	resource "k8s.io/client-go/informers/resource"
	scheduling "k8s.io/client-go/informers/scheduling"
	storage "k8s.io/client-go/informers/storage"
	kubernetes "k8s.io/client-go/kubernetes"
	cache "k8s.io/client-go/tools/cache"
)

type SharedInformerOption func(*sharedInformerFactory) *sharedInformerFactory

type sharedInformerFactory struct {
	client           kubernetes.Interface
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	lock             sync.Mutex
	defaultResync    time.Duration
	customResync     map[reflect.Type]time.Duration
	transform        cache.TransformFunc
	informerName     *cache.InformerName

	informers        map[reflect.Type]cache.SharedIndexInformer
	startedInformers map[reflect.Type]bool
	wg               sync.WaitGroup
	shuttingDown     bool
}

func WithCustomResyncConfig(resyncConfig map[v1.Object]time.Duration) SharedInformerOption {
	return func(factory *sharedInformerFactory) *sharedInformerFactory {
		for k, v := range resyncConfig {
			factory.customResync[reflect.TypeOf(k)] = v
		}
		return factory
	}
}
func WithTweakListOptions(tweakListOptions internalinterfaces.TweakListOptionsFunc) SharedInformerOption {
	return func(factory *sharedInformerFactory) *sharedInformerFactory {
		factory.tweakListOptions = tweakListOptions
		return factory
	}
}
func WithNamespace(namespace string) SharedInformerOption {
	return func(factory *sharedInformerFactory) *sharedInformerFactory {
		factory.namespace = namespace
		return factory
	}
}
func WithTransform(transform cache.TransformFunc) SharedInformerOption {
	return func(factory *sharedInformerFactory) *sharedInformerFactory {
		factory.transform = transform
		return factory
	}
}
func WithInformerName(informerName *cache.InformerName) SharedInformerOption {
	return func(factory *sharedInformerFactory) *sharedInformerFactory {
		factory.informerName = informerName
		return factory
	}
}

func (f *sharedInformerFactory) InformerName() *cache.InformerName {
	return f.informerName
}
func NewSharedInformerFactory(client kubernetes.Interface, defaultResync time.Duration) SharedInformerFactory {
	return NewSharedInformerFactoryWithOptions(client, defaultResync)
}
func NewFilteredSharedInformerFactory(client kubernetes.Interface, defaultResync time.Duration, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) SharedInformerFactory {
	return NewSharedInformerFactoryWithOptions(client, defaultResync, WithNamespace(namespace), WithTweakListOptions(tweakListOptions))
}
func NewSharedInformerFactoryWithOptions(client kubernetes.Interface, defaultResync time.Duration, options ...SharedInformerOption) SharedInformerFactory {
	factory := &sharedInformerFactory{
		client:           client,
		namespace:        v1.NamespaceAll,
		defaultResync:    defaultResync,
		informers:        make(map[reflect.Type]cache.SharedIndexInformer),
		startedInformers: make(map[reflect.Type]bool),
		customResync:     make(map[reflect.Type]time.Duration),
	}

	for _, opt := range options {
		factory = opt(factory)
	}

	return factory
}

func (f *sharedInformerFactory) Start(stopCh <-chan struct{}) {
	f.StartWithContext(wait.ContextForChannel(stopCh))
}

func (f *sharedInformerFactory) StartWithContext(ctx context.Context) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.shuttingDown {
		return
	}

	for informerType, informer := range f.informers {
		if !f.startedInformers[informerType] {
			f.wg.Go(func() {
				informer.RunWithContext(ctx)
			})
			f.startedInformers[informerType] = true
		}
	}
}

func (f *sharedInformerFactory) Shutdown() {
	f.lock.Lock()
	f.shuttingDown = true
	f.lock.Unlock()

	f.wg.Wait()
	f.informerName.Release()
}

func (f *sharedInformerFactory) WaitForCacheSync(stopCh <-chan struct{}) map[reflect.Type]bool {
	result := f.WaitForCacheSyncWithContext(wait.ContextForChannel(stopCh))
	return result.Synced
}

func (f *sharedInformerFactory) WaitForCacheSyncWithContext(ctx context.Context) cache.SyncResult {
	informers := func() map[reflect.Type]cache.SharedIndexInformer {
		f.lock.Lock()
		defer f.lock.Unlock()

		informers := map[reflect.Type]cache.SharedIndexInformer{}
		for informerType, informer := range f.informers {
			if f.startedInformers[informerType] {
				informers[informerType] = informer
			}
		}
		return informers
	}()

	cacheSyncs := make([]cache.DoneChecker, 0, len(informers))
	for _, informer := range informers {
		cacheSyncs = append(cacheSyncs, informer.HasSyncedChecker())
	}
	cache.WaitFor(ctx, "" /* no logging */, cacheSyncs...)

	res := cache.SyncResult{
		Synced: make(map[reflect.Type]bool, len(informers)),
	}
	failed := false
	for informType, informer := range informers {
		hasSynced := informer.HasSynced()
		if !hasSynced {
			failed = true
		}
		res.Synced[informType] = hasSynced
	}
	if failed {
		res.Err = context.Cause(ctx)
	}

	return res
}
func (f *sharedInformerFactory) InformerFor(obj runtime.Object, newFunc internalinterfaces.NewInformerFunc) cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(obj)
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	resyncPeriod, exists := f.customResync[informerType]
	if !exists {
		resyncPeriod = f.defaultResync
	}

	informer = newFunc(f.client, resyncPeriod)
	if f.transform != nil {
		informer.SetTransform(f.transform)
	}
	f.informers[informerType] = informer

	return informer
}

type GenericInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() cache.GenericLister
}

func (f *sharedInformerFactory) ForResource(resource schema.GroupVersionResource) (GenericInformer, error) {
	return nil, fmt.Errorf("informers: ForResource is not available in this build")
}

type SharedInformerFactory interface {
	internalinterfaces.SharedInformerFactory

	Start(stopCh <-chan struct{})
	StartWithContext(ctx context.Context)
	Shutdown()
	WaitForCacheSync(stopCh <-chan struct{}) map[reflect.Type]bool
	WaitForCacheSyncWithContext(ctx context.Context) cache.SyncResult
	ForResource(resource schema.GroupVersionResource) (GenericInformer, error)
	InformerFor(obj runtime.Object, newFunc internalinterfaces.NewInformerFunc) cache.SharedIndexInformer

	Apps() apps.Interface
	Autoscaling() autoscaling.Interface
	Batch() batch.Interface
	Admissionregistration() admissionregistration.Interface
	Coordination() coordination.Interface
	Discovery() discovery.Interface
	Core() core.Interface
	Policy() policy.Interface
	Resource() resource.Interface
	Scheduling() scheduling.Interface
	Storage() storage.Interface
}

func (f *sharedInformerFactory) Apps() apps.Interface {
	return apps.New(f, f.namespace, f.tweakListOptions)
}

func (f *sharedInformerFactory) Autoscaling() autoscaling.Interface {
	return autoscaling.New(f, f.namespace, f.tweakListOptions)
}

func (f *sharedInformerFactory) Batch() batch.Interface {
	return batch.New(f, f.namespace, f.tweakListOptions)
}

func (f *sharedInformerFactory) Admissionregistration() admissionregistration.Interface {
	return admissionregistration.New(f, f.namespace, f.tweakListOptions)
}

func (f *sharedInformerFactory) Coordination() coordination.Interface {
	return coordination.New(f, f.namespace, f.tweakListOptions)
}

func (f *sharedInformerFactory) Discovery() discovery.Interface {
	return discovery.New(f, f.namespace, f.tweakListOptions)
}

func (f *sharedInformerFactory) Core() core.Interface {
	return core.New(f, f.namespace, f.tweakListOptions)
}

func (f *sharedInformerFactory) Policy() policy.Interface {
	return policy.New(f, f.namespace, f.tweakListOptions)
}

func (f *sharedInformerFactory) Resource() resource.Interface {
	return resource.New(f, f.namespace, f.tweakListOptions)
}

func (f *sharedInformerFactory) Scheduling() scheduling.Interface {
	return scheduling.New(f, f.namespace, f.tweakListOptions)
}

func (f *sharedInformerFactory) Storage() storage.Interface {
	return storage.New(f, f.namespace, f.tweakListOptions)
}
