//go:build js && wasm

package controllers

import (
	"context"
	"fmt"
	"net"
	"time"

	restclient "k8s.io/client-go/rest"

	"k8s.io/client-go/util/flowcontrol"

	leanclientset "github.com/k8flare/k8flare/pkg/leanclient/clientset"
	leaninformers "github.com/k8flare/k8flare/pkg/leanclient/informers"
	"k8s.io/kubernetes/pkg/controller/cronjob"
	"k8s.io/kubernetes/pkg/controller/daemon"
	"k8s.io/kubernetes/pkg/controller/deployment"
	"k8s.io/kubernetes/pkg/controller/endpoint"
	"k8s.io/kubernetes/pkg/controller/endpointslice"
	"k8s.io/kubernetes/pkg/controller/job"
	"k8s.io/kubernetes/pkg/controller/nodeipam"
	"k8s.io/kubernetes/pkg/controller/nodeipam/ipam"
	"k8s.io/kubernetes/pkg/controller/nodelifecycle"
	"k8s.io/kubernetes/pkg/controller/replicaset"
	"k8s.io/kubernetes/pkg/controller/replication"
	"k8s.io/kubernetes/pkg/controller/statefulset"
	"k8s.io/kubernetes/pkg/controller/tainteviction"
)

// clusterCIDR matches the /16 the deleted workers/storage/src/scheduler.ts
// (nodeipam's predecessor) allocated /24s from, and cmd/agent's k3s
// default flannel config -- keep these in sync if either changes.
const clusterCIDR = "10.42.0.0/16"

// Worker counts and periods below are hardcoded to kube-controller-manager
// v1.36.2's own --concurrent-*-syncs / node-lifecycle flag defaults
// (verified by running the real cmd/controller-manager binary with -v=2
// and reading its FLAG: log lines -- CLAUDE.md rule #2), not invented.
// This package bypasses kube-controller-manager's own CLI/options layer
// entirely (see the "why" in this file's sibling restconfig.go and the
// package-level doc comment there), so there is no --flag mechanism here
// to override them -- if that's ever needed, add real flags to
// workers/controllers, not a reimplementation of KubeControllerManagerOptions.
const (
	replicationControllerWorkers = 5 // matches RecommendedDefaultReplicationControllerConfiguration
	replicaSetWorkers            = 5
	deploymentWorkers            = 5
	daemonSetWorkers             = 2
	statefulSetWorkers           = 5 // matches RecommendedDefaultStatefulSetControllerConfiguration
	jobWorkers                   = 5
	cronJobWorkers               = 5
	endpointWorkers              = 5
	endpointSliceWorkers         = 5

	endpointUpdatesBatchPeriod = 0 // upstream default: no batching delay
	maxEndpointsPerSlice       = 100

	nodeCIDRMaskSize = 24 // /24 per node from clusterCIDR, matches deleted scheduler.ts

	nodeMonitorPeriod           = 5 * time.Second
	nodeStartupGracePeriod      = 60 * time.Second
	nodeMonitorGracePeriod      = 50 * time.Second
	evictionLimiterQPS          = 0.1
	secondaryEvictionLimiterQPS = 0.01
	largeClusterThreshold       = 50
	unhealthyZoneThreshold      = 0.55

	minResyncPeriod = 12 * time.Hour // matches --min-resync-period default
)

