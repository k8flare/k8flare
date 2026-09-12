//go:build js && wasm

package cloudflare

import (
	"fmt"
	"log"
	"sync"
	"syscall/js"
	"time"
)

const pumpTraceVar = "PUMP_TRACE"

var (
	pumpTraceOnce sync.Once
	pumpTraceOn   bool
)

// PumpTraceEnabled reports whether this deployment asked for pump-window
// boundary tracing.
//
// Answered once per isolate and cached. The var is set at deploy time and
// cannot change under a running isolate, and this is asked on the hot path
// -- fetch.RoundTrip calls it for every outbound request every resident
// controller makes. Reading it live cost three syscall/js boundary
// crossings per request whether or not anyone was measuring, which is the
// opposite of what this knob promises.
//
// It reads the runtime context directly rather than through Getenv because
// Getenv panics when there is no context yet, and an observability path
// that can crash the control plane is worse than no observability path:
// with no context, there is nothing to trace, so the answer is no.
func PumpTraceEnabled() bool {
	pumpTraceOnce.Do(func() { pumpTraceOn = readPumpTraceVar() })
	return pumpTraceOn
}

func readPumpTraceVar() bool {
	ctx := js.Global().Get("context")
	if ctx.Type() != js.TypeObject {
		return false
	}
	env := ctx.Get("env")
	if env.Type() != js.TypeObject {
		return false
	}
	return env.Get(pumpTraceVar).String() == "1"
}

// PumpTrace emits one boundary observation, attributed to the pump window
// it happened inside and to the component that made it. With tracing off
// it emits nothing and costs one cached bool read -- the control plane
// must cost the same when nobody is measuring it.
func PumpTrace(boundary, component, object, resourceVersion string) {
	if !PumpTraceEnabled() {
		return
	}
	log.Printf("pumptrace %s", formatPumpTrace(boundary, component+"@"+InstanceID(), object, resourceVersion, CurrentWindowID(), time.Now()))
}

func formatPumpTrace(boundary, component, object, resourceVersion string, window int, at time.Time) string {
	return fmt.Sprintf(`{"b":%q,"c":%q,"w":%d,"o":%q,"rv":%q,"t":%d}`,
		boundary, component, window, object, resourceVersion, at.UnixMilli())
}
