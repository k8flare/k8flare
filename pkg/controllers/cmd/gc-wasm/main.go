//go:build js && wasm

// Command gc is the Go WASM entrypoint for the real, unmodified upstream
// garbagecollector controller (ownerReferences cascade delete), hosted
// as a THIRD Loader-loaded dynamic worker inside packages/k8flare-worker's
// Controllers Durable Object (src/controllers/index.ts), alongside the
// kube-controller-manager one built from ../kcm-wasm and the (not yet
// buildable) scheduler one. Deliberately its own isolate rather than
// folded into RunControllerManager's six workload controllers: the
// garbage collector's dependency-graph builder needs an informer/watch
// per resource type across this apiserver's entire apidef.Table, which
// would compete for the same 128MiB isolate memory budget that already
// forced five other controllers out of that binary once (see
// pkg/controllers/controllermanager.go's doc comment, 2026-07-06
// finding). See pkg/controllers/gc for the real wiring
// (metadata.Interface client, static RESTMapper, GVR-generic informer
// factory) and why this replaces pkg/apiserver's old, synchronous-in-
// request CascadeDeleteDependents mechanism -- and pkg/controllers/
// restconfig's doc comment for why this imports that instead of
// pkg/controllers itself (which would link the six workload
// controllers' reachable code into this binary for nothing).
package main

import (
	"context"

	"github.com/k8flare/k8flare/pkg/cfruntime"
	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
	"github.com/k8flare/k8flare/pkg/controllers/gc"
	"github.com/k8flare/k8flare/pkg/controllers/restconfig"
)

func main() {
	token := cloudflare.GetenvDefault("K3S_TOKEN", "k8flare-dev-token")
	restCfg := restconfig.RestConfig("GATEWAY", token, cloudflare.Getenv("CLUSTER_BASE_PATH"))
	workers.ResidentService("garbageCollector", func(ctx context.Context) error {
		return gc.RunGarbageCollector(ctx, restCfg)
	})
}
