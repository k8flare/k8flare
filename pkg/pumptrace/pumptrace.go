//go:build js && wasm

package pumptrace

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"

	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
)

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
	add, update, del := observe("add"), observe("update"), observe("delete")
	_, _ = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    add,
		UpdateFunc: func(_, obj any) { update(obj) },
		DeleteFunc: del,
	})
}
