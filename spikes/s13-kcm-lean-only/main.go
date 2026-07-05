//go:build js && wasm

// Command s13-kcm-lean-only is throwaway size-measurement scaffolding:
// it duplicates pkg/controllers/controllermanager.go's RunControllerManager
// body verbatim (10 real upstream controllers + pkg/leanclient) WITHOUT
// importing pkg/controllers itself, so pkg/controllers/scheduler.go's
// dependency on the real, unpruned k8s.io/client-go/informers aggregate
// factory (which pkg/scheduler.New's informerFactory parameter requires --
// see docs/platform-verification.md's Phase 10 honest-correction entry)
// never enters this binary's package graph. Answers: does the lean-client
// win (pkg/leanclient, avoiding runtime.Scheme/serializer.CodecFactory)
// survive for kube-controller-manager specifically, in a build that never
// combines it with kube-scheduler?
package main

import (
	"context"
	"fmt"
	"net"
	"runtime"
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
	"k8s.io/kubernetes/pkg/controller/tainteviction"
)

const clusterCIDR = "10.42.0.0/16"
const nodeCIDRMaskSize = 24
const minResyncPeriod = 12 * time.Hour

func run(ctx context.Context, restCfg *restclient.Config) error {
	client, err := leanclientset.NewForConfig(restCfg)
	if err != nil {
		return err
	}
	factory := leaninformers.New(client, minResyncPeriod)

	// NOTE(s14 Part 7 re-test): pkg/leanclient/gen's Events() was a
	// panic("not implemented") stub that crashed every one of these
	// controllers at Run()-time (record.EventBroadcaster calls it
	// immediately). Re-enabling all 10 controllers here now that
	// cmd/k8flare-gen/leanclient.go + regenerated pkg/leanclient/gen have a
	// real Events() implementation, to verify the fix end to end.
	rsc := replicaset.NewReplicaSetController(ctx, factory.ReplicaSets(), factory.Pods(), client, replicaset.BurstReplicas)
	dc, err := deployment.NewDeploymentController(ctx, factory.Deployments(), factory.ReplicaSets(), factory.Pods(), client)
	if err != nil {
		return err
	}
	dsc, err := daemon.NewDaemonSetsController(ctx, factory.DaemonSets(), factory.ControllerRevisions(), factory.Pods(), factory.Nodes(), client, flowcontrol.NewBackOff(time.Second, 15*time.Minute))
	if err != nil {
		return err
	}
	jc, err := job.NewController(ctx, client, factory.Pods(), factory.Jobs(), nil, nil)
	if err != nil {
		return err
	}
	cjc, err := cronjob.NewControllerV2(ctx, factory.Jobs(), factory.CronJobs(), client)
	if err != nil {
		return err
	}
	ec := endpoint.NewEndpointController(ctx, factory.Pods(), factory.Services(), factory.Endpoints(), client, 0)
	esc := endpointslice.NewController(ctx, factory.Pods(), factory.Services(), factory.Nodes(), factory.EndpointSlices(), 100, client, 0)
	_, cidr, err := net.ParseCIDR(clusterCIDR)
	if err != nil {
		return err
	}
	nic, err := nodeipam.NewNodeIpamController(ctx, factory.Nodes(), nil, client, []*net.IPNet{cidr}, nil, nil, []int{nodeCIDRMaskSize}, ipam.RangeAllocatorType)
	if err != nil {
		return err
	}
	nlc, err := nodelifecycle.NewNodeLifecycleController(ctx, factory.Leases(), factory.Pods(), factory.Nodes(), factory.DaemonSets(), client, 5*time.Second, 60*time.Second, 50*time.Second, 0.1, 0.01, 50, 0.55)
	if err != nil {
		return err
	}
	tec, err := tainteviction.New(ctx, client, factory.Pods(), factory.Nodes(), "taint-eviction-controller")
	if err != nil {
		return err
	}

	factory.Start(ctx.Done())
	go rsc.Run(ctx, 5)
	go dc.Run(ctx, 5)
	go dsc.Run(ctx, 2)
	go jc.Run(ctx, 5)
	go cjc.Run(ctx, 5)
	go ec.Run(ctx, 5)
	go esc.Run(ctx, 5)
	go nic.Run(ctx)
	go nlc.Run(ctx)
	go tec.Run(ctx)
	<-ctx.Done()
	return ctx.Err()
}

func logMemStats(ctx context.Context) {
	var m runtime.MemStats
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runtime.ReadMemStats(&m)
			fmt.Printf("MEMSTATS Alloc=%dKB Sys=%dKB HeapAlloc=%dKB HeapSys=%dKB NumGoroutine=%d\n",
				m.Alloc/1024, m.Sys/1024, m.HeapAlloc/1024, m.HeapSys/1024, runtime.NumGoroutine())
		}
	}
}

func main() {
	ctx := context.Background()
	go logMemStats(ctx)
	go run(ctx, &restclient.Config{
		Host:        "http://localhost:8861",
		BearerToken: "k8flare-dev-token",
	})
	select {}
}
