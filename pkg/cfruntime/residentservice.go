//go:build js && wasm

package workers

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
)

// ResidentService starts run at most once per WASM instance, under
// cloudflare.WaitUntil so it keeps making real progress after the
// triggering request's own response closes (S8 finding: a single
// WaitUntil call keeps every goroutine on the instance pumped, not just
// the one that registered it). It serves /healthz reporting label's
// status and blocks forever -- the shared shape behind
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
			cloudflare.WaitUntil(func() {
				setStatus("running")
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
				err := run(ctx)
				log.Printf("%s: exited: %v", label, err)
				setStatus("exited: " + errString(err))
			})
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
