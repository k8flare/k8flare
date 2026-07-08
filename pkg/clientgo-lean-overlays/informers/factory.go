// Hand-curated: narrows k8s.io/client-go/informers' SharedInformerFactory
// from 19 top-level API group accessors to the 5
// (Core/Apps/Storage/Resource/Scheduling) this repo's real, unmodified
// upstream kube-scheduler (pkg/scheduler.New's informerFactory parameter,
// see docs/platform-verification.md's S8 kube-scheduler-wasm-fork entry)
// actually calls. See pkg/clientgo-lean-overlays/README.md for why
// this whole mirror exists; this file extends that same "prune the
// aggregate, not the leaf packages" technique one level up, from
// kubernetes/clientset.go (kubernetes.Interface, ~54 typed-client
// accessors -- reverted to full width, see that file's own doc comment)
// to informers/factory.go (informers.SharedInformerFactory, 19
// accessors). Unlike kubernetes.Interface, narrowing this one IS safe:
// nothing outside pkg/scheduler ever needs informers.SharedInformerFactory
// to satisfy an *external* fixed-width contract the way
// client-go/tools/leaderelection/resourcelock needs the full
// kubernetes.Interface -- the only consumer is pkg/scheduler.New itself,
// and this repo's fork of it (this same overlay tree) is the one place
// that constructs and calls the factory.
//
// What's kept real and unmodified below this level (deliberately NOT
// pruned further, matching gen-clientgo-lean-mirror.sh's existing
// philosophy for informers/<group>/<version> and listers/<group>/<version>
// -- those are "plain cache.Indexer wrappers", cheap regardless of
// version count): Core() (only ever had V1 upstream), Apps() (V1/
// V1beta1/V1beta2 -- all three kept, unpruned), Storage() (V1/V1alpha1/
// V1beta1), Resource() (V1/V1alpha3/V1beta1/V1beta2 -- scheduler.go's DRA
// block reaches V1beta2().DeviceTaintRules() when
// features.DRADeviceTaintRules is on; that gate is Beta/Default:false and
// NOT locked in v1.36.2-k3s1, unlike DynamicResourceAllocation itself
// which IS GA/LockToDefault:true -- see docs/platform-verification.md's
// S8 entry for the verification), Scheduling() (V1/V1alpha2/V1beta1),
// Policy() (V1/V1beta1 -- framework/preemption's PodDisruptionBudget
// lister, unconditional: DefaultPreemption is not in
// pkg/controllers/sched.RunScheduler's disabled-plugins list).
//
// Dropped entirely (each costs its own informers/<group>[/<version>]
// package plus, for groups this repo's Clientset only panic-stubs at the
// kubernetes.Interface level anyway, no offsetting benefit):
// Admissionregistration, Internal (apiserverinternal), Autoscaling,
// Batch, Certificates, Coordination, Discovery, Events, Extensions,
// Flowcontrol, Networking, Node, Rbac, Storagemigration.
//
// ForResource: no call site in pkg/scheduler's own tree reaches it
// (confirmed by grep across scheduler.go, eventhandlers.go,
// framework/runtime/framework.go, backend/queue/scheduling_queue.go,
// framework/plugins/podtopologyspread/plugin.go,
// framework/plugins/dynamicresources/dra_manager.go) -- generic.go (the
// upstream file that implements it, via a GroupVersionResource switch
// spanning all ~54 API types) is deleted from the mirror by
// gen-clientgo-lean-mirror.sh, and GenericInformer/ForResource are
// redeclared here as a permanent panic stub, same pattern as this
// repo's other confirmed-unused-methods (pkg/leanclient/clientset/
// stubs.go).
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
	core "k8s.io/client-go/informers/core"
	internalinterfaces "k8s.io/client-go/informers/internalinterfaces"
	policy "k8s.io/client-go/informers/policy"
	resource "k8s.io/client-go/informers/resource"
	scheduling "k8s.io/client-go/informers/scheduling"
	storage "k8s.io/client-go/informers/storage"
	kubernetes "k8s.io/client-go/kubernetes"
	cache "k8s.io/client-go/tools/cache"
)

