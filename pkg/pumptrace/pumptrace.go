//go:build js && wasm

package pumptrace

import (
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"

	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
)

// Per-event trace lines are written to the dynamic worker's console, and
// that output is dropped under load: a burst of a hundred writes in ten
// seconds showed four of them (docs/platform-verification.md S56). So the
// counts are kept in memory and reported on a timer instead -- one line
// every few seconds survives any burst, and the difference between what
// the apiserver accepted and what the informer delivered is the number
// that P0-7 turns on.
var (
	countsMu     sync.Mutex
	counts       = map[string]int{}
	reporterOnce sync.Once
	reportEvery  = 5 * time.Second
)

func countObservation(component, verb string) {
	countsMu.Lock()
	counts[component+"."+verb]++
	countsMu.Unlock()
}

func startCountReporter(component string) {
	reporterOnce.Do(func() {
		go func() {
			for {
				time.Sleep(reportEvery)
				countsMu.Lock()
				snapshot := make(map[string]int, len(counts))
				for k, v := range counts {
					snapshot[k] = v
				}
				countsMu.Unlock()
				for k, v := range snapshot {
					cloudflare.PumpTrace("observed.count", k, "", itoa(v))
				}
			}
		}()
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// Observations makes every event informer delivers emit one
// boundary observation attributed to component and to the pump window it
// arrived in. It is what separates "the commit was slow to reach the
// controller" from "the controller was slow to act on it" -- the two are
// indistinguishable from the outside, and guessing between them is how
// the previous attempt at this measurement went wrong.
//
// It registers no handler at all when tracing is off, rather than
// registering one that returns early: a handler costs a call per event
// per object even when it does nothing, and an unmeasured cluster must
// cost what it always did.
func Observations(informer cache.SharedIndexInformer, component string) {
	if !cloudflare.PumpTraceEnabled() {
		return
	}
	observe := func(verb string) func(any) {
		return func(obj any) {
			if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = tombstone.Obj
			}
			m, err := meta.Accessor(obj)
			if err != nil {
				return
			}
			cloudflare.PumpTrace("observed."+verb, component, m.GetNamespace()+"/"+m.GetName(), m.GetResourceVersion())
		}
	}
	counted := func(verb string, inner func(any)) func(any) {
		return func(obj any) {
			countObservation(component, verb)
			inner(obj)
		}
	}
	add, update, del := counted("add", observe("add")), counted("update", observe("update")), counted("delete", observe("delete"))
	startCountReporter(component)
	_, _ = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    add,
		UpdateFunc: func(_, obj any) { update(obj) },
		DeleteFunc: del,
	})
}
