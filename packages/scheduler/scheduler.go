package scheduler

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	v1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/reference"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler"
	schedconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/latest"
)

const (
	listPage          = 500
	inFlightWait      = 20 * time.Second
	bindGrace         = 70 * time.Second
	inFlightPoll      = 50 * time.Millisecond
	resourceCachePoll = 10 * time.Millisecond
	resourceCacheWait = 10 * time.Second
	schedulerName     = "default-scheduler"
	reportingActor    = "default-scheduler"
)

func init() {
	_ = utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
		string(features.SchedulerAsyncPreemption): false,
	})
}

type PodRef struct {
	Namespace string `json:"ns"`
	Name      string `json:"name"`
	UID       string `json:"uid"`
}

type QueueMessage struct {
	Kind    string `json:"kind"`
	Attempt *int   `json:"attempt,omitempty"`
}

type Result struct {
	Bound         int      `json:"bound"`
	Unschedulable []PodRef `json:"unschedulable"`
	RetryAfterS   int      `json:"retryAfterS,omitempty"`
	Attempt       int      `json:"attempt"`
}

func QueueAttempt(msgs []QueueMessage) (int, bool) {
	attempt := -1
	hasChange := false
	for _, msg := range msgs {
		switch msg.Kind {
		case "change":
			hasChange = true
			if attempt < 0 {
				attempt = 0
			}
		case "retry":
			n := 0
			if msg.Attempt != nil {
				n = *msg.Attempt
			}
			if n+1 > attempt {
				attempt = n + 1
			}
		}
	}
	if attempt < 0 {
		return 0, false
	}
	if hasChange {
		return 0, true
	}
	return attempt, true
}

func RetryDelaySeconds(attempt, unschedulable int) int {
	if unschedulable == 0 {
		return 0
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt >= 6 {
		return 60
	}
	delay := 1 << attempt
	if delay > 60 {
		return 60
	}
	return delay
}

func Schedule(ctx context.Context, client kubernetes.Interface) (*Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	config, err := latest.Default()
	if err != nil {
		return nil, err
	}
	for _, pc := range config.Profiles[0].PluginConfig {
		if args, ok := pc.Args.(*schedconfig.VolumeBindingArgs); ok {
			args.BindTimeoutSeconds = 0
		}
	}
	factory := scheduler.NewInformerFactory(client, 0)
	if err := startResourceInformers(ctx, client, factory); err != nil {
		return nil, err
	}
	var scheduled atomic.Int64
	sched, err := scheduler.New(ctx, client, factory, nil, syncRecorderFactory(client, &scheduled),
		scheduler.WithComponentConfigVersion(config.TypeMeta.APIVersion),
		scheduler.WithProfiles(config.Profiles...),
		scheduler.WithPercentageOfNodesToScore(config.PercentageOfNodesToScore),
		scheduler.WithPodMaxBackoffSeconds(config.PodMaxBackoffSeconds),
		scheduler.WithPodInitialBackoffSeconds(config.PodInitialBackoffSeconds),
		scheduler.WithParallelism(config.Parallelism),
	)
	if err != nil {
		return nil, err
	}
	if err := waitForResourceCaches(ctx, sched, factory); err != nil {
		return nil, err
	}
	logger := klog.FromContext(ctx)
	if sched.APIDispatcher != nil {
		sched.APIDispatcher.Run(logger)
		defer sched.APIDispatcher.Close()
	}
	defer sched.SchedulingQueue.Close()

	nodes, err := listNodes(ctx, client)
	if err != nil {
		return nil, err
	}
	pods, err := listPods(ctx, client)
	if err != nil {
		return nil, err
	}
	if err := fillSupporting(ctx, client, factory); err != nil {
		return nil, err
	}
	nodeIndexer := factory.Core().V1().Nodes().Informer().GetIndexer()
	for i := range nodes {
		nodeIndexer.Add(&nodes[i])
		sched.Cache.AddNode(logger, &nodes[i])
	}
	podIndexer := factory.Core().V1().Pods().Informer().GetIndexer()
	var queued []*v1.Pod
	for i := range pods {
		pod := &pods[i]
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}
		podIndexer.Add(pod)
		switch {
		case pod.Spec.NodeName != "":
			sched.Cache.AddPod(logger, pod)
		case pod.DeletionTimestamp == nil && pod.Spec.SchedulerName == schedulerName:
			queued = append(queued, pod)
		}
	}
	for _, pod := range queued {
		sched.SchedulingQueue.Add(ctx, pod)
	}

	for len(sched.SchedulingQueue.PodsInActiveQ()) > 0 {
		sched.ScheduleOne(ctx)
	}
	deadline := time.Now().Add(inFlightWait)
	finishBy := deadline.Add(bindGrace)
	for {
		pending, _ := sched.SchedulingQueue.PendingPods()
		inFlight := len(sched.SchedulingQueue.InFlightPods())
		if inFlight == 0 && int(scheduled.Load())+len(pending) >= len(queued) {
			break
		}
		now := time.Now()
		if now.After(finishBy) || (now.After(deadline) && inFlight == 0) {
			break
		}
		time.Sleep(inFlightPoll)
	}

	result := &Result{Unschedulable: []PodRef{}}
	waiting := map[string]bool{}
	pending, _ := sched.SchedulingQueue.PendingPods()
	for _, p := range pending {
		waiting[string(p.UID)] = true
		result.Unschedulable = append(result.Unschedulable, PodRef{Namespace: p.Namespace, Name: p.Name, UID: string(p.UID)})
	}
	for _, p := range sched.SchedulingQueue.InFlightPods() {
		if !waiting[string(p.UID)] {
			waiting[string(p.UID)] = true
			result.Unschedulable = append(result.Unschedulable, PodRef{Namespace: p.Namespace, Name: p.Name, UID: string(p.UID)})
		}
	}
	result.Bound = int(scheduled.Load())
	result.Unschedulable = keepUnbound(queued, result.Bound, result.Unschedulable)
	return result, nil
}

