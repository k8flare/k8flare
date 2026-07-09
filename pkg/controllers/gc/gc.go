//go:build js && wasm

// Package gc hosts the real, unmodified upstream garbagecollector
// controller (ownerReferences cascade delete) for
// pkg/controllers/cmd/gc-wasm. Its own package, not pkg/controllers
// itself, so kube-controller-manager's WASM binary (built from
// pkg/controllers) never links this controller's reachable code in --
// see pkg/controllers/restconfig's doc comment for why that separation
// is load-bearing, not just tidiness (measured, not assumed: this file
// briefly lived in pkg/controllers directly and added ~4MB to the KCM
// binary despite RunControllerManager never calling RunGarbageCollector).
package gc

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	apimachinerywatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/metadata"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/controller-manager/pkg/informerfactory"
	"k8s.io/kubernetes/pkg/controller/garbagecollector"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
	"github.com/k8flare/k8flare/pkg/leanclient"
	leanclientset "github.com/k8flare/k8flare/pkg/leanclient/clientset"
)

// resyncPeriod matches ResourceResyncTime in the real
// k8s.io/kubernetes/pkg/controller/garbagecollector package (0 -- the
// graph builder relies on watch events, not periodic relist, exactly
// like every other real controller this repo already embeds).
const resyncPeriod = 0

// workers and syncPeriod match kube-controller-manager v1.36.2's own
// --concurrent-gc-syncs default (20,
// pkg/controller/garbagecollector/config/v1alpha1/defaults.go) and the
// app package's hardcoded 30s syncPeriod (both gc.Run's
// initialSyncTimeout and gc.Sync's period -- see
// cmd/kube-controller-manager/app/core.go's garbageCollectorController.Run),
// not invented -- same "hand-wire the real defaults, no CLI layer here"
// posture as pkg/controllers/controllermanager.go's own worker-count
// constants.
const (
	workers    = 20
	syncPeriod = 30 * time.Second
)

// informerFactory implements k8s.io/controller-manager/pkg/informerfactory.
// InformerFactory (the interface the real garbagecollector controller
// needs) directly against a metadata.Interface, always as
// PartialObjectMetadata-only informers -- unlike the real app package's
// production wiring (which prefers a typed informer per resource and
// only falls back to metadata-only for the rest), the garbage collector
// only ever reads ObjectMeta/OwnerReferences off anything it watches, so
// there is no reason to ever build the typed half here. Built from real
// upstream primitives (cache.NewSharedIndexInformer, cache.ListWatch,
// cache.NewGenericLister) fed by pkg/leanclient.MetadataClient --
// exactly the same non-Scheme DoRaw+json approach every other client in
// this repo's WASM binaries uses (see pkg/leanclient's doc comment).
//
// The real garbagecollector controller discovers which GVRs to monitor
// lazily, via ForResource calls made from inside its own Sync/
// resyncMonitors goroutine -- which can happen either before or after
// this factory's own Start() is called (Start's caller has no way to
// force an ordering, since Sync's first pass is itself asynchronous).
// So Start() just records stopCh and replays it forward: any informer
// that already existed gets started immediately, and ForResource starts
// any NEW informer immediately too if Start has already run. This
// mirrors client-go's own real SharedInformerFactory, which has the
// exact same "already started" bookkeeping for the same reason.
type informerFactory struct {
	metadataClient metadata.Interface

	mu        sync.Mutex
	informers map[schema.GroupVersionResource]informers.GenericInformer
	stopCh    <-chan struct{}
	// started is a separate flag from stopCh itself: this factory's own
	// ctx is context.Background() (pkg/cfruntime.ResidentService's run()
	// argument), whose Done() is documented to return a literal nil
	// channel ("Done may return nil if this context can never be
	// canceled") -- checking stopCh != nil as the "has Start been
	// called" signal is wrong precisely when stopCh's real, intended
	// value legitimately IS nil, which silently skipped starting every
	// informer here (found live 2026-07-09: ListWatch was constructed
	// but its Run loop never started, so cache sync never had a chance
	// to begin at all -- not a scale/isolate-memory issue as first
	// suspected).
	started bool
}

