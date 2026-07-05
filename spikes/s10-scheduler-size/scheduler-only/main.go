//go:build js && wasm

// Command scheduler-only is throwaway size-measurement scaffolding for
// Phase 10 Stage A -- see ../README.md. Not a real Worker entrypoint.
package main

import (
	"context"

	"github.com/k8flare/k8flare/pkg/controllers"
)

func main() {
	restCfg := controllers.RestConfig("GATEWAY", "measurement-only")
	go controllers.RunScheduler(context.Background(), restCfg)
	select {}
}