func keepUnbound(queued []*v1.Pod, bound int, found []PodRef) []PodRef {
	if bound >= len(queued) || len(found) > 0 {
		return found
	}
	out := make([]PodRef, 0, len(queued))
	for _, p := range queued {
		out = append(out, PodRef{Namespace: p.Namespace, Name: p.Name, UID: string(p.UID)})
	}
	return out
}

func listNodes(ctx context.Context, client kubernetes.Interface) ([]v1.Node, error) {
	var out []v1.Node
	opts := metav1.ListOptions{Limit: listPage}
	for {
		list, err := client.CoreV1().Nodes().List(ctx, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, list.Items...)
		if list.Continue == "" {
			return out, nil
		}
		opts.Continue = list.Continue
	}
}

func listPods(ctx context.Context, client kubernetes.Interface) ([]v1.Pod, error) {
	var out []v1.Pod
	opts := metav1.ListOptions{Limit: listPage}
	for {
		list, err := client.CoreV1().Pods("").List(ctx, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, list.Items...)
		if list.Continue == "" {
			return out, nil
		}
		opts.Continue = list.Continue
	}
}

func fillSupporting(ctx context.Context, client kubernetes.Interface, factory informers.SharedInformerFactory) error {
	fill := func(informer cache.SharedIndexInformer, list func() ([]runtime.Object, error)) error {
		objs, err := list()
		if err != nil {
			return err
		}
		for _, o := range objs {
			informer.GetIndexer().Add(o)
		}
		return nil
	}
	all := metav1.ListOptions{}
	if err := fill(factory.Core().V1().Namespaces().Informer(), func() ([]runtime.Object, error) {
		l, err := client.CoreV1().Namespaces().List(ctx, all)
		if err != nil {
			return nil, err
		}
		out := make([]runtime.Object, 0, len(l.Items))
		for i := range l.Items {
			out = append(out, &l.Items[i])
		}
		return out, nil
	}); err != nil {
		return err
	}
	if err := fill(factory.Core().V1().Services().Informer(), func() ([]runtime.Object, error) {
		l, err := client.CoreV1().Services("").List(ctx, all)
		if err != nil {
			return nil, err
		}
		out := make([]runtime.Object, 0, len(l.Items))
		for i := range l.Items {
			out = append(out, &l.Items[i])
		}
		return out, nil
	}); err != nil {
		return err
	}
	if err := fill(factory.Policy().V1().PodDisruptionBudgets().Informer(), func() ([]runtime.Object, error) {
		l, err := client.PolicyV1().PodDisruptionBudgets("").List(ctx, all)
		if err != nil {
			return nil, err
		}
		out := make([]runtime.Object, 0, len(l.Items))
		for i := range l.Items {
			out = append(out, &l.Items[i])
		}
		return out, nil
	}); err != nil {
		return err
	}
	return fillVolumes(ctx, client, factory)
}

func startResourceInformers(ctx context.Context, client kubernetes.Interface, factory informers.SharedInformerFactory) error {
	resources := client.ResourceV1()
	sources := map[runtime.Object]cache.ListerWatcher{
		&resourceapi.ResourceClaim{}: listOnly(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return resources.ResourceClaims("").List(ctx, opts)
		}),
		&resourceapi.ResourceSlice{}: listOnly(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return resources.ResourceSlices().List(ctx, opts)
		}),
		&resourceapi.DeviceClass{}: listOnly(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return resources.DeviceClasses().List(ctx, opts)
		}),
	}
	for obj, source := range sources {
		informer := factory.InformerFor(obj, func(_ kubernetes.Interface, _ time.Duration) cache.SharedIndexInformer {
			return cache.NewSharedIndexInformer(source, obj, 0, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
		})
		go informer.Run(ctx.Done())
		if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
			return ctx.Err()
		}
	}
	return nil
}

