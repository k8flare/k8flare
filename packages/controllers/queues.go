package controllers

import (
	"sync"
	"sync/atomic"

	"k8s.io/client-go/util/workqueue"
)

type queueDepths struct {
	mu     sync.Mutex
	depths []*atomic.Int64
}

func trackQueueDepths() *queueDepths {
	q := &queueDepths{}
	workqueue.SetProvider(q)
	return q
}

func (q *queueDepths) total() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	var n int64
	for _, d := range q.depths {
		n += d.Load()
	}
	return n
}

type depth struct{ n *atomic.Int64 }

func (d depth) Inc() { d.n.Add(1) }
func (d depth) Dec() { d.n.Add(-1) }

type noop struct{}

func (noop) Inc()            {}
func (noop) Observe(float64) {}
func (noop) Set(float64)     {}

func (q *queueDepths) NewDepthMetric(string) workqueue.GaugeMetric {
	q.mu.Lock()
	defer q.mu.Unlock()
	d := &atomic.Int64{}
	q.depths = append(q.depths, d)
	return depth{d}
}

func (q *queueDepths) NewAddsMetric(string) workqueue.CounterMetric           { return noop{} }
func (q *queueDepths) NewLatencyMetric(string) workqueue.HistogramMetric      { return noop{} }
func (q *queueDepths) NewWorkDurationMetric(string) workqueue.HistogramMetric { return noop{} }
func (q *queueDepths) NewUnfinishedWorkSecondsMetric(string) workqueue.SettableGaugeMetric {
	return noop{}
}
func (q *queueDepths) NewLongestRunningProcessorSecondsMetric(string) workqueue.SettableGaugeMetric {
	return noop{}
}
func (q *queueDepths) NewRetriesMetric(string) workqueue.CounterMetric { return noop{} }
