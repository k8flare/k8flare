//go:build js && wasm

package leanclient

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1informers "k8s.io/client-go/informers/apps/v1"
	coordinationv1informers "k8s.io/client-go/informers/coordination/v1"
	corev1informers "k8s.io/client-go/informers/core/v1"
	discoveryv1informers "k8s.io/client-go/informers/discovery/v1"

	appsv1listers "k8s.io/client-go/listers/apps/v1"
	coordinationv1listers "k8s.io/client-go/listers/coordination/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	discoveryv1listers "k8s.io/client-go/listers/discovery/v1"

	"k8s.io/client-go/tools/cache"
)

// namespaceIndexers is the same cache.Indexers every generated
// informers/<group>/<version>/*.go constructor in client-go passes by
// default -- including for cluster-scoped types like Node (verified by
// reading e.g. k8s.io/client-go/informers/core/v1/node.go's own
// defaultInformer, which uses this same indexer despite Nodes having no
// namespace) -- so this replicates that instead of inventing its own.
var namespaceIndexers = cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}

// Informers is a narrow stand-in for k8s.io/client-go/informers'
// SharedInformerFactory: it lazily constructs and shares (never
// duplicates) exactly the seven SharedIndexInformers workers/controllers'
// five enabled controllers take as constructor parameters (Pods, Nodes,
// Services, Endpoints, EndpointSlices, Leases, DaemonSets -- see
// pkg/controllers/controllermanager.go), by calling upstream's own
// per-resource NewFilteredXInformer constructor for each directly (e.g.
// k8s.io/client-go/informers/core/v1.NewFilteredPodInformer). Those
// constructors already build the actual ListWatch/Reflector/
// SharedIndexInformer/Indexer via k8s.io/client-go/tools/cache exactly
// like the real SharedInformerFactory does internally -- genuine reuse of
// upstream's watch machinery (CLAUDE.md rule 3), not a reimplementation.
//
// The only reason not to just use the real, aggregate
// k8s.io/client-go/informers package directly: it defines Interface{}
// accessors (Core().V1(), Apps().V1(), Batch().V1(), Discovery().V1(),
// ...) for every group it supports, and a generic.go ForResource switch
// covering every GroupVersionResource across all of them -- unlike this
// package's approach of calling one leaf NewFilteredXInformer function
// per resource directly, that indirection risks pulling in constructor
// code for many resources this repo's five enabled controllers never
// touch. This wasn't required to hit this package's size target (see
// clientset.go's doc comment: the dominant cost turned out to be CoreV1
// itself, not how many groups/resources are registered elsewhere), but
// costs nothing extra either, so there was no reason to prefer the
// larger-surface-area option.
type Informers struct {
	client *Clientset
	resync time.Duration

	pods           cache.SharedIndexInformer
	nodes          cache.SharedIndexInformer
	services       cache.SharedIndexInformer
	endpoints      cache.SharedIndexInformer
	endpointSlices cache.SharedIndexInformer
	leases         cache.SharedIndexInformer
	daemonSets     cache.SharedIndexInformer
}

// NewInformers returns an Informers sharing client's connections. resync
// should match controllermanager.go's minResyncPeriod (kept as a param
// here, not a hardcoded constant, so this package doesn't duplicate that
// policy decision).
func NewInformers(client *Clientset, resync time.Duration) *Informers {
	return &Informers{client: client, resync: resync}
}

func (f *Informers) Pods() corev1informers.PodInformer {
	if f.pods == nil {
		f.pods = corev1informers.NewFilteredPodInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &podInformer{f.pods}
}

func (f *Informers) Nodes() corev1informers.NodeInformer {
	if f.nodes == nil {
		f.nodes = corev1informers.NewFilteredNodeInformer(f.client, f.resync, namespaceIndexers, nil)
	}
	return &nodeInformer{f.nodes}
}

func (f *Informers) Services() corev1informers.ServiceInformer {
	if f.services == nil {
		f.services = corev1informers.NewFilteredServiceInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &serviceInformer{f.services}
}

func (f *Informers) Endpoints() corev1informers.EndpointsInformer {
	if f.endpoints == nil {
		f.endpoints = corev1informers.NewFilteredEndpointsInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &endpointsInformer{f.endpoints}
}

func (f *Informers) EndpointSlices() discoveryv1informers.EndpointSliceInformer {
	if f.endpointSlices == nil {
		f.endpointSlices = discoveryv1informers.NewFilteredEndpointSliceInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &endpointSliceInformer{f.endpointSlices}
}

// Leases watches every namespace (metav1.NamespaceAll), matching this
// repo's existing controllermanager.go behavior (a single shared factory
// with no namespace restriction) exactly, rather than "fixing" it to
// scope to kube-node-lease the way real kube-controller-manager's own
// dedicated lease informer factory does -- out of scope here (CLAUDE.md's
// "surgical changes" guidance: this package swaps the client layer only).
func (f *Informers) Leases() coordinationv1informers.LeaseInformer {
	if f.leases == nil {
		f.leases = coordinationv1informers.NewFilteredLeaseInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &leaseInformer{f.leases}
}

func (f *Informers) DaemonSets() appsv1informers.DaemonSetInformer {
	if f.daemonSets == nil {
		f.daemonSets = appsv1informers.NewFilteredDaemonSetInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &daemonSetInformer{f.daemonSets}
}

// Start runs every informer actually constructed by the accessors above
// (mirrors SharedInformerFactory.Start's "only start what's been
// requested" behavior). Call after every controller constructor that
// needs one of the seven resources above has already run once (so all
// seven accessors that will ever be called, have been).
func (f *Informers) Start(stopCh <-chan struct{}) {
	for _, informer := range []cache.SharedIndexInformer{
		f.pods, f.nodes, f.services, f.endpoints, f.endpointSlices, f.leases, f.daemonSets,
	} {
		if informer != nil {
			go informer.Run(stopCh)
		}
	}
}

type podInformer struct{ informer cache.SharedIndexInformer }

func (i *podInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *podInformer) Lister() corev1listers.PodLister {
	return corev1listers.NewPodLister(i.informer.GetIndexer())
}

type nodeInformer struct{ informer cache.SharedIndexInformer }

func (i *nodeInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *nodeInformer) Lister() corev1listers.NodeLister {
	return corev1listers.NewNodeLister(i.informer.GetIndexer())
}

type serviceInformer struct{ informer cache.SharedIndexInformer }

func (i *serviceInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *serviceInformer) Lister() corev1listers.ServiceLister {
	return corev1listers.NewServiceLister(i.informer.GetIndexer())
}

type endpointsInformer struct{ informer cache.SharedIndexInformer }

func (i *endpointsInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *endpointsInformer) Lister() corev1listers.EndpointsLister {
	return corev1listers.NewEndpointsLister(i.informer.GetIndexer())
}

type endpointSliceInformer struct{ informer cache.SharedIndexInformer }

func (i *endpointSliceInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *endpointSliceInformer) Lister() discoveryv1listers.EndpointSliceLister {
	return discoveryv1listers.NewEndpointSliceLister(i.informer.GetIndexer())
}

type leaseInformer struct{ informer cache.SharedIndexInformer }

func (i *leaseInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *leaseInformer) Lister() coordinationv1listers.LeaseLister {
	return coordinationv1listers.NewLeaseLister(i.informer.GetIndexer())
}

type daemonSetInformer struct{ informer cache.SharedIndexInformer }

func (i *daemonSetInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *daemonSetInformer) Lister() appsv1listers.DaemonSetLister {
	return appsv1listers.NewDaemonSetLister(i.informer.GetIndexer())
}
