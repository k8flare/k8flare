//go:build js && wasm

// Command scheduler is the Go WASM entrypoint for the second dynamic
// worker workers/controllers' Controllers DO loads (alongside the
// kube-controller-manager one built from the parent directory): the
// real, unmodified upstream kube-scheduler, started via
// pkg/controllers.RunScheduler. It is NOT a separately deployed Worker --
// its wasm-opt'd binary ships in workers/controllers' Static Assets
// (sched.* chunks, scripts/build-controllers-wasm.sh) and runs as a
// Loader-loaded dynamic worker, exactly like the KCM binary.
//
// The scheduler and KCM cannot share one binary: the combined build is
// 71.5MB after wasm-opt -Oz, over the Loader's hard 64MiB
// total-module-bytes cap, while each half fits on its own (KCM ~62.5MB,
// scheduler ~66.4MB with the DynamicResources registry overlay --
// measured 2026-07-05, see docs/platform-verification.md's S14 section).
//
// Execution shape is identical to ../main.go (see its doc comment):
// instantiate once per isolate, start the resident loop on first
// dispatch under cloudflare.WaitUntil, park forever; every poke's
// bounded pump window (BOOTSTRAP_JS in ../src/index.ts) keeps the
// goroutines serviced while there is work.
package main

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/k8flare/k8flare/pkg/controllers"
	"github.com/k8flare/k8flare/pkg/controllers/sched"
	"github.com/syumai/workers"
	"github.com/syumai/workers/cloudflare"
)

var (
	startOnce   sync.Once
	schedStatus = "not started"
	mu          sync.Mutex
)

func getToken() string {
	if t := cloudflare.Getenv("K3S_TOKEN"); t != "" {
		return t
	}
	return "k8flare-dev-token" // fallback for dev, matches every other Worker in this repo
}

func setStatus(msg string) {
	mu.Lock()
	schedStatus = msg
	mu.Unlock()
}

// ensureStarted starts the resident scheduler goroutine at most once per
// WASM instance -- same idempotent shape as ../main.go's ensureStarted.
func ensureStarted() {
	startOnce.Do(func() {
		restCfg := controllers.RestConfig("GATEWAY", getToken(), cloudflare.Getenv("CLUSTER_BASE_PATH"))
		cloudflare.WaitUntil(func() {
			ctx := context.Background()
			setStatus("running")
			err := sched.RunScheduler(ctx, restCfg)
			log.Printf("scheduler: exited: %v", err)
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
		status := schedStatus
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"scheduler":"` + status + `"}`))
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
