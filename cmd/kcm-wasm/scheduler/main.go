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

	"github.com/k8flare/k8flare/pkg/cfruntime"
	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
	"github.com/k8flare/k8flare/pkg/controllers"
	"github.com/k8flare/k8flare/pkg/controllers/sched"
)

func getToken() string {
	if t := cloudflare.Getenv("K3S_TOKEN"); t != "" {
		return t
	}
	return "k8flare-dev-token" // fallback for dev, matches every other Worker in this repo
}

func main() {
	restCfg := controllers.RestConfig("GATEWAY", getToken(), cloudflare.Getenv("CLUSTER_BASE_PATH"))
	workers.ResidentService("scheduler", func(ctx context.Context) error {
		return sched.RunScheduler(ctx, restCfg)
	})
}
