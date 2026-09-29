package workloads

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"k8s.io/client-go/util/workqueue"
)

type counters struct {
	depth    atomic.Int64
	inFlight atomic.Int64
}

type activity struct {
	mu     sync.Mutex
	queues map[string]*counters
}

var work = &activity{queues: map[string]*counters{}}

func init() {
	workqueue.SetProvider(work)
}

func (a *activity) of(name string) *counters {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.queues[name]
	if !ok {
		c = &counters{}
		a.queues[name] = c
	}
	return c
}

func (a *activity) idle(owned func(string) bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for name, c := range a.queues {
		if owned(name) && (c.depth.Load() != 0 || c.inFlight.Load() != 0) {
			return false
		}
	}
	return true
}

func (a *activity) inFlight(owned func(string) bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for name, c := range a.queues {
		if owned(name) && c.inFlight.Load() != 0 {
			return true
		}
	}
	return false
}

func (a *activity) reset(owned func(string) bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for name := range a.queues {
		if owned(name) {
			delete(a.queues, name)
		}
	}
}

func (a *activity) busy(owned func(string) bool) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var names []string
	for name, c := range a.queues {
		if owned(name) && (c.depth.Load() != 0 || c.inFlight.Load() != 0) {
			names = append(names, name+"="+strconv.FormatInt(c.depth.Load(), 10)+"/"+strconv.FormatInt(c.inFlight.Load(), 10))
		}
	}
	return strings.Join(names, " ")
}

func workloadQueue(string) bool { return true }

type depthGauge struct{ c *counters }

func (g depthGauge) Inc() { g.c.depth.Add(1) }
func (g depthGauge) Dec() { g.c.depth.Add(-1) }

type getCounter struct{ c *counters }

func (g getCounter) Observe(float64) { g.c.inFlight.Add(1) }

type doneCounter struct{ c *counters }

func (g doneCounter) Observe(float64) { g.c.inFlight.Add(-1) }

type noop struct{}

func (noop) Inc()            {}
func (noop) Set(float64)     {}
func (noop) Observe(float64) {}

func (a *activity) NewDepthMetric(name string) workqueue.GaugeMetric { return depthGauge{a.of(name)} }
func (a *activity) NewAddsMetric(string) workqueue.CounterMetric     { return noop{} }
func (a *activity) NewLatencyMetric(name string) workqueue.HistogramMetric {
	return getCounter{a.of(name)}
}
func (a *activity) NewWorkDurationMetric(name string) workqueue.HistogramMetric {
	return doneCounter{a.of(name)}
}
func (a *activity) NewUnfinishedWorkSecondsMetric(string) workqueue.SettableGaugeMetric {
	return noop{}
}
func (a *activity) NewLongestRunningProcessorSecondsMetric(string) workqueue.SettableGaugeMetric {
	return noop{}
}
func (a *activity) NewRetriesMetric(string) workqueue.CounterMetric { return noop{} }
