package workloads

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

var pending = &deadlines{}

func init() {
	workqueue.SetProvider(work)
	workqueue.DelayObserver = pending.add
}

type deadlines struct {
	mu    sync.Mutex
	at    []time.Time
	muted int
}

func (d *deadlines) withoutBooking(deliver func()) {
	d.mu.Lock()
	d.muted++
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.muted--
		d.mu.Unlock()
	}()
	deliver()
}

func (d *deadlines) add(delay time.Duration) {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.muted > 0 {
		return
	}
	kept := d.at[:0]
	for _, t := range d.at {
		if t.After(now) {
			kept = append(kept, t)
		}
	}
	d.at = append(kept, now.Add(delay))
}

func (d *deadlines) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.at = nil
}

func (d *deadlines) next() (time.Duration, bool) {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	var earliest time.Time
	for _, t := range d.at {
		if t.After(now) && (earliest.IsZero() || t.Before(earliest)) {
			earliest = t
		}
	}
	if earliest.IsZero() {
		return 0, false
	}
	return earliest.Sub(now), true
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
