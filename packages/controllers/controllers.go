package controllers

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/metadata/metadatainformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/controller-manager/pkg/informerfactory"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/controller/certificates/rootcacertpublisher"
	"k8s.io/kubernetes/pkg/controller/cronjob"
	"k8s.io/kubernetes/pkg/controller/daemon"
	"k8s.io/kubernetes/pkg/controller/deployment"
	"k8s.io/kubernetes/pkg/controller/endpoint"
	"k8s.io/kubernetes/pkg/controller/endpointslice"
	"k8s.io/kubernetes/pkg/controller/garbagecollector"
	"k8s.io/kubernetes/pkg/controller/job"
	"k8s.io/kubernetes/pkg/controller/namespace"
	"k8s.io/kubernetes/pkg/controller/nodeipam"
	"k8s.io/kubernetes/pkg/controller/nodeipam/ipam"
	"k8s.io/kubernetes/pkg/controller/nodelifecycle"
	"k8s.io/kubernetes/pkg/controller/replicaset"
	"k8s.io/kubernetes/pkg/controller/replication"
	"k8s.io/kubernetes/pkg/controller/serviceaccount"
	"k8s.io/kubernetes/pkg/controller/statefulset"
	"k8s.io/kubernetes/pkg/controller/tainteviction"
)

const (
	workers                     = 5
	daemonSetWorkers            = 2
	maxEndpointsPerSlice        = 100
	nodeCIDRMaskSize            = 24
	nodeMonitorPeriod           = 5 * time.Second
	nodeStartupGracePeriod      = 60 * time.Second
	nodeMonitorGracePeriod      = 50 * time.Second
	evictionLimiterQPS          = 0.1
	secondaryEvictionLimiterQPS = 0.01
	largeClusterThreshold       = 50
	unhealthyZoneThreshold      = 0.55
	minResyncPeriod             = 12 * time.Hour
	namespaceSyncPeriod         = 5 * time.Minute
	namespaceWorkers            = 10
	garbageCollectorWorkers     = 20
	garbageCollectorSyncPeriod  = 30 * time.Second
	retryQuiet                  = 20 * time.Second
	retryPendingFor             = 5 * time.Minute
	addQuiet                    = 10 * time.Second
)

type Controllers struct {
	factory          informers.SharedInformerFactory
	metadataFactory  metadatainformer.SharedInformerFactory
	informersStarted chan struct{}
	runs             []func(context.Context)
	started          atomic.Bool
}

var queues = &queueDepths{}

func init() {
	workqueue.SetProvider(queues)
}

