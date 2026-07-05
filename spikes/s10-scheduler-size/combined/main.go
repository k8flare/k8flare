//go:build js && wasm

// Command combined is throwaway size-measurement scaffolding for Phase 10
// Stage A -- see ../README.md. Not a real Worker entrypoint: it calls both
// Run functions with independent informer factories, which is irrelevant
// for a size measurement (see ../README.md for why).
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
