package scheduler

import (
	"context"
	"sort"
	"strings"
	"sync/atomic"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"k8s.io/kubernetes/pkg/scheduler"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/latest"
)

var volumePlugins = map[string]bool{"VolumeBinding": true, "VolumeRestrictions": true, "NodeVolumeLimits": true, "VolumeZone": true, "DynamicResources": true}

type Scheduler struct {
	sched    *scheduler.Scheduler
	factory  informers.SharedInformerFactory
	recorder events.EventBroadcasterAdapter
	synced   atomic.Bool
}

func New(ctx context.Context, cfg *rest.Config) (*Scheduler, error) {
	client, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "kube-scheduler"))
	if err != nil {
		return nil, err
	}
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
	recorder := events.NewEventBroadcasterAdapter(client)
	sched, err := scheduler.New(ctx, client, factory, nil, recorder.NewRecorder,
		scheduler.WithComponentConfigVersion(config.TypeMeta.APIVersion),
		scheduler.WithKubeConfig(cfg),
		scheduler.WithProfiles(config.Profiles...),
		scheduler.WithPercentageOfNodesToScore(config.PercentageOfNodesToScore),
		scheduler.WithPodMaxBackoffSeconds(config.PodMaxBackoffSeconds),
		scheduler.WithPodInitialBackoffSeconds(config.PodInitialBackoffSeconds),
		scheduler.WithParallelism(config.Parallelism),
	)
	if err != nil {
		return nil, err
	}
	return &Scheduler{sched: sched, factory: factory, recorder: recorder}, nil
}

func (s *Scheduler) Run(ctx context.Context) {
	s.recorder.StartRecordingToSink(ctx.Done())
	defer s.recorder.Shutdown()
	s.factory.Start(ctx.Done())
	s.factory.WaitForCacheSync(ctx.Done())
	s.synced.Store(true)
	s.sched.Run(ctx)
}

func (s *Scheduler) Pending() string {
	if !s.synced.Load() {
		return "starting"
	}
	pods, _ := s.sched.SchedulingQueue.PendingPods()
	uids := make([]string, 0, len(pods))
	for _, p := range pods {
		uids = append(uids, string(p.UID))
	}
	sort.Strings(uids)
	return strings.Join(uids, ",")
}

func (s *Scheduler) Idle() bool {
	q := s.sched.SchedulingQueue
	return s.synced.Load() && len(q.PodsInActiveQ()) == 0 && len(q.PodsInBackoffQ()) == 0 && len(q.InFlightPods()) == 0
}

var closedChannel = func() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()