func New(ctx context.Context, cfg *rest.Config) (*Controllers, error) {
	client, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "kube-controller-manager"))
	if err != nil {
		return nil, err
	}
	factory := informers.NewSharedInformerFactory(client, minResyncPeriod)
	core, apps, batch := factory.Core().V1(), factory.Apps().V1(), factory.Batch().V1()
	c := &Controllers{factory: factory, informersStarted: make(chan struct{})}
	rc := replication.NewReplicationManager(ctx, core.Pods(), core.ReplicationControllers(), client, replication.BurstReplicas)
	c.add(func(ctx context.Context) { rc.Run(ctx, workers) })
	rs := replicaset.NewReplicaSetController(ctx, apps.ReplicaSets(), core.Pods(), client, replicaset.BurstReplicas)
	c.add(func(ctx context.Context) { rs.Run(ctx, workers) })
	dc, err := deployment.NewDeploymentController(ctx, apps.Deployments(), apps.ReplicaSets(), core.Pods(), client)
	if err != nil {
		return nil, err
	}
	c.add(func(ctx context.Context) { dc.Run(ctx, workers) })
	ds, err := daemon.NewDaemonSetsController(ctx, apps.DaemonSets(), apps.ControllerRevisions(), core.Pods(), core.Nodes(), client, flowcontrol.NewBackOff(time.Second, 15*time.Minute))
	if err != nil {
		return nil, err
	}
	c.add(func(ctx context.Context) { ds.Run(ctx, daemonSetWorkers) })
	ss := statefulset.NewStatefulSetController(ctx, core.Pods(), apps.StatefulSets(), core.PersistentVolumeClaims(), apps.ControllerRevisions(), client)
	c.add(func(ctx context.Context) { ss.Run(ctx, workers) })
	jobs, err := job.NewController(ctx, client, core.Pods(), batch.Jobs(), nil, nil)
	if err != nil {
		return nil, err
	}
	c.add(func(ctx context.Context) { jobs.Run(ctx, workers) })
	cron, err := cronjob.NewControllerV2(ctx, batch.Jobs(), batch.CronJobs(), client)
	if err != nil {
		return nil, err
	}
	c.add(func(ctx context.Context) { cron.Run(ctx, workers) })
	ep := endpoint.NewEndpointController(ctx, core.Pods(), core.Services(), core.Endpoints(), client, 0)
	c.add(func(ctx context.Context) { ep.Run(ctx, workers) })
	eps := endpointslice.NewController(ctx, core.Pods(), core.Services(), core.Nodes(), factory.Discovery().V1().EndpointSlices(), maxEndpointsPerSlice, client, 0)
	c.add(func(ctx context.Context) { eps.Run(ctx, workers) })
	ipam, err := nodeipam.NewNodeIpamController(ctx, core.Nodes(), nil, client, []*net.IPNet{supervisor.ClusterCIDR}, supervisor.ServiceCIDR, nil, []int{nodeCIDRMaskSize}, ipam.RangeAllocatorType)
	if err != nil {
		return nil, err
	}
	c.add(ipam.Run)
	lifecycle, err := nodelifecycle.NewNodeLifecycleController(ctx, factory.Coordination().V1().Leases(), core.Pods(), core.Nodes(), apps.DaemonSets(), client,
		nodeMonitorPeriod, nodeStartupGracePeriod, nodeMonitorGracePeriod, evictionLimiterQPS, secondaryEvictionLimiterQPS, largeClusterThreshold, unhealthyZoneThreshold)
	if err != nil {
		return nil, err
	}
	c.add(lifecycle.Run)
	taints, err := tainteviction.New(ctx, client, core.Pods(), core.Nodes(), "taint-eviction-controller")
	if err != nil {
		return nil, err
	}
	c.add(taints.Run)
	accounts, err := serviceaccount.NewServiceAccountsController(klog.FromContext(ctx), core.ServiceAccounts(), core.Namespaces(), client, serviceaccount.DefaultServiceAccountsControllerOptions())
	if err != nil {
		return nil, err
	}
	c.add(func(ctx context.Context) { accounts.Run(ctx, 1) })
	metadataClient, err := metadata.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	namespaces := namespace.NewNamespaceController(ctx, client, metadataClient, client.Discovery().ServerPreferredNamespacedResources, core.Namespaces(), namespaceSyncPeriod, corev1.FinalizerKubernetes)
	c.add(func(ctx context.Context) { namespaces.Run(ctx, namespaceWorkers) })
	c.metadataFactory = metadatainformer.NewSharedInformerFactory(metadataClient, minResyncPeriod)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(client.Discovery()))
	graph := garbagecollector.NewDependencyGraphBuilder(ctx, metadataClient, mapper, garbagecollector.DefaultIgnoredResources(), informerfactory.NewInformerFactory(factory, c.metadataFactory), c.informersStarted)
	collector, err := garbagecollector.NewComposedGarbageCollector(ctx, client, metadataClient, mapper, graph)
	if err != nil {
		return nil, err
	}
	c.add(func(ctx context.Context) { collector.Run(ctx, garbageCollectorWorkers, garbageCollectorSyncPeriod) })
	c.add(func(ctx context.Context) { collector.Sync(ctx, client.Discovery(), garbageCollectorSyncPeriod) })
	c.add(func(ctx context.Context) { wait.Until(mapper.Reset, garbageCollectorSyncPeriod, ctx.Done()) })
	rootCA, err := serverCA(ctx, cfg)
	if err != nil {
		return nil, err
	}
	publisher, err := rootcacertpublisher.NewPublisher(core.ConfigMaps(), core.Namespaces(), client, rootCA)
	if err != nil {
		return nil, err
	}
	c.add(func(ctx context.Context) { publisher.Run(ctx, 1) })
	return c, nil
}

