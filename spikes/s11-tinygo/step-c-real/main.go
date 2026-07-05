//go:build js && wasm

// Command step-c-real is throwaway TinyGo-viability scaffolding for the
// S11 spike -- see ../README.md. Not a real Worker entrypoint. Mirrors
// spikes/s10-scheduler-size/combined/main.go's shape (call both Run
// functions to force the linker/typechecker to pull in their full
// transitive dependency graph) but built with tinygo instead of stock go,
// to get the real, un-guessed error for the actual scheduler/KCM code
// path rather than assuming it inherits step-b's failure.
package main

import (
	"context"

	"github.com/k8flare/k8flare/pkg/controllers"
)

func main() {
	restCfg := controllers.RestConfig("GATEWAY", "measurement-only")
	go controllers.RunScheduler(context.Background(), restCfg)
	go controllers.RunControllerManager(context.Background(), restCfg)
	select {}
}