// SharedInformerOption defines the functional option type for SharedInformerFactory.
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

	informers map[reflect.Type]cache.SharedIndexInformer
	// startedInformers is used for tracking which informers have been started.
	// This allows Start() to be called multiple times safely.
	startedInformers map[reflect.Type]bool
	// wg tracks how many goroutines were started.
	wg sync.WaitGroup
	// shuttingDown is true when Shutdown has been called. It may still be running
	// because it needs to wait for goroutines.
	shuttingDown bool
}

// WithCustomResyncConfig sets a custom resync period for the specified informer types.
func WithCustomResyncConfig(resyncConfig map[v1.Object]time.Duration) SharedInformerOption {
	return func(factory *sharedInformerFactory) *sharedInformerFactory {
		for k, v := range resyncConfig {
			factory.customResync[reflect.TypeOf(k)] = v
		}
		return factory
	}
}

// WithTweakListOptions sets a custom filter on all listers of the configured SharedInformerFactory.
func WithTweakListOptions(tweakListOptions internalinterfaces.TweakListOptionsFunc) SharedInformerOption {
	return func(factory *sharedInformerFactory) *sharedInformerFactory {
		factory.tweakListOptions = tweakListOptions
		return factory
	}
}

// WithNamespace limits the SharedInformerFactory to the specified namespace.
func WithNamespace(namespace string) SharedInformerOption {
	return func(factory *sharedInformerFactory) *sharedInformerFactory {
		factory.namespace = namespace
		return factory
	}
}

// WithTransform sets a transform on all informers.
func WithTransform(transform cache.TransformFunc) SharedInformerOption {
	return func(factory *sharedInformerFactory) *sharedInformerFactory {
		factory.transform = transform
		return factory
	}
}

// WithInformerName sets the InformerName for informer identity used in metrics.
func WithInformerName(informerName *cache.InformerName) SharedInformerOption {
	return func(factory *sharedInformerFactory) *sharedInformerFactory {
		factory.informerName = informerName
		return factory
	}
}

func (f *sharedInformerFactory) InformerName() *cache.InformerName {
	return f.informerName
}

// NewSharedInformerFactory constructs a new instance of sharedInformerFactory for all namespaces.
func NewSharedInformerFactory(client kubernetes.Interface, defaultResync time.Duration) SharedInformerFactory {
	return NewSharedInformerFactoryWithOptions(client, defaultResync)
}

// NewFilteredSharedInformerFactory constructs a new instance of sharedInformerFactory.
//
// Deprecated: Please use NewSharedInformerFactoryWithOptions instead
func NewFilteredSharedInformerFactory(client kubernetes.Interface, defaultResync time.Duration, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) SharedInformerFactory {
	return NewSharedInformerFactoryWithOptions(client, defaultResync, WithNamespace(namespace), WithTweakListOptions(tweakListOptions))
}

// NewSharedInformerFactoryWithOptions constructs a new instance of a SharedInformerFactory with additional options.
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

// InformerFor returns the SharedIndexInformer for obj using an internal client.
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

// GenericInformer is type of SharedIndexInformer which will locate and
// delegate to other sharedInformers based on type -- upstream's own
// generic.go declares this identically; redeclared here because
// generic.go itself (the ~54-type GroupVersionResource switch that
// implements ForResource) is deleted from this pruned mirror, see this
// file's doc comment.
type GenericInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() cache.GenericLister
}

// ForResource: confirmed unused by every call site in pkg/scheduler's own
// tree (see this file's doc comment) -- permanent panic stub, not a
// narrowed real implementation.
func (f *sharedInformerFactory) ForResource(resource schema.GroupVersionResource) (GenericInformer, error) {
	return nil, fmt.Errorf("k8flare: informers.SharedInformerFactory.ForResource pruned -- unused by pkg/scheduler (see pkg/clientgo-lean-overlays/informers/factory.go)")
}

// SharedInformerFactory provides shared informers for the 5 API groups
// (Core/Apps/Storage/Resource/Scheduling) this repo's real, unmodified
// upstream kube-scheduler actually uses -- see this file's doc comment
// for the full accounting of what's dropped and why.
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
	Core() core.Interface
	Policy() policy.Interface
	Resource() resource.Interface
	Scheduling() scheduling.Interface
	Storage() storage.Interface
}

func (f *sharedInformerFactory) Apps() apps.Interface {
	return apps.New(f, f.namespace, f.tweakListOptions)
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
