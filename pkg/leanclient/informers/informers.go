//go:build js && wasm

// Package informers is a narrow stand-in for k8s.io/client-go/informers'
// SharedInformerFactory: it lazily constructs and shares (never
// duplicates) exactly the fourteen SharedIndexInformers this repo's
// eleven enabled controllers (pkg/controller/{replicaset,deployment,
// daemon,statefulset,job,cronjob,endpoint,endpointslice,nodeipam,
// nodelifecycle,tainteviction}) and scheduler (pkg/scheduler) take as
// constructor parameters, by
// calling upstream's own per-resource NewFilteredXInformer constructor
// for each directly (e.g. k8s.io/client-go/informers/core/v1.
// NewFilteredPodInformer). Those constructors already build the actual
// ListWatch/Reflector/SharedIndexInformer/Indexer via
// k8s.io/client-go/tools/cache exactly like the real SharedInformerFactory
// does internally, and this repo's pkg/leanclient/clientset.Clientset
// satisfies the kubernetes.Interface parameter they require -- genuine
// reuse of upstream's watch machinery (CLAUDE.md rule 3), not a
// reimplementation. Listers are upstream's own generated
// k8s.io/client-go/listers/<group>/<version> packages, unmodified --
// those are plain cache.Indexer wrappers with no runtime.Scheme
// dependency, so unlike kubernetes/typed/* and applyconfigurations/*
// (see pkg/leanclient/leanclient.go's doc comment) they never needed
// pruning in the first place.
//
// Deliberately does not use the real, aggregate k8s.io/client-go/informers
// package's own Interface{} (Core().V1(), Apps().V1(), ...): unlike this
// package's one-leaf-constructor-per-resource approach, that indirection
// risks pulling in every resource's constructor across every group it
// supports, most of which this repo's ten controllers never touch (see
// third_party/clientgo-lean-overlays/README.md for why "referencing
// anything in a package can link unrelated package-mate code" is a real,
// measured risk here, not a theoretical one).
package informers

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1informers "k8s.io/client-go/informers/apps/v1"
	batchv1informers "k8s.io/client-go/informers/batch/v1"
	coordinationv1informers "k8s.io/client-go/informers/coordination/v1"
	corev1informers "k8s.io/client-go/informers/core/v1"
	discoveryv1informers "k8s.io/client-go/informers/discovery/v1"

	appsv1listers "k8s.io/client-go/listers/apps/v1"
	batchv1listers "k8s.io/client-go/listers/batch/v1"
	coordinationv1listers "k8s.io/client-go/listers/coordination/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	discoveryv1listers "k8s.io/client-go/listers/discovery/v1"

	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// namespaceIndexers is the same cache.Indexers every generated
// informers/<group>/<version>/*.go constructor in client-go passes by
// default -- including for cluster-scoped types like Node (verified by
// reading e.g. k8s.io/client-go/informers/core/v1/node.go's own
// defaultInformer, which uses this same indexer despite Nodes having no
// namespace) -- so this replicates that instead of inventing its own.
var namespaceIndexers = cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}

// Informers shares client's connections. resync should match
// pkg/controllers' minResyncPeriod (kept as a param here, not a
// hardcoded constant, so this package doesn't duplicate that policy
// decision).
type Informers struct {
	client kubernetes.Interface
	resync time.Duration

	pods                   cache.SharedIndexInformer
	nodes                  cache.SharedIndexInformer
	services               cache.SharedIndexInformer
	endpoints              cache.SharedIndexInformer
	replicaSets            cache.SharedIndexInformer
	deployments            cache.SharedIndexInformer
	daemonSets             cache.SharedIndexInformer
	statefulSets           cache.SharedIndexInformer
	controllerRevisions    cache.SharedIndexInformer
	jobs                   cache.SharedIndexInformer
	cronJobs               cache.SharedIndexInformer
	endpointSlices         cache.SharedIndexInformer
	leases                 cache.SharedIndexInformer
	persistentVolumeClaims cache.SharedIndexInformer
}