var _ informerfactory.InformerFactory = (*informerFactory)(nil)

func newInformerFactory(metadataClient metadata.Interface) *informerFactory {
	return &informerFactory{
		metadataClient: metadataClient,
		informers:      make(map[schema.GroupVersionResource]informers.GenericInformer),
	}
}

func (f *informerFactory) ForResource(gvr schema.GroupVersionResource) (informers.GenericInformer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if inf, ok := f.informers[gvr]; ok {
		return inf, nil
	}

	res := f.metadataClient.Resource(gvr)
	lw := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			return res.Namespace(metav1.NamespaceAll).List(context.Background(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (apimachinerywatch.Interface, error) {
			return res.Namespace(metav1.NamespaceAll).Watch(context.Background(), opts)
		},
	}
	sharedIndexInformer := cache.NewSharedIndexInformer(
		lw,
		&metav1.PartialObjectMetadata{},
		resyncPeriod,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	inf := &genericInformer{
		informer: sharedIndexInformer,
		lister:   cache.NewGenericLister(sharedIndexInformer.GetIndexer(), gvr.GroupResource()),
	}
	f.informers[gvr] = inf
	if f.started {
		go inf.Informer().Run(f.stopCh)
	}
	return inf, nil
}

func (f *informerFactory) Start(stopCh <-chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCh = stopCh
	f.started = true
	for _, inf := range f.informers {
		go inf.Informer().Run(stopCh)
	}
}

type genericInformer struct {
	informer cache.SharedIndexInformer
	lister   cache.GenericLister
}

func (i *genericInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *genericInformer) Lister() cache.GenericLister         { return i.lister }

// staticDiscovery implements discovery.ServerResourcesInterface (the
// interface gc.Sync needs to notice newly servable resources) from
// apidef.Table instead of a real HTTP round trip to /apis: this
// project's resource set is fixed at build time, so there is nothing a
// real discovery client would ever tell gc.Sync that this static answer
// doesn't already know -- same reasoning as apidef.NewRESTMapper.
type staticDiscovery struct {
	lists []*metav1.APIResourceList
}

var _ discovery.ServerResourcesInterface = (*staticDiscovery)(nil)

func newStaticDiscovery() *staticDiscovery {
	byGV := make(map[schema.GroupVersion]*metav1.APIResourceList)
	for _, r := range apidef.Table {
		rl, ok := byGV[r.GroupVersion]
		if !ok {
			rl = &metav1.APIResourceList{GroupVersion: r.GroupVersion.String()}
			byGV[r.GroupVersion] = rl
		}
		verbs := r.Verbs
		if verbs == nil {
			verbs = apidef.StandardVerbs
		}
		rl.APIResources = append(rl.APIResources, metav1.APIResource{
			Name:       r.Resource,
			Kind:       r.Kind,
			Namespaced: r.Namespaced,
			Verbs:      verbs,
		})
	}
	lists := make([]*metav1.APIResourceList, 0, len(byGV))
	for _, rl := range byGV {
		lists = append(lists, rl)
	}
	return &staticDiscovery{lists: lists}
}

func (d *staticDiscovery) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	for _, rl := range d.lists {
		if rl.GroupVersion == groupVersion {
			return rl, nil
		}
	}
	return nil, fmt.Errorf("garbage-collector: no such group-version %q", groupVersion)
}

func (d *staticDiscovery) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	return nil, d.lists, nil
}

func (d *staticDiscovery) ServerPreferredResources() ([]*metav1.APIResourceList, error) {
	return d.lists, nil
}

func (d *staticDiscovery) ServerPreferredNamespacedResources() ([]*metav1.APIResourceList, error) {
	namespaced := make([]*metav1.APIResourceList, 0, len(d.lists))
	for _, rl := range d.lists {
		nsResources := make([]metav1.APIResource, 0, len(rl.APIResources))
		for _, r := range rl.APIResources {
			if r.Namespaced {
				nsResources = append(nsResources, r)
			}
		}
		if len(nsResources) > 0 {
			namespaced = append(namespaced, &metav1.APIResourceList{GroupVersion: rl.GroupVersion, APIResources: nsResources})
		}
	}
	return namespaced, nil
}

