package workloads

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/controller/deployment"
	"k8s.io/kubernetes/pkg/controller/endpoint"
	"k8s.io/kubernetes/pkg/controller/endpointslice"
	"k8s.io/kubernetes/pkg/controller/replicaset"
	"k8s.io/kubernetes/pkg/controller/replication"
)

const (
	workers              = 5
	listPage             = 500
	drainPoll            = 200 * time.Millisecond
	maxDrain             = 60 * time.Second
	maxEndpointsPerSlice = 100
)

func init() {
	if err := utilfeature.DefaultMutableFeatureGate.Set("StaleControllerConsistencyReplicaSet=false"); err != nil {
		panic(err)
	}
}

type Result struct {
	Objects map[string]int `json:"objects"`
	Drained bool           `json:"drained"`
}

type pageFunc func(context.Context, metav1.ListOptions) (runtime.Object, error)

type source struct {
	name    string
	example runtime.Object
	page    pageFunc
}

func sources(client kubernetes.Interface) []source {
	core, apps := client.CoreV1(), client.AppsV1()
	return []source{
		{"pods", &v1.Pod{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Pods("").List(ctx, o)
		}},
		{"replicasets", &appsv1.ReplicaSet{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return apps.ReplicaSets("").List(ctx, o)
		}},
		{"deployments", &appsv1.Deployment{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return apps.Deployments("").List(ctx, o)
		}},
		{"replicationcontrollers", &v1.ReplicationController{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.ReplicationControllers("").List(ctx, o)
		}},
		{"services", &v1.Service{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Services("").List(ctx, o)
		}},
		{"endpoints", &v1.Endpoints{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Endpoints("").List(ctx, o)
		}},
		{"endpointslices", &discoveryv1.EndpointSlice{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.DiscoveryV1().EndpointSlices("").List(ctx, o)
		}},
		{"nodes", &v1.Node{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Nodes().List(ctx, o)
		}},
	}
}

func Sync(ctx context.Context, client kubernetes.Interface) (*Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	factory := informers.NewSharedInformerFactory(client, 0)
	result := &Result{Objects: map[string]int{}}
	type loaded struct {
		informer *snapshotInformer
		objs     []runtime.Object
	}
	var all []loaded
	for _, s := range sources(client) {
		objs, err := list(ctx, s.page)
		if err != nil {
			return nil, err
		}
		result.Objects[s.name] = len(objs)
		all = append(all, loaded{register(factory, s.example), objs})
	}

	apps, core := factory.Apps().V1(), factory.Core().V1()
	runs := []func(context.Context){}
	rs := replicaset.NewReplicaSetController(ctx, apps.ReplicaSets(), core.Pods(), client, replicaset.BurstReplicas)
	runs = append(runs, func(ctx context.Context) { rs.Run(ctx, workers) })
	rc := replication.NewReplicationManager(ctx, core.Pods(), core.ReplicationControllers(), client, replication.BurstReplicas)
	runs = append(runs, func(ctx context.Context) { rc.Run(ctx, workers) })
	dc, err := deployment.NewDeploymentController(ctx, apps.Deployments(), apps.ReplicaSets(), core.Pods(), client)
	if err != nil {
		return nil, err
	}
	runs = append(runs, func(ctx context.Context) { dc.Run(ctx, workers) })
	ep := endpoint.NewEndpointController(ctx, core.Pods(), core.Services(), core.Endpoints(), client, 0)
	runs = append(runs, func(ctx context.Context) { ep.Run(ctx, workers) })
	eps := endpointslice.NewController(ctx, core.Pods(), core.Services(), core.Nodes(), factory.Discovery().V1().EndpointSlices(), maxEndpointsPerSlice, client, 0)
	runs = append(runs, func(ctx context.Context) { eps.Run(ctx, workers) })

	for _, l := range all {
		l.informer.fill(l.objs)
	}
	for _, l := range all {
		l.informer.replay(l.objs)
	}
	done := make(chan struct{}, len(runs))
	for _, run := range runs {
		go func() { run(ctx); done <- struct{}{} }()
	}

	deadline := time.Now().Add(maxDrain)
	quiet := 0
	for time.Now().Before(deadline) {
		time.Sleep(drainPoll)
		if work.idle() {
			quiet++
		} else {
			quiet = 0
		}
		if quiet >= 2 {
			result.Drained = true
			break
		}
	}
	cancel()
	for range runs {
		<-done
	}
	return result, nil
}

func register(factory informers.SharedInformerFactory, example runtime.Object) *snapshotInformer {
	s := newSnapshotInformer(example)
	factory.InformerFor(example, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer { return s })
	return s
}

func list(ctx context.Context, page pageFunc) ([]runtime.Object, error) {
	var out []runtime.Object
	opts := metav1.ListOptions{Limit: listPage}
	for {
		l, err := page(ctx, opts)
		if err != nil {
			return nil, err
		}
		items, err := meta.ExtractList(l)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		lm, err := meta.ListAccessor(l)
		if err != nil {
			return nil, err
		}
		if lm.GetContinue() == "" {
			return out, nil
		}
		opts.Continue = lm.GetContinue()
	}
}
