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
				err := run(context.Background())
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