// RunGarbageCollector starts the real, unmodified upstream
// garbagecollector controller (ownerReferences cascade delete) against
// restCfg, and blocks until ctx is canceled. Runs as its own dynamic
// worker (pkg/controllers/cmd/gc-wasm), a separate Loader isolate from
// the six workload controllers pkg/controllers.RunControllerManager
// runs -- unlike those, the garbage collector's graph builder needs an
// informer/watch per resource type across this apiserver's entire
// apidef.Table, which would compete for the same 128MiB isolate memory
// budget that already forced endpoint/endpointslice/nodeipam/
// nodelifecycle/tainteviction out of RunControllerManager once (see that
// function's doc comment, 2026-07-06 finding).
//
// Replaces pkg/apiserver/gc.go's CascadeDeleteDependents, this
// project's own narrower, synchronous-in-request substitute -- see git
// history and docs/general-purpose-k8s-plan.md's "ownerReferences GC"
// entry for that mechanism and why it existed. This is a deliberate,
// user-approved behavior change: `kubectl delete deployment` now returns
// as soon as the Deployment itself is gone, with its ReplicaSets/Pods
// cascading asynchronously afterward (this poked, event-armed dynamic
// worker's own delay), rather than blocking until the whole cascade
// completes in the same request -- eventually consistent, matching real
// upstream Kubernetes, not the synchronous guarantee the old mechanism
// gave for free by construction.
func RunGarbageCollector(ctx context.Context, restCfg *restclient.Config) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("garbage-collector: panic: %v", r)
		}
	}()

	kubeClient, err := leanclientset.NewForConfig(restclient.AddUserAgent(restCfg, "garbage-collector"))
	if err != nil {
		return fmt.Errorf("garbage-collector: build client: %w", err)
	}
	metadataClient := leanclient.NewMetadataClient(restclient.AddUserAgent(restCfg, "garbage-collector"))
	mapper := apidef.NewRESTMapper()
	factory := newInformerFactory(metadataClient)

	alwaysStarted := make(chan struct{})
	close(alwaysStarted)

	garbageCollector, err := garbagecollector.NewGarbageCollector(
		ctx,
		kubeClient,
		metadataClient,
		mapper,
		garbagecollector.DefaultIgnoredResources(),
		factory,
		alwaysStarted,
	)
	if err != nil {
		return fmt.Errorf("garbage-collector: new garbage collector: %w", err)
	}

	// Surface panics from the informer/reflector goroutine tree instead
	// of letting HandleCrash re-panic and kill the whole WASM instance:
	// a resident dynamic worker that dies here just reload-loops with no
	// visible cause (observed live 2026-07-09 as "gc dynamic worker up"
	// repeating), because the Go runtime's own crash output does not
	// reliably surface through the Loader bootstrap's console. Keeping
	// ReallyCrash=false ALSO matches this binary's survival posture:
	// pkg/controllers.runRecovered already established that one
	// component's panic must not kill the shared instance.
	utilruntime.ReallyCrash = false
	utilruntime.PanicHandlers = append(utilruntime.PanicHandlers, func(_ context.Context, r interface{}) {
		println("garbage-collector: captured panic:", fmt.Sprint(r))
		println(string(debug.Stack()))
	})

	// factory.Start before Sync's first resyncMonitors pass so every
	// informer Sync lazily creates via ForResource self-starts
	// immediately (see informerFactory's doc comment) -- there is no
	// fixed set of informers to start up front the way
	// RunControllerManager's factory.Start(ctx.Done()) has, since which
	// GVRs matter is exactly what Sync is about to discover.
	factory.Start(ctx.Done())

	// Run and Sync concurrently, matching the real app package's own
	// garbageCollectorController.Run (concurrentRun of the same two
	// calls) -- Sync's first pass populates the dependency graph
	// builder's monitors (which is what actually creates the
	// per-resource informers above), Run processes the resulting
	// attemptToDelete/attemptToOrphan queues.
	go garbageCollector.Sync(ctx, newStaticDiscovery(), syncPeriod)
	garbageCollector.Run(ctx, workers, syncPeriod)
	return ctx.Err()
}
