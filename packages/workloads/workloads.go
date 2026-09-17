package workloads

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/controller/deployment"
	"k8s.io/kubernetes/pkg/controller/replicaset"
	"k8s.io/kubernetes/pkg/controller/replication"
)

const (
	workers   = 5
	listPage  = 500
	drainPoll = 200 * time.Millisecond
	maxDrain  = 60 * time.Second
)

func init() {
	if err := utilfeature.DefaultMutableFeatureGate.Set("StaleControllerConsistencyReplicaSet=false"); err != nil {
		panic(err)
	}
}

type Result struct {
	Pods        int  `json:"pods"`
	ReplicaSets int  `json:"replicaSets"`
	Deployments int  `json:"deployments"`
	RCs         int  `json:"replicationControllers"`
	Drained     bool `json:"drained"`
}

type snapshot struct {
	informer *snapshotInformer
	objs     []runtime.Object
}

func Sync(ctx context.Context, client kubernetes.Interface) (*Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	pods, err := list(ctx, func(o metav1.ListOptions) ([]runtime.Object, string, error) {
		l, err := client.CoreV1().Pods("").List(ctx, o)
		if err != nil {
			return nil, "", err
		}
		out := make([]runtime.Object, len(l.Items))
		for i := range l.Items {
			out[i] = &l.Items[i]
		}
		return out, l.Continue, nil
	})
	if err != nil {
		return nil, err
	}
	rss, err := list(ctx, func(o metav1.ListOptions) ([]runtime.Object, string, error) {
		l, err := client.AppsV1().ReplicaSets("").List(ctx, o)
		if err != nil {
			return nil, "", err
		}
		out := make([]runtime.Object, len(l.Items))
		for i := range l.Items {
			out[i] = &l.Items[i]
		}
		return out, l.Continue, nil
	})
	if err != nil {
		return nil, err
	}
	deploys, err := list(ctx, func(o metav1.ListOptions) ([]runtime.Object, string, error) {
		l, err := client.AppsV1().Deployments("").List(ctx, o)
		if err != nil {
			return nil, "", err
		}
		out := make([]runtime.Object, len(l.Items))
		for i := range l.Items {
			out[i] = &l.Items[i]
		}
		return out, l.Continue, nil
	})
	if err != nil {
		return nil, err
	}

	rcs, err := list(ctx, func(o metav1.ListOptions) ([]runtime.Object, string, error) {
		l, err := client.CoreV1().ReplicationControllers("").List(ctx, o)
		if err != nil {
			return nil, "", err
		}
		out := make([]runtime.Object, len(l.Items))
		for i := range l.Items {
			out[i] = &l.Items[i]
		}
		return out, l.Continue, nil
	})
	if err != nil {
		return nil, err
	}

	factory := informers.NewSharedInformerFactory(client, 0)
	snaps := []snapshot{
		{register(factory, &v1.Pod{}), pods},
		{register(factory, &appsv1.ReplicaSet{}), rss},
		{register(factory, &appsv1.Deployment{}), deploys},
		{register(factory, &v1.ReplicationController{}), rcs},
	}
	apps, core := factory.Apps().V1(), factory.Core().V1()
	rs := replicaset.NewReplicaSetController(ctx, apps.ReplicaSets(), core.Pods(), client, replicaset.BurstReplicas)
	rc := replication.NewReplicationManager(ctx, core.Pods(), core.ReplicationControllers(), client, replication.BurstReplicas)
	dc, err := deployment.NewDeploymentController(ctx, apps.Deployments(), apps.ReplicaSets(), core.Pods(), client)
	if err != nil {
		return nil, err
	}
	for _, s := range snaps {
		s.informer.fill(s.objs)
	}
	for _, s := range snaps {
		s.informer.replay(s.objs)
	}
	done := make(chan struct{}, 3)
	go func() { rc.Run(ctx, workers); done <- struct{}{} }()
	go func() { rs.Run(ctx, workers); done <- struct{}{} }()
	go func() { dc.Run(ctx, workers); done <- struct{}{} }()

	result := &Result{Pods: len(pods), ReplicaSets: len(rss), Deployments: len(deploys), RCs: len(rcs)}
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
	<-done
	<-done
	<-done
	return result, nil
}

func register(factory informers.SharedInformerFactory, example runtime.Object) *snapshotInformer {
	s := newSnapshotInformer(example)
	factory.InformerFor(example, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer { return s })
	return s
}

func list(ctx context.Context, page func(metav1.ListOptions) ([]runtime.Object, string, error)) ([]runtime.Object, error) {
	var out []runtime.Object
	opts := metav1.ListOptions{Limit: listPage}
	for {
		objs, next, err := page(opts)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
		if next == "" {
			return out, nil
		}
		opts.Continue = next
	}
}
