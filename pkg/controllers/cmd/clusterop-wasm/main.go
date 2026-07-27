//go:build js && wasm

// Command clusterop is the Go WASM entrypoint for the cluster operator
// (pkg/controllers/clusterop), hosted as a FOURTH Loader-loaded dynamic
// worker inside packages/k8flare-worker's Controllers Durable Object
// (src/controllers/index.ts) alongside kcm, sched, and gc. It only ever
// loads in the management ("default") cluster -- tenant clusters have no
// Cluster objects to reconcile, and giving each one an operator would put
// a Loader charge on every cluster for nothing.
//
// Its own binary rather than a passenger in gc's: sharing a package or a
// binary links the other's reachable controller code regardless of
// whether it runs (see pkg/controllers/restconfig's doc comment for the
// ~4MB measurement behind that rule).
package main

import (
	"context"

	"github.com/k8flare/k8flare/pkg/cfruntime"
	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
	"github.com/k8flare/k8flare/pkg/controllers/clusterop"
	"github.com/k8flare/k8flare/pkg/controllers/restconfig"
)

func main() {
	token := cloudflare.GetenvDefault("K3S_TOKEN", "k8flare-dev-token")
	restCfg := restconfig.RestConfig("GATEWAY", token, cloudflare.Getenv("CLUSTER_BASE_PATH"))
	workers.ResidentService("clusterOperator", func(ctx context.Context) error {
		return clusterop.Run(ctx, restCfg, "GATEWAY", token)
	})
}
