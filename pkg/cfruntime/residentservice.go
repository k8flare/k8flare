//go:build js && wasm

package workers

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
)

// ResidentService starts run at most once per WASM instance and keeps it
// going for as long as some request's pump window is open (S8 finding: a
// single open request keeps every goroutine on the instance pumped, not
// just the one that registered it; the bootstrap's per-dispatch
// ctx.waitUntil timer is what holds one open).
//
// It used to register run under cloudflare.WaitUntil on the request that
// instantiated the isolate, on the premise that that request stayed alive
// for the instance's lifetime and could therefore carry every outbound
// call the controllers make. Production disproved the premise: the
// runtime tears that request down anyway, after which its bindings'
// promises never settle and every reflector wedges silently (S31). Nothing
// may be anchored to one particular request now -- outbound I/O finds a
// live window per call (cloudflare.Window).
//
// It serves /healthz reporting label's status and blocks forever -- the
// shared shape behind
// pkg/controllers/cmd/kcm-wasm's and its scheduler subdirectory's WASM
// entrypoints, which differ only in which real upstream binary run
// starts.
func ResidentService(label string, run func(ctx context.Context) error) {
	var (
		startOnce sync.Once
		status    = "not started"
		mu        sync.Mutex
	)
	setStatus := func(msg string) {
		mu.Lock()
		status = msg
		mu.Unlock()
	}
	ensureStarted := func() {
		startOnce.Do(func() {
			setStatus("running")
			go func() {
				// WithCancel, NOT bare context.Background(): Background's
				// Done() returns a nil channel by spec ("Done may return
				// nil if this context can never be canceled"), and real
				// upstream k8s code passes ctx.Done() around as a stop
				// channel where nil silently changes behavior -- it broke
				// the GC informer factory's started-check AND deadlocked
				// the scheduler's factory.WaitForCacheSync (both found
				// live, 2026-07-09/10). A cancellable context's Done() is
				// a real channel that simply never closes here, which is
				// what "runs for the instance's lifetime" actually means.
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				log.Printf("%s: run starting (instance %s)", label, cloudflare.InstanceID())
				err := run(ctx)
				// A resident controller returning at all is a fault: it is
				// supposed to block for the instance's lifetime. Logged
				// loudly because a silent exit looks exactly like a stalled
				// reconcile from the outside (docs/cluster-api-design.md's
				// intermittent production stall).
				log.Printf("%s: RUN RETURNED (controller is no longer running): %v", label, err)
				setStatus("exited: " + errString(err))
			}()
		})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ensureStarted()
		mu.Lock()
		s := status
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"` + label + `":"` + s + `"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ensureStarted()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	ServeNonBlock(mux)
	Ready()
	select {} // park forever; do not depend on any single request's Done() (S8 (a)/resident pattern)
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
