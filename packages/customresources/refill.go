package customresources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	clientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	informers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	apiextensionsinformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions/apiextensions/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	discoveryQueue      = "DiscoveryController"
	discoverySettle     = 2 * time.Second
	discoverySettlePoll = 20 * time.Millisecond
)

type refillableInformer struct {
	cache.SharedIndexInformer
	mu       sync.Mutex
	handlers []cache.ResourceEventHandler
}

func registerRefillable(factory informers.SharedInformerFactory) *refillableInformer {
	r := &refillableInformer{}
	factory.InformerFor(&apiextensionsv1.CustomResourceDefinition{}, func(client clientset.Interface, resync time.Duration) cache.SharedIndexInformer {
		r.SharedIndexInformer = apiextensionsinformers.NewCustomResourceDefinitionInformer(client, resync, cache.Indexers{})
		return r
	})
	return r
}

func (r *refillableInformer) AddEventHandler(h cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	r.mu.Lock()
	r.handlers = append(r.handlers, h)
	r.mu.Unlock()
	return r.SharedIndexInformer.AddEventHandler(h)
}

func (r *refillableInformer) AddEventHandlerWithResyncPeriod(h cache.ResourceEventHandler, resync time.Duration) (cache.ResourceEventHandlerRegistration, error) {
	r.mu.Lock()
	r.handlers = append(r.handlers, h)
	r.mu.Unlock()
	return r.SharedIndexInformer.AddEventHandlerWithResyncPeriod(h, resync)
}

func (r *refillableInformer) AddEventHandlerWithOptions(h cache.ResourceEventHandler, opts cache.HandlerOptions) (cache.ResourceEventHandlerRegistration, error) {
	r.mu.Lock()
	r.handlers = append(r.handlers, h)
	r.mu.Unlock()
	return r.SharedIndexInformer.AddEventHandlerWithOptions(h, opts)
}

func (r *refillableInformer) refill(kvs []kine.KV) int {
	want := make(map[string]*apiextensionsv1.CustomResourceDefinition, len(kvs))
	for _, kv := range kvs {
		data, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil {
			continue
		}
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := json.Unmarshal(data, crd); err != nil {
			continue
		}
		crd.ResourceVersion = strconv.FormatInt(kv.ModRevision, 10)
		want[strings.TrimPrefix(kv.Key, crdStoragePrefix)] = crd
	}
	indexer := r.GetIndexer()
	r.mu.Lock()
	handlers := append([]cache.ResourceEventHandler(nil), r.handlers...)
	r.mu.Unlock()
	changed := 0
	for _, obj := range indexer.List() {
		old := obj.(*apiextensionsv1.CustomResourceDefinition)
		if _, ok := want[old.Name]; ok {
			continue
		}
		indexer.Delete(old)
		changed++
		for _, h := range handlers {
			h.OnDelete(old)
		}
	}
	for name, crd := range want {
		obj, exists, _ := indexer.GetByKey(name)
		if !exists {
			indexer.Add(crd)
			changed++
			for _, h := range handlers {
				h.OnAdd(crd, false)
			}
			continue
		}
		old := obj.(*apiextensionsv1.CustomResourceDefinition)
		if old.ResourceVersion == crd.ResourceVersion {
			continue
		}
		indexer.Update(crd)
		changed++
		for _, h := range handlers {
			h.OnUpdate(old, crd)
		}
	}
	return changed
}

type discoveryActivity struct {
	pending atomic.Int64
}

var discoveryWork = &discoveryActivity{}

func init() {
	workqueue.SetProvider(discoveryWork)
}

func settleDiscovery(ctx context.Context) {
	deadline := time.Now().Add(discoverySettle)
	for discoveryWork.pending.Load() > 0 && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(discoverySettlePoll):
		}
	}
}

type pendingGauge struct{ a *discoveryActivity }

func (g pendingGauge) Inc() { g.a.pending.Add(1) }
func (g pendingGauge) Dec() {}

type startCounter struct{ a *discoveryActivity }

func (c startCounter) Observe(float64) {}

type doneCounter struct{ a *discoveryActivity }

func (c doneCounter) Observe(float64) { c.a.pending.Add(-1) }

type noopMetric struct{}

func (noopMetric) Inc()            {}
func (noopMetric) Dec()            {}
func (noopMetric) Set(float64)     {}
func (noopMetric) Observe(float64) {}

func (a *discoveryActivity) NewDepthMetric(name string) workqueue.GaugeMetric {
	if name == discoveryQueue {
		return pendingGauge{a}
	}
	return noopMetric{}
}
func (a *discoveryActivity) NewAddsMetric(string) workqueue.CounterMetric      { return noopMetric{} }
func (a *discoveryActivity) NewLatencyMetric(string) workqueue.HistogramMetric { return noopMetric{} }
func (a *discoveryActivity) NewWorkDurationMetric(name string) workqueue.HistogramMetric {
	if name == discoveryQueue {
		return doneCounter{a}
	}
	return noopMetric{}
}
func (a *discoveryActivity) NewUnfinishedWorkSecondsMetric(string) workqueue.SettableGaugeMetric {
	return noopMetric{}
}
func (a *discoveryActivity) NewLongestRunningProcessorSecondsMetric(string) workqueue.SettableGaugeMetric {
	return noopMetric{}
}
func (a *discoveryActivity) NewRetriesMetric(string) workqueue.CounterMetric { return noopMetric{} }

var instanceID = strconv.FormatInt(time.Now().UnixNano()%1_000_000_007, 36)

func refillHeader(w http.ResponseWriter, n int) {
	w.Header().Set("X-CRD-Refill", strconv.Itoa(n))
}