func serverCA(ctx context.Context, cfg *rest.Config) ([]byte, error) {
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.Host+"/cacerts", nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cacerts: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (c *Controllers) add(run func(context.Context)) {
	c.runs = append(c.runs, run)
}

func (c *Controllers) Run(ctx context.Context) {
	c.factory.Start(ctx.Done())
	c.metadataFactory.Start(ctx.Done())
	close(c.informersStarted)
	c.factory.WaitForCacheSync(ctx.Done())
	for _, run := range c.runs {
		go recovered(ctx, run)
	}
	c.started.Store(true)
	<-ctx.Done()
}

func (c *Controllers) Pending() string {
	if !c.started.Load() {
		return "starting"
	}
	pending := queues.summary()
	if queues.retries.recent(retryPendingFor) {
		pending = strings.TrimPrefix(pending+",retries", ",")
	}
	return pending
}

func (c *Controllers) Idle() bool {
	return c.started.Load() && queues.summary() == "" && !queues.retries.recent(retryQuiet) && !queues.adds.recent(addQuiet)
}

type queueDepths struct {
	mu         sync.Mutex
	depth      map[string]*gauge
	unfinished map[string]*gauge
	retries    retryClock
	adds       retryClock
}

type retryClock struct{ last atomic.Int64 }

func (r *retryClock) Inc() { r.last.Store(time.Now().UnixNano()) }

func (r *retryClock) recent(d time.Duration) bool {
	last := r.last.Load()
	return last != 0 && time.Since(time.Unix(0, last)) < d
}

type gauge struct{ n atomic.Int64 }

func (g *gauge) Inc()          { g.n.Add(1) }
func (g *gauge) Dec()          { g.n.Add(-1) }
func (g *gauge) Set(v float64) { g.n.Store(int64(v)) }

type noop struct{}

func (noop) Inc()            {}
func (noop) Observe(float64) {}

func (q *queueDepths) track(m *map[string]*gauge, name string) *gauge {
	q.mu.Lock()
	defer q.mu.Unlock()
	if *m == nil {
		*m = map[string]*gauge{}
	}
	if g, ok := (*m)[name]; ok {
		return g
	}
	g := &gauge{}
	(*m)[name] = g
	return g
}

func (q *queueDepths) summary() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	var busy []string
	for name, g := range q.depth {
		if g.n.Load() > 0 {
			busy = append(busy, fmt.Sprintf("%s=%d", name, g.n.Load()))
		}
	}
	for name, g := range q.unfinished {
		if g.n.Load() > 0 {
			busy = append(busy, name+"=working")
		}
	}
	sort.Strings(busy)
	return strings.Join(busy, ",")
}

func (q *queueDepths) NewDepthMetric(name string) workqueue.GaugeMetric {
	return q.track(&q.depth, name)
}

func (q *queueDepths) NewUnfinishedWorkSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return q.track(&q.unfinished, name)
}

func (q *queueDepths) NewAddsMetric(string) workqueue.CounterMetric           { return &q.adds }
func (q *queueDepths) NewLatencyMetric(string) workqueue.HistogramMetric      { return noop{} }
func (q *queueDepths) NewWorkDurationMetric(string) workqueue.HistogramMetric { return noop{} }
func (q *queueDepths) NewLongestRunningProcessorSecondsMetric(string) workqueue.SettableGaugeMetric {
	return &gauge{}
}
func (q *queueDepths) NewRetriesMetric(string) workqueue.CounterMetric { return &q.retries }

func recovered(ctx context.Context, run func(context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			println(fmt.Sprintf("controllers: controller stopped: %v", r))
		}
	}()
	run(ctx)
}

var closedChannel = func() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()