// RunControllerManager starts the real, unmodified upstream
// kube-controller-manager controllers this repo enables (see
// cmd/controller-manager/main.go's --controllers default, kept in sync by
// hand) against restCfg, and blocks until ctx is canceled.
//
// Unlike RunScheduler's sibling attempt (removed -- see this file's
// module-level doc comment in restconfig.go's neighbor and
// docs/platform-verification.md's S8 section for why kube-scheduler
// itself cannot compile for GOOS=js/wasm at all), this does NOT go
// through k8s.io/kubernetes/cmd/kube-controller-manager/app's
// NewControllerManagerCommand/Run/NewControllerDescriptors: that
// top-level package unconditionally references every controller's
// constructor (including PersistentVolumeBinder, AttachDetach,
// EphemeralVolume, CSRSigning, and others this repo never enables), and
// several of those transitively import k8s.io/mount-utils,
// k8s.io/kubernetes/pkg/probe, pkg/securitycontext, and
// pkg/util/filesystem -- all of which have Linux/Windows-only syscalls
// with no GOOS=js build variant anywhere in the dependency graph
// (confirmed by isolating each failure with `GOOS=js GOARCH=wasm go
// build`, not assumed).
//
// Individually, the specific controller packages this repo actually
// enables (pkg/controller/{replicaset,deployment,daemon,statefulset,job,
// cronjob,endpoint,endpointslice,nodeipam,nodelifecycle,tainteviction})
// compile cleanly for GOOS=js/wasm on their own -- verified the same way
// (statefulset: `GOOS=js GOARCH=wasm go build
// k8s.io/kubernetes/pkg/controller/statefulset/...` with zero output).
// So this function hand-wires exactly those eleven, using a plain
// k8s.io/client-go/informers.SharedInformerFactory (the same type
// ControllerContext.InformerFactory in the real app package is declared
// as) instead of the app package's own context/registry machinery.
func RunControllerManager(ctx context.Context, restCfg *restclient.Config) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("controller-manager: panic: %v", r)
		}
	}()

	client, err := leanclientset.NewForConfig(restclient.AddUserAgent(restCfg, "kube-controller-manager"))
	if err != nil {
		return fmt.Errorf("controller-manager: build client: %w", err)
	}

	factory := leaninformers.New(client, minResyncPeriod)

	rcc := replication.NewReplicationManager(
		ctx,
		factory.Pods(),
		factory.ReplicationControllers(),
		client,
		replication.BurstReplicas,
	)

	rsc := replicaset.NewReplicaSetController(
		ctx,
		factory.ReplicaSets(),
		factory.Pods(),
		client,
		replicaset.BurstReplicas,
	)

	dc, err := deployment.NewDeploymentController(
		ctx,
		factory.Deployments(),
		factory.ReplicaSets(),
		factory.Pods(),
		client,
	)
	if err != nil {
		return fmt.Errorf("controller-manager: new deployment controller: %w", err)
	}

	dsc, err := daemon.NewDaemonSetsController(
		ctx,
		factory.DaemonSets(),
		factory.ControllerRevisions(),
		factory.Pods(),
		factory.Nodes(),
		client,
		flowcontrol.NewBackOff(1*time.Second, 15*time.Minute),
	)
	if err != nil {
		return fmt.Errorf("controller-manager: new daemonset controller: %w", err)
	}

	// workloadInformer/podGroupInformer stay nil: only needed when the
	// alpha WorkloadWithJob feature gate is enabled, which it is not by
	// default (matches upstream's own newJobController, which only
	// populates them behind the same gate check).
	jc, err := job.NewController(
		ctx,
		client,
		factory.Pods(),
		factory.Jobs(),
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("controller-manager: new job controller: %w", err)
	}

	cjc, err := cronjob.NewControllerV2(
		ctx,
		factory.Jobs(),
		factory.CronJobs(),
		client,
	)
	if err != nil {
		return fmt.Errorf("controller-manager: new cronjob controller: %w", err)
	}

	// NewStatefulSetController takes no context/error return (unlike its
	// deployment/daemonset/job/cronjob siblings above) -- confirmed against
	// the real signature in k8s.io/kubernetes/pkg/controller/statefulset,
	// not assumed.
	ssc := statefulset.NewStatefulSetController(
		ctx,
		factory.Pods(),
		factory.StatefulSets(),
		factory.PersistentVolumeClaims(),
		factory.ControllerRevisions(),
		client,
	)

	ec := endpoint.NewEndpointController(
		ctx,
		factory.Pods(),
		factory.Services(),
		factory.Endpoints(),
		client,
		endpointUpdatesBatchPeriod,
	)

	esc := endpointslice.NewController(
		ctx,
		factory.Pods(),
		factory.Services(),
		factory.Nodes(),
		factory.EndpointSlices(),
		maxEndpointsPerSlice,
		client,
		endpointUpdatesBatchPeriod,
	)

	_, clusterCIDRNet, err := net.ParseCIDR(clusterCIDR)
	if err != nil {
		return fmt.Errorf("controller-manager: parse cluster CIDR %q: %w", clusterCIDR, err)
	}

	nic, err := nodeipam.NewNodeIpamController(
		ctx,
		factory.Nodes(),
		nil,
		client,
		[]*net.IPNet{clusterCIDRNet},
		nil,
		nil,
		[]int{nodeCIDRMaskSize},
		ipam.RangeAllocatorType,
	)
	if err != nil {
		return fmt.Errorf("controller-manager: new nodeipam controller: %w", err)
	}

	// Must be constructed BEFORE tainteviction.New below: this
	// constructor is what registers the "spec.nodeName" pod indexer on
	// the shared Pod informer, and tainteviction's own
	// getPodsAssignedToNode closure reads that index by name.
	nlc, err := nodelifecycle.NewNodeLifecycleController(
		ctx,
		factory.Leases(),
		factory.Pods(),
		factory.Nodes(),
		factory.DaemonSets(),
		client,
		nodeMonitorPeriod,
		nodeStartupGracePeriod,
		nodeMonitorGracePeriod,
		evictionLimiterQPS,
		secondaryEvictionLimiterQPS,
		largeClusterThreshold,
		unhealthyZoneThreshold,
	)
	if err != nil {
		return fmt.Errorf("controller-manager: new nodelifecycle controller: %w", err)
	}

	tec, err := tainteviction.New(ctx, client, factory.Pods(), factory.Nodes(), "taint-eviction-controller")
	if err != nil {
		return fmt.Errorf("controller-manager: new taint-eviction controller: %w", err)
	}

	factory.Start(ctx.Done())

	for _, run := range []func(context.Context){
		func(ctx context.Context) { rcc.Run(ctx, replicationControllerWorkers) },
		func(ctx context.Context) { rsc.Run(ctx, replicaSetWorkers) },
		func(ctx context.Context) { dc.Run(ctx, deploymentWorkers) },
		func(ctx context.Context) { dsc.Run(ctx, daemonSetWorkers) },
		func(ctx context.Context) { ssc.Run(ctx, statefulSetWorkers) },
		func(ctx context.Context) { jc.Run(ctx, jobWorkers) },
		func(ctx context.Context) { cjc.Run(ctx, cronJobWorkers) },
		func(ctx context.Context) { ec.Run(ctx, endpointWorkers) },
		func(ctx context.Context) { esc.Run(ctx, endpointSliceWorkers) },
		func(ctx context.Context) { nic.Run(ctx) },
		func(ctx context.Context) { nlc.Run(ctx) },
		func(ctx context.Context) { tec.Run(ctx) },
	} {
		go runRecovered(ctx, run)
	}

	<-ctx.Done()
	return ctx.Err()
}

// runRecovered wraps a single controller's Run loop with its own
// recover(): an unrecovered panic in any one goroutine kills the whole Go
// program, taking down every other controller sharing this WASM instance
// (S8 FINDINGS.md, "Other pitfalls worth remembering") -- one controller
// misbehaving should not be able to do that to the other five.
func runRecovered(ctx context.Context, run func(context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("controllers: panic in controller goroutine: %v\n", r)
		}
	}()
	run(ctx)
}
