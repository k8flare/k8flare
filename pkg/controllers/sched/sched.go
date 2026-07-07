//go:build js && wasm

// Package sched is deliberately a separate package from its parent
// pkg/controllers: merely importing k8s.io/kubernetes/pkg/scheduler links
// its init-time closure (scheme registration, metrics, the CEL evaluator
// behind the DRA machinery, ...) into ANY binary that imports the
// package, dead-code elimination notwithstanding -- keeping RunScheduler
// here means the kube-controller-manager binary (workers/controllers,
// which must stay under the Worker Loader's 64MiB cap on its own) never
// pays for the scheduler tree, and vice versa.
package sched

import (
	"context"
	"fmt"

	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"

	"k8s.io/kubernetes/pkg/scheduler"
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/latest"

	leanclientset "github.com/k8flare/k8flare/pkg/leanclient/clientset"
)

// RunScheduler starts the real, unmodified upstream kube-scheduler
// (via k8s.io/kubernetes/pkg/scheduler, not cmd/kube-scheduler/app) against
// restCfg, and blocks until ctx is canceled.
//
// This does NOT go through cmd/kube-scheduler/app's NewSchedulerCommand /
// Setup / options.Options: that path loads its ComponentConfig from a real
// file via clientcmd/config-file parsing (see cmd/scheduler/main.go's doc
// comment -- same reason RunControllerManager bypasses
// NewControllerManagerCommand), which this environment cannot support.
// Instead this hand-wires exactly what Setup's own body does (verified by
// reading cmd/kube-scheduler/app/server.go's Setup and Run, not assumed):
// a typed client, scheduler.NewInformerFactory (the exported helper
// kube-scheduler itself uses, not a bare informers.NewSharedInformerFactory
// -- it adds a scheduler-specific Pod informer transform), an
// events.EventBroadcasterAdapter-backed RecorderFactory, and a
// scheduler.New call passing the same Options Setup does apart from the
// ones that only matter for flag/logging plumbing this repo doesn't have
// (WithFrameworkOutOfTreeRegistry with a populated registry, WithExtenders,
// WithBuildFrameworkCapturer, WithPodMaxInUnschedulablePodsDuration -- the
// last already defaults to the library's own internalqueue default when
// omitted, same value Setup's cc.PodMaxInUnschedulablePodsDuration would
// carry unless a non-default flag were passed, which nothing here does).
//
// Until GOOS=js support existed for k8s.io/kubernetes's scheduler package
// tree at all (see docs/platform-verification.md's honest-correction entry
// and third_party/k8s-js-overlays/README.md for why and how that was
// unblocked), kube-scheduler was BYO-VM/host-process-only; this restores
// it to workers/controllers on the same terms RunControllerManager already
// established for kube-controller-manager.
func RunScheduler(ctx context.Context, restCfg *restclient.Config) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("scheduler: panic: %v", r)
		}
	}()

	client, err := leanclientset.NewSchedulerClientsetForConfig(restclient.AddUserAgent(restCfg, "kube-scheduler"))
	if err != nil {
		return fmt.Errorf("scheduler: build client: %w", err)
	}

	factory := scheduler.NewInformerFactory(client, 0)

	cfg, err := latest.Default()
	if err != nil {
		return fmt.Errorf("scheduler: default config: %w", err)
	}
	// Matches cmd/scheduler/main.go's writeSchedulerConfig: disable the
	// PV/PVC/StorageClass-touching plugins this apiserver can't back.
	// latest.Default() already allocates Profiles[0].Plugins non-nil with
	// these four present (and enabled) in MultiPoint, verified by probing
	// its return value directly rather than assumed.
	cfg.Profiles[0].Plugins.MultiPoint.Disabled = []schedulerapi.Plugin{
		{Name: "VolumeBinding"},
		{Name: "VolumeRestrictions"},
		{Name: "NodeVolumeLimits"},
		{Name: "VolumeZone"},
		// DynamicResources must be disabled on GOOS=js: the js half of the
		// scheduler-registry overlay (third_party/k8s-js-overlays/
		// scheduler-registry_js.go) drops it from the in-tree registry to
		// fit the Worker Loader's 64MiB cap, and a profile that names a
		// plugin missing from the registry fails framework construction.
		// DRA is unusable against this apiserver anyway (ResourceClaim/
		// ResourceSlice/DeviceClass are permanently-empty stubs).
		{Name: "DynamicResources"},
	}

	recorderAdapter := events.NewEventBroadcasterAdapter(client)

	sched, err := scheduler.New(ctx,
		client,
		factory,
		nil, // dynInformerFactory: only reached as a fallback for out-of-tree GVK event sources (eventhandlers.go) that no in-tree, default-profile plugin registers; nil is upstream's own documented safe default ("Tests may not instantiate dynInformerFactory").
		recorderAdapter.NewRecorder,
		scheduler.WithComponentConfigVersion(cfg.TypeMeta.APIVersion),
		scheduler.WithKubeConfig(restCfg),
		scheduler.WithProfiles(cfg.Profiles...),
		scheduler.WithPercentageOfNodesToScore(cfg.PercentageOfNodesToScore),
		scheduler.WithPodMaxBackoffSeconds(cfg.PodMaxBackoffSeconds),
		scheduler.WithPodInitialBackoffSeconds(cfg.PodInitialBackoffSeconds),
		scheduler.WithParallelism(cfg.Parallelism),
	)
	if err != nil {
		return fmt.Errorf("scheduler: new scheduler: %w", err)
	}

	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())

	sched.Run(ctx) // blocks until ctx is canceled, same as Setup's own callers
	return ctx.Err()
}
