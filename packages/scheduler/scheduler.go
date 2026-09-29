package scheduler

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	"k8s.io/kubernetes/pkg/scheduler/apis/config/latest"
)

const (
	listPage       = 500
	inFlightWait   = 20 * time.Second
	bindGrace      = 70 * time.Second
	inFlightPoll   = 50 * time.Millisecond
	schedulerName  = "default-scheduler"
	reportingActor = "default-scheduler"
)

var volumePlugins = map[string]bool{"VolumeBinding": true, "VolumeRestrictions": true, "NodeVolumeLimits": true, "VolumeZone": true, "DynamicResources": true}

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
	enabled := config.Profiles[0].Plugins.MultiPoint.Enabled[:0]
	for _, p := range config.Profiles[0].Plugins.MultiPoint.Enabled {
		if !volumePlugins[p.Name] {
			enabled = append(enabled, p)
		}
	}
	config.Profiles[0].Plugins.MultiPoint.Enabled = enabled
	factory := scheduler.NewInformerFactory(client, 0)
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
	return fill(factory.Policy().V1().PodDisruptionBudgets().Informer(), func() ([]runtime.Object, error) {
		l, err := client.PolicyV1().PodDisruptionBudgets("").List(ctx, all)
		if err != nil {
			return nil, err
		}
		out := make([]runtime.Object, 0, len(l.Items))
		for i := range l.Items {
			out = append(out, &l.Items[i])
		}
		return out, nil
	})
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
