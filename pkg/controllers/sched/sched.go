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
	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
	"github.com/k8flare/k8flare/pkg/pumptrace"
	"runtime/debug"

	utilfeature "k8s.io/apiserver/pkg/util/feature"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"

	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler"
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
// and pkg/k8s-js-overlays/README.md for why and how that was
// unblocked), kube-scheduler was BYO-VM/host-process-only; this restores
// it to workers/controllers on the same terms RunControllerManager already
// established for kube-controller-manager.
func RunScheduler(ctx context.Context, restCfg *restclient.Config) (err error) {
	defer func() {
		if r := recover(); r != nil {
			// Print the stack too: a resident dynamic worker has no other
			// way to surface WHERE a startup panic happened (found the
			// hard way during the schedwidth bring-up, 2026-07-10).
			println(string(debug.Stack()))
			err = fmt.Errorf("scheduler: panic: %v", r)
		}
	}()

	// DRAExtendedResource (Beta, default-on in v1.36) must be off in this
	// build: the js scheduler runs with a nil SharedDRAManager (the DRA
	// plugin/manager is severed from the js mirror to fit the 64MiB
	// Loader cap -- see gen-k8s-js-mirror.ts's DRA/CEL severing comment),
	// and both eventhandlers.go's DeviceClass case
	// (draManager.DeviceClassResolver(), nil-panicked live 2026-07-10)
	// and noderesources' extended-resource scoring path
	// (draManager.ResourceClaims().GatherAllocatedState()) dereference
	// the manager whenever this gate is on. Same effect as the
	// --feature-gates=DRAExtendedResource=false flag the host binary
	// could take; DynamicResourceAllocation itself is GA/locked and
	// stays on, which is fine -- its remaining event handlers only touch
	// the (kept, apimachinery-only) claim cache and slice tracker.
	if err := utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
		string(features.DRAExtendedResource): false,
	}); err != nil {
		return fmt.Errorf("scheduler: disable DRAExtendedResource: %w", err)
	}

	client, err := leanclientset.NewSchedulerClientsetForConfig(restclient.AddUserAgent(restCfg, "kube-scheduler"))
	if err != nil {
		return fmt.Errorf("scheduler: build client: %w", err)
	}

	factory := scheduler.NewInformerFactory(client, 0)

	if cloudflare.PumpTraceEnabled() {
		pumptrace.Observations(factory.Core().V1().Pods().Informer(), "sched")
	}

	cfg, err := latest.Default()
	if err != nil {
		return fmt.Errorf("scheduler: default config: %w", err)
	}
	// Matches cmd/scheduler/main.go's writeSchedulerConfig: disable the
	// PV/PVC/StorageClass-touching plugins this apiserver can't back.
	// latest.Default() already allocates Profiles[0].Plugins non-nil with
	// these four present (and enabled) in MultiPoint, verified by probing
	// its return value directly rather than assumed.
	// STRIP the unwanted default plugins from MultiPoint.Enabled --
	// setting MultiPoint.Disabled does NOT work for any of these
	// (confirmed live 2026-07-10, twice):
	//   - DynamicResources: the js registry overlay drops it from the
	//     in-tree REGISTRY, and framework construction errors on any
	//     MultiPoint.Enabled entry missing from the registry BEFORE it
	//     consults any Disabled list (expandMultiPointPlugins's
	//     '%s %q does not exist').
	//   - VolumeBinding & friends: with only Disabled set, the final
	//     framework plugin dump still showed all four ENABLED at every
	//     extension point, and VolumeBinding's event registrations then
	//     started VolumeAttachment/CSIStorageCapacity informers -- two
	//     resources this apiserver does not even serve (not in
	//     apidef.Table), whose watches therefore never receive the
	//     initial-events-end bookmark, permanently deadlocking
	//     factory.WaitForCacheSync.
	// Same four Volume plugins the host binary disables via its config
	// file (cmd/scheduler's writeSchedulerConfig), same reason: this
	// apiserver has no volume binding to back. DRA is unusable against
	// this apiserver anyway (ResourceClaim/ResourceSlice/DeviceClass are
	// permanently-empty stubs).
	strip := map[string]bool{
		"DynamicResources":   true,
		"VolumeBinding":      true,
		"VolumeRestrictions": true,
		"NodeVolumeLimits":   true,
		"VolumeZone":         true,
	}
	enabled := cfg.Profiles[0].Plugins.MultiPoint.Enabled[:0]
	for _, p := range cfg.Profiles[0].Plugins.MultiPoint.Enabled {
		if !strip[p.Name] {
			enabled = append(enabled, p)
		}
	}
	cfg.Profiles[0].Plugins.MultiPoint.Enabled = enabled

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

	// Starts the queued-event -> API-server write pipeline (mirrors
	// cmd/kube-scheduler/app/server.go's Run, which calls this on
	// cc.EventBroadcaster before its own sched.Run). Recorders built
	// above via recorderAdapter.NewRecorder only enqueue into this
	// adapter's internal watch.Broadcaster; without this call nothing
	// ever drains that queue, so every Scheduled/FailedScheduling event
	// is silently dropped -- confirmed live 2026-07-11 (wrangler dev: a
	// Pod bound successfully but neither /api/v1/events nor
	// /apis/events.k8s.io/v1/... ever recorded it, no error logged).
	recorderAdapter.StartRecordingToSink(ctx.Done())
	defer recorderAdapter.Shutdown()

	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())

	sched.Run(ctx) // blocks until ctx is canceled, same as Setup's own callers
	return ctx.Err()
}
