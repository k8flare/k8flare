package controllers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/metadata/metadatainformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/controller-manager/pkg/informerfactory"
	"k8s.io/kubernetes/pkg/controller/garbagecollector"
)

const (
	workers                    = 5
	minResyncPeriod            = 12 * time.Hour
	garbageCollectorWorkers    = 20
	garbageCollectorSyncPeriod = 30 * time.Second
	retryQuiet                 = 20 * time.Second
	retryPendingFor            = 5 * time.Minute
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
	c := &Controllers{factory: factory, informersStarted: make(chan struct{})}
	metadataClient, err := metadata.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
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
	return c, nil
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
	return c.started.Load() && queues.summary() == "" && !queues.retries.recent(retryQuiet)
}

type queueDepths struct {
	mu         sync.Mutex
	depth      map[string]*gauge
	unfinished map[string]*gauge
	retries    retryClock
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

func (q *queueDepths) NewAddsMetric(string) workqueue.CounterMetric           { return noop{} }
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
