package workloads

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/controller/certificates/rootcacertpublisher"
	"k8s.io/kubernetes/pkg/controller/cronjob"
	"k8s.io/kubernetes/pkg/controller/daemon"
	"k8s.io/kubernetes/pkg/controller/deployment"
	"k8s.io/kubernetes/pkg/controller/endpoint"
	"k8s.io/kubernetes/pkg/controller/endpointslice"
	"k8s.io/kubernetes/pkg/controller/job"
	"k8s.io/kubernetes/pkg/controller/replicaset"
	"k8s.io/kubernetes/pkg/controller/replication"
	"k8s.io/kubernetes/pkg/controller/serviceaccount"
	"k8s.io/kubernetes/pkg/controller/statefulset"
)

const (
	workers              = 5
	listPage             = 500
	drainPoll            = 200 * time.Millisecond
	maxDrain             = 60 * time.Second
	maxEndpointsPerSlice = 100
	daemonSetWorkers     = 2
	unfinishedJobRecheck = 10 * time.Second
	maxDelay             = 24 * time.Hour
)

func init() {
	for _, gate := range []string{"ReplicaSet", "Job", "StatefulSet", "DaemonSet"} {
		if err := utilfeature.DefaultMutableFeatureGate.Set("StaleControllerConsistency" + gate + "=false"); err != nil {
			panic(err)
		}
	}
}

type Result struct {
	Objects map[string]int `json:"objects"`
	Drained bool           `json:"drained"`
	NextMs  int64          `json:"nextMs"`
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
		{"jobs", &batchv1.Job{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.BatchV1().Jobs("").List(ctx, o)
		}},
		{"cronjobs", &batchv1.CronJob{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.BatchV1().CronJobs("").List(ctx, o)
		}},
		{"statefulsets", &appsv1.StatefulSet{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return apps.StatefulSets("").List(ctx, o)
		}},
		{"daemonsets", &appsv1.DaemonSet{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return apps.DaemonSets("").List(ctx, o)
		}},
		{"controllerrevisions", &appsv1.ControllerRevision{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return apps.ControllerRevisions("").List(ctx, o)
		}},
		{"persistentvolumeclaims", &v1.PersistentVolumeClaim{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.PersistentVolumeClaims("").List(ctx, o)
		}},
		{"namespaces", &v1.Namespace{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Namespaces().List(ctx, o)
		}},
		{"serviceaccounts", &v1.ServiceAccount{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.ServiceAccounts("").List(ctx, o)
		}},
		{"configmaps", &v1.ConfigMap{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.ConfigMaps("").List(ctx, o)
		}},
		{"nodes", &v1.Node{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Nodes().List(ctx, o)
		}},
	}
}

func Sync(ctx context.Context, client kubernetes.Interface, rootCA []byte) (*Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	work.reset(workloadQueue)
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
		if s.name == "jobs" && anyUnfinished(objs) {
			result.NextMs = unfinishedJobRecheck.Milliseconds()
		}
		if s.name == "cronjobs" {
			if next, ok := nextCronRun(objs); ok {
				result.NextMs = soonest(result.NextMs, next)
			}
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

	ds, err := daemon.NewDaemonSetsController(ctx, apps.DaemonSets(), apps.ControllerRevisions(), core.Pods(), core.Nodes(), client, flowcontrol.NewBackOff(time.Second, 15*time.Minute))
	if err != nil {
		return nil, err
	}
	runs = append(runs, func(ctx context.Context) { ds.Run(ctx, daemonSetWorkers) })
	ss := statefulset.NewStatefulSetController(ctx, core.Pods(), apps.StatefulSets(), core.PersistentVolumeClaims(), apps.ControllerRevisions(), client)
	runs = append(runs, func(ctx context.Context) { ss.Run(ctx, workers) })
	jobs, err := job.NewController(ctx, client, core.Pods(), factory.Batch().V1().Jobs(), nil, nil)
	if err != nil {
		return nil, err
	}
	runs = append(runs, func(ctx context.Context) { jobs.Run(ctx, workers) })
	cron, err := cronjob.NewControllerV2(ctx, factory.Batch().V1().Jobs(), factory.Batch().V1().CronJobs(), client)
	if err != nil {
		return nil, err
	}
	runs = append(runs, func(ctx context.Context) { cron.Run(ctx, workers) })

	accounts, err := serviceaccount.NewServiceAccountsController(klog.FromContext(ctx), core.ServiceAccounts(), core.Namespaces(), client, serviceaccount.DefaultServiceAccountsControllerOptions())
	if err != nil {
		return nil, err
	}
	runs = append(runs, func(ctx context.Context) { accounts.Run(ctx, 1) })
	publisher, err := rootcacertpublisher.NewPublisher(core.ConfigMaps(), core.Namespaces(), client, rootCA)
	if err != nil {
		return nil, err
	}
	runs = append(runs, func(ctx context.Context) { publisher.Run(ctx, 1) })

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

	result.Drained = drain(workloadQueue)
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

func anyUnfinished(objs []runtime.Object) bool {
	for _, o := range objs {
		j := o.(*batchv1.Job)
		finished := false
		for _, c := range j.Status.Conditions {
			if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == v1.ConditionTrue {
				finished = true
			}
		}
		if !finished && j.DeletionTimestamp == nil {
			return true
		}
	}
	return false
}

func drain(owned func(string) bool) bool {
	deadline := time.Now().Add(maxDrain)
	quiet := 0
	for time.Now().Before(deadline) {
		time.Sleep(drainPoll)
		if work.idle(owned) {
			quiet++
		} else {
			quiet = 0
		}
		if quiet >= 2 {
			return true
		}
	}
	return false
}

func nextCronRun(objs []runtime.Object) (time.Duration, bool) {
	var soonestRun time.Duration
	found := false
	now := time.Now()
	for _, o := range objs {
		cj := o.(*batchv1.CronJob)
		if cj.DeletionTimestamp != nil || (cj.Spec.Suspend != nil && *cj.Spec.Suspend) {
			continue
		}
		next, ok := nextSchedule(cj, now)
		if !ok {
			continue
		}
		wait := time.Until(next)
		if wait < time.Second {
			wait = time.Second
		}
		if wait > maxDelay {
			wait = maxDelay
		}
		if !found || wait < soonestRun {
			soonestRun = wait
			found = true
		}
	}
	return soonestRun, found
}

func soonest(currentMs int64, next time.Duration) int64 {
	nextMs := next.Milliseconds()
	if currentMs <= 0 || nextMs < currentMs {
		return nextMs
	}
	return currentMs
}

func nextSchedule(cj *batchv1.CronJob, now time.Time) (time.Time, bool) {
	schedule := cj.Spec.Schedule
	if cj.Spec.TimeZone != nil && *cj.Spec.TimeZone != "" {
		schedule = "TZ=" + *cj.Spec.TimeZone + " " + schedule
	}
	parsed, err := cron.ParseStandard(schedule)
	if err != nil {
		return time.Time{}, false
	}
	next := parsed.Next(now)
	if next.IsZero() {
		return time.Time{}, false
	}
	return next, true
}
