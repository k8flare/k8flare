//go:build js && wasm

// Command controllers is the Go WASM entrypoint hosted inside
// workers/k8flare's Controllers Durable Object (src/controllers/index.ts). It
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
// contained pkg/cfruntime/ (a used-only subset of one upstream library,
// absorbed rather than vendored as a full fork) and was not attempted here --
// this is a new, material finding beyond what spikes/s8-wasm-resident's
// toy Go programs could have caught (they never imported the real
// kube-scheduler/kube-controller-manager package trees), recorded in
// docs/platform-verification.md's S8 section per CLAUDE.md's honest-
// correction rule. cmd/scheduler is unchanged and still works as a BYO VM
// / host-process binary (see .github/workflows/e2e-conformance.yml, which
// runs it exactly that way); it is just not part of the Controllers DO.
package main

import (
	"context"

	"github.com/k8flare/k8flare/pkg/cfruntime"
	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
	"github.com/k8flare/k8flare/pkg/controllers"
)

func main() {
	token := cloudflare.GetenvDefault("K3S_TOKEN", "k8flare-dev-token")
	restCfg := controllers.RestConfig("GATEWAY", token, cloudflare.Getenv("CLUSTER_BASE_PATH"))
	workers.ResidentService("controllerManager", func(ctx context.Context) error {
		return controllers.RunControllerManager(ctx, restCfg)
	})
}