func waitForResourceCaches(ctx context.Context, sched *scheduler.Scheduler, factory informers.SharedInformerFactory) error {
	drm := sched.Profiles[schedulerName].SharedDRAManager()
	if drm == nil {
		return nil
	}
	claims := factory.Resource().V1().ResourceClaims().Informer().GetStore()
	slices := factory.Resource().V1().ResourceSlices().Informer().GetStore()
	return wait.PollUntilContextTimeout(ctx, resourceCachePoll, resourceCacheWait, true, func(context.Context) (bool, error) {
		assumedClaims, err := drm.ResourceClaims().List()
		if err != nil {
			return false, err
		}
		trackedSlices, err := drm.ResourceSlices().ListWithDeviceTaintRules()
		if err != nil {
			return false, err
		}
		return len(assumedClaims) == len(claims.List()) && len(trackedSlices) == len(slices.List()), nil
	})
}

type listOnlyWatcher struct {
	*cache.ListWatch
}

func (listOnlyWatcher) IsWatchListSemanticsUnSupported() bool { return true }

func listOnly(list func(context.Context, metav1.ListOptions) (runtime.Object, error)) cache.ListerWatcher {
	return listOnlyWatcher{&cache.ListWatch{
		ListWithContextFunc: list,
		WatchFuncWithContext: func(context.Context, metav1.ListOptions) (watch.Interface, error) {
			return watch.NewFake(), nil
		},
	}}
}

func fillVolumes(ctx context.Context, client kubernetes.Interface, factory informers.SharedInformerFactory) error {
	all := metav1.ListOptions{}
	storage := client.StorageV1()
	lists := []struct {
		informer cache.SharedIndexInformer
		list     func() (runtime.Object, error)
	}{
		{factory.Core().V1().PersistentVolumes().Informer(), func() (runtime.Object, error) { return client.CoreV1().PersistentVolumes().List(ctx, all) }},
		{factory.Core().V1().PersistentVolumeClaims().Informer(), func() (runtime.Object, error) { return client.CoreV1().PersistentVolumeClaims("").List(ctx, all) }},
		{factory.Storage().V1().StorageClasses().Informer(), func() (runtime.Object, error) { return storage.StorageClasses().List(ctx, all) }},
		{factory.Storage().V1().CSINodes().Informer(), func() (runtime.Object, error) { return storage.CSINodes().List(ctx, all) }},
		{factory.Storage().V1().CSIDrivers().Informer(), func() (runtime.Object, error) { return storage.CSIDrivers().List(ctx, all) }},
		{factory.Storage().V1().CSIStorageCapacities().Informer(), func() (runtime.Object, error) { return storage.CSIStorageCapacities("").List(ctx, all) }},
		{factory.Storage().V1().VolumeAttachments().Informer(), func() (runtime.Object, error) { return storage.VolumeAttachments().List(ctx, all) }},
	}
	for _, l := range lists {
		list, err := l.list()
		if err != nil {
			return err
		}
		objs, err := meta.ExtractList(list)
		if err != nil {
			return err
		}
		for _, o := range objs {
			l.informer.GetIndexer().Add(o)
		}
	}
	return nil
}

type syncRecorder struct {
	client    kubernetes.Interface
	instance  string
	scheduled *atomic.Int64
}

func syncRecorderFactory(client kubernetes.Interface, scheduled *atomic.Int64) func(string) events.EventRecorderLogger {
	host, _ := os.Hostname()
	return func(string) events.EventRecorderLogger {
		return &syncRecorder{client: client, instance: reportingActor + "-" + host, scheduled: scheduled}
	}
}

func (r *syncRecorder) WithLogger(klog.Logger) events.EventRecorderLogger { return r }

func (r *syncRecorder) Eventf(regarding runtime.Object, related runtime.Object, eventtype, reason, action, note string, args ...interface{}) {
	if reason == "Scheduled" {
		r.scheduled.Add(1)
	}
	ref, err := reference.GetReference(scheme.Scheme, regarding)
	if err != nil {
		return
	}
	now := time.Now()
	ns := ref.Namespace
	if ns == "" {
		ns = metav1.NamespaceDefault
	}
	stamp := metav1.Time{Time: now}
	event := &v1.Event{
		ObjectMeta:          metav1.ObjectMeta{Name: fmt.Sprintf("%v.%x", ref.Name, now.UnixNano()), Namespace: ns},
		InvolvedObject:      *ref,
		Reason:              reason,
		Message:             fmt.Sprintf(note, args...),
		Source:              v1.EventSource{Component: reportingActor},
		FirstTimestamp:      stamp,
		LastTimestamp:       stamp,
		Count:               1,
		Type:                eventtype,
		Action:              action,
		ReportingController: reportingActor,
		ReportingInstance:   r.instance,
	}
	if related != nil {
		if rel, err := reference.GetReference(scheme.Scheme, related); err == nil {
			event.Related = rel
		}
	}
	if _, err := r.client.CoreV1().Events(ns).Create(context.Background(), event, metav1.CreateOptions{}); err != nil {
		println("scheduler: event create failed:", err.Error())
	}
}
