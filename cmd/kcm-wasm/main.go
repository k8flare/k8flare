//go:build js && wasm

// Command controllers is the Go WASM entrypoint hosted inside
// workers/controllers' Controllers Durable Object (src/index.ts). It
// starts the real, unmodified upstream kube-controller-manager (via
// pkg/controllers.RunControllerManager) once per DO instance and keeps it
// running for as long as that instance stays resident -- see
// docs/platform-verification.md's S8 section ("DO-hosted + WaitUntil-
// resident + event-armed alarm() safety net") for the execution shape and
// its verification, and pkg/controllers/controllermanager.go's doc
// comment for why cmd/controller-manager's normal file-based kubeconfig
// startup path can't be reused as-is here.
//
// kube-scheduler is NOT hosted here. Unlike kube-controller-manager, the
// real k8s.io/kubernetes/pkg/scheduler package has an unconditional,
// compile-time dependency on syscall.SIGUSR2 (a debug cache-compare
// signal handler, pkg/scheduler/backend/cache/debugger/signal.go,
// imported directly by pkg/scheduler/scheduler.go) with no GOOS=js build
// variant anywhere in k8s.io/kubernetes -- confirmed by actually building
// for GOOS=js/wasm, not assumed. Fixing it would require vendoring a
// patched fork of all of k8s.io/kubernetes (~107MB, 5,200+ files in this
// repo's pinned k3s-io/kubernetes v1.36.2-k3s1) into this repository,
// which is a fundamentally different scale of commitment than the small,
// contained third_party/syumai-workers-fork/ (a few hundred KB, one
// upstream library, two changed files) and was not attempted here --
// this is a new, material finding beyond what spikes/s8-wasm-resident's
// toy Go programs could have caught (they never imported the real
// kube-scheduler/kube-controller-manager package trees), recorded in
// docs/platform-verification.md's S8 section per CLAUDE.md's honest-
// correction rule. cmd/scheduler is unchanged and still works as a BYO VM
// / host-process binary (see .github/workflows/e2e-conformance.yml, which
// runs it exactly that way); it is just not part of workers/controllers.
package main

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/k8flare/k8flare/pkg/controllers"
	"github.com/syumai/workers"
	"github.com/syumai/workers/cloudflare"
)

var (
	startOnce sync.Once
	kcmStatus = "not started"
	mu        sync.Mutex
)

func getToken() string {
	if t := cloudflare.Getenv("K3S_TOKEN"); t != "" {
		return t
	}
	return "k8flare-dev-token" // fallback for dev, matches every other Worker in this repo
}

func setStatus(msg string) {
	mu.Lock()
	kcmStatus = msg
	mu.Unlock()
}

// ensureStarted starts the resident controller-manager goroutine at most
// once per WASM instance (idempotent -- every dispatched request, and the
// DO's alarm() safety net, call this the same way). Wrapped in
// cloudflare.WaitUntil so it keeps making real progress after this
// triggering request's own response closes (S8 finding: a single
// WaitUntil call keeps the whole shared scheduler pumped for every
// goroutine on the instance, including the per-controller goroutines
// RunControllerManager itself starts).
func ensureStarted() {
	startOnce.Do(func() {
		restCfg := controllers.RestConfig("GATEWAY", getToken())
		cloudflare.WaitUntil(func() {
			ctx := context.Background()
			setStatus("running")
			err := controllers.RunControllerManager(ctx, restCfg)
			log.Printf("controllers: controller-manager exited: %v", err)
			setStatus("exited: " + errString(err))
		})
	})
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ensureStarted()
		mu.Lock()
		status := kcmStatus
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"controllerManager":"` + status + `"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ensureStarted()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	workers.ServeNonBlock(mux)
	workers.Ready()
	select {} // park forever; do not depend on any single request's Done() (S8 (a)/resident pattern)
}
