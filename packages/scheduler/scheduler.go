package scheduler

import (
	"context"
	"fmt"
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
	running  atomic.Bool
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
	s.running.Store(true)
	s.sched.Run(ctx)
}

// Idle is false until Run has the scheduler going; an unstarted factory
// reports no informers, which would read as idle before any work began.
func (s *Scheduler) Idle() bool {
	if !s.running.Load() {
		return false
	}
	for _, synced := range s.factory.WaitForCacheSync(closedChannel) {
		if !synced {
			return false
		}
	}
	var active, backoff, unschedulable int
	_, summary := s.sched.SchedulingQueue.PendingPods()
	if _, err := fmt.Sscanf(summary, "activeQ:%d; backoffQ:%d; unschedulablePods:%d", &active, &backoff, &unschedulable); err != nil {
		return true
	}
	return active == 0 && backoff == 0
}

var closedChannel = func() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()
