package workloads

import (
	"sync/atomic"

	"k8s.io/client-go/util/workqueue"
)

type activity struct {
	depth    atomic.Int64
	inFlight atomic.Int64
}

var work = &activity{}

func init() {
	workqueue.SetProvider(work)
}

func (a *activity) idle() bool { return a.depth.Load() == 0 && a.inFlight.Load() == 0 }

type depthGauge struct{ a *activity }

func (g depthGauge) Inc() { g.a.depth.Add(1) }
func (g depthGauge) Dec() { g.a.depth.Add(-1) }

type getCounter struct{ a *activity }

func (c getCounter) Observe(float64) { c.a.inFlight.Add(1) }

type doneCounter struct{ a *activity }

func (c doneCounter) Observe(float64) { c.a.inFlight.Add(-1) }

type noop struct{}

func (noop) Inc()            {}
func (noop) Set(float64)     {}
func (noop) Observe(float64) {}

func (a *activity) NewDepthMetric(string) workqueue.GaugeMetric            { return depthGauge{a} }
func (a *activity) NewAddsMetric(string) workqueue.CounterMetric           { return noop{} }
func (a *activity) NewLatencyMetric(string) workqueue.HistogramMetric      { return getCounter{a} }
func (a *activity) NewWorkDurationMetric(string) workqueue.HistogramMetric { return doneCounter{a} }
func (a *activity) NewUnfinishedWorkSecondsMetric(string) workqueue.SettableGaugeMetric {
	return noop{}
}
func (a *activity) NewLongestRunningProcessorSecondsMetric(string) workqueue.SettableGaugeMetric {
	return noop{}
}
func (a *activity) NewRetriesMetric(string) workqueue.CounterMetric { return noop{} }
