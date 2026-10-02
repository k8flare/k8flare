package hpa

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/cache"
	podautoscaler "k8s.io/kubernetes/pkg/controller/podautoscaler"
	metricsclient "k8s.io/kubernetes/pkg/controller/podautoscaler/metrics"
)

const (
	listPage = 500
	maxDrain = 10 * time.Second
)

type Result struct {
	Objects map[string]int `json:"objects"`
	Drained bool           `json:"drained"`
	NextMs  int64          `json:"nextMs"`
}

type pageFunc func(context.Context, metav1.ListOptions) (runtime.Object, error)

type loadedSource struct {
	informer *snapshotInformer
	objs     []runtime.Object
}

func Sync(ctx context.Context, client kubernetes.Interface, scales scale.ScalesGetter, metrics metricsclient.MetricsClient, mapper meta.RESTMapper) (*Result, error) {
	return syncFor(ctx, client, scales, metrics, mapper, maxDrain)
}

func syncFor(ctx context.Context, client kubernetes.Interface, scales scale.ScalesGetter, metrics metricsclient.MetricsClient, mapper meta.RESTMapper, drainFor time.Duration) (*Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	hpas, err := list(ctx, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
		return client.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, o)
	})
	if err != nil {
		return nil, fmt.Errorf("horizontalpodautoscalers: %w", err)
	}
	if len(hpas) == 0 {
		return &Result{
			Objects: map[string]int{"horizontalpodautoscalers": 0},
			Drained: true,
		}, nil
	}

	pods, err := list(ctx, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
		return client.CoreV1().Pods("").List(ctx, o)
	})
	if err != nil {
		return nil, fmt.Errorf("pods: %w", err)
	}

	factory := informers.NewSharedInformerFactory(client, 0)
	result := &Result{
		Objects: map[string]int{
			"pods":                     len(pods),
			"horizontalpodautoscalers": len(hpas),
		},
	}
	all := []loadedSource{
		{register(factory, &v1.Pod{}), pods},
		{register(factory, &autoscalingv2.HorizontalPodAutoscaler{}), hpas},
	}

	ctrl := podautoscaler.NewHorizontalController(
		ctx,
		client.CoreV1(),
		scales,
		client.AutoscalingV2(),
		mapper,
		metrics,
		factory.Autoscaling().V2().HorizontalPodAutoscalers(),
		factory.Core().V1().Pods(),
		50*time.Millisecond,
		0,
		0.1,
		0,
		0,
	)
	for _, l := range all {
		l.informer.fill(l.objs)
	}
	for _, l := range all {
		l.informer.replay(l.objs)
	}
	done := make(chan struct{})
	go func() { ctrl.Run(ctx, 1); close(done) }()
	time.Sleep(drainFor)
	result.Drained = true
	cancel()
	<-done
	return result, nil
}

func RESTMapper() meta.RESTMapper {
	m := meta.NewDefaultRESTMapper([]schema.GroupVersion{appsv1.SchemeGroupVersion, v1.SchemeGroupVersion})
	for _, kind := range []schema.GroupVersionKind{
		appsv1.SchemeGroupVersion.WithKind("Deployment"),
		appsv1.SchemeGroupVersion.WithKind("ReplicaSet"),
		appsv1.SchemeGroupVersion.WithKind("StatefulSet"),
		v1.SchemeGroupVersion.WithKind("ReplicationController"),
	} {
		m.Add(kind, meta.RESTScopeNamespace)
	}
	return m
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