func New(client kubernetes.Interface, resync time.Duration) *Informers {
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

func (f *Informers) ReplicaSets() appsv1informers.ReplicaSetInformer {
	if f.replicaSets == nil {
		f.replicaSets = appsv1informers.NewFilteredReplicaSetInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &replicaSetInformer{f.replicaSets}
}

func (f *Informers) Deployments() appsv1informers.DeploymentInformer {
	if f.deployments == nil {
		f.deployments = appsv1informers.NewFilteredDeploymentInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &deploymentInformer{f.deployments}
}

func (f *Informers) DaemonSets() appsv1informers.DaemonSetInformer {
	if f.daemonSets == nil {
		f.daemonSets = appsv1informers.NewFilteredDaemonSetInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &daemonSetInformer{f.daemonSets}
}

func (f *Informers) StatefulSets() appsv1informers.StatefulSetInformer {
	if f.statefulSets == nil {
		f.statefulSets = appsv1informers.NewFilteredStatefulSetInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &statefulSetInformer{f.statefulSets}
}

func (f *Informers) PersistentVolumeClaims() corev1informers.PersistentVolumeClaimInformer {
	if f.persistentVolumeClaims == nil {
		f.persistentVolumeClaims = corev1informers.NewFilteredPersistentVolumeClaimInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &persistentVolumeClaimInformer{f.persistentVolumeClaims}
}

func (f *Informers) ControllerRevisions() appsv1informers.ControllerRevisionInformer {
	if f.controllerRevisions == nil {
		f.controllerRevisions = appsv1informers.NewFilteredControllerRevisionInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &controllerRevisionInformer{f.controllerRevisions}
}

func (f *Informers) Jobs() batchv1informers.JobInformer {
	if f.jobs == nil {
		f.jobs = batchv1informers.NewFilteredJobInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &jobInformer{f.jobs}
}

func (f *Informers) CronJobs() batchv1informers.CronJobInformer {
	if f.cronJobs == nil {
		f.cronJobs = batchv1informers.NewFilteredCronJobInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &cronJobInformer{f.cronJobs}
}

func (f *Informers) EndpointSlices() discoveryv1informers.EndpointSliceInformer {
	if f.endpointSlices == nil {
		f.endpointSlices = discoveryv1informers.NewFilteredEndpointSliceInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &endpointSliceInformer{f.endpointSlices}
}

// Leases watches every namespace (metav1.NamespaceAll), matching this
// repo's pre-existing controllermanager.go behavior (a single shared
// factory with no namespace restriction) exactly, rather than "fixing"
// it to scope to kube-node-lease the way real kube-controller-manager's
// own dedicated lease informer factory does -- out of scope here
// (CLAUDE.md's "surgical changes" guidance: this package swaps the
// client layer only).
func (f *Informers) Leases() coordinationv1informers.LeaseInformer {
	if f.leases == nil {
		f.leases = coordinationv1informers.NewFilteredLeaseInformer(f.client, metav1.NamespaceAll, f.resync, namespaceIndexers, nil)
	}
	return &leaseInformer{f.leases}
}

// Start runs every informer actually constructed by the accessors above
// (mirrors SharedInformerFactory.Start's "only start what's been
// requested" behavior). Call after every controller constructor that
// needs one of the twelve resources above has already run once (so all
// twelve accessors that will ever be called, have been).
func (f *Informers) Start(stopCh <-chan struct{}) {
	for _, informer := range []cache.SharedIndexInformer{
		f.pods, f.nodes, f.services, f.endpoints,
		f.replicaSets, f.deployments, f.daemonSets, f.statefulSets, f.controllerRevisions,
		f.jobs, f.cronJobs, f.endpointSlices, f.leases, f.persistentVolumeClaims,
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

type replicaSetInformer struct{ informer cache.SharedIndexInformer }

func (i *replicaSetInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *replicaSetInformer) Lister() appsv1listers.ReplicaSetLister {
	return appsv1listers.NewReplicaSetLister(i.informer.GetIndexer())
}

type deploymentInformer struct{ informer cache.SharedIndexInformer }

func (i *deploymentInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *deploymentInformer) Lister() appsv1listers.DeploymentLister {
	return appsv1listers.NewDeploymentLister(i.informer.GetIndexer())
}

type daemonSetInformer struct{ informer cache.SharedIndexInformer }

func (i *daemonSetInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *daemonSetInformer) Lister() appsv1listers.DaemonSetLister {
	return appsv1listers.NewDaemonSetLister(i.informer.GetIndexer())
}

type statefulSetInformer struct{ informer cache.SharedIndexInformer }

func (i *statefulSetInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *statefulSetInformer) Lister() appsv1listers.StatefulSetLister {
	return appsv1listers.NewStatefulSetLister(i.informer.GetIndexer())
}

type controllerRevisionInformer struct{ informer cache.SharedIndexInformer }

func (i *controllerRevisionInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *controllerRevisionInformer) Lister() appsv1listers.ControllerRevisionLister {
	return appsv1listers.NewControllerRevisionLister(i.informer.GetIndexer())
}

type jobInformer struct{ informer cache.SharedIndexInformer }

func (i *jobInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *jobInformer) Lister() batchv1listers.JobLister {
	return batchv1listers.NewJobLister(i.informer.GetIndexer())
}

type cronJobInformer struct{ informer cache.SharedIndexInformer }

func (i *cronJobInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *cronJobInformer) Lister() batchv1listers.CronJobLister {
	return batchv1listers.NewCronJobLister(i.informer.GetIndexer())
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

type persistentVolumeClaimInformer struct{ informer cache.SharedIndexInformer }

func (i *persistentVolumeClaimInformer) Informer() cache.SharedIndexInformer { return i.informer }
func (i *persistentVolumeClaimInformer) Lister() corev1listers.PersistentVolumeClaimLister {
	return corev1listers.NewPersistentVolumeClaimLister(i.informer.GetIndexer())
}
