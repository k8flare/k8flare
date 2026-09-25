package hpa

import (
	"context"
	"fmt"
	"sync"
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

type source struct {
	name    string
	example runtime.Object
	page    pageFunc
}

type loadedSource struct {
	informer *snapshotInformer
	objs     []runtime.Object
}

func sources(client kubernetes.Interface) []source {
	return []source{
		{"pods", &v1.Pod{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Pods("").List(ctx, o)
		}},
		{"horizontalpodautoscalers", &autoscalingv2.HorizontalPodAutoscaler{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, o)
		}},
	}
}

func Sync(ctx context.Context, client kubernetes.Interface, scales scale.ScalesGetter, metrics metricsclient.MetricsClient, mapper meta.RESTMapper) (*Result, error) {
	return syncFor(ctx, client, scales, metrics, mapper, maxDrain)
}

func syncFor(ctx context.Context, client kubernetes.Interface, scales scale.ScalesGetter, metrics metricsclient.MetricsClient, mapper meta.RESTMapper, drainFor time.Duration) (*Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	factory := informers.NewSharedInformerFactory(client, 0)
	result := &Result{Objects: map[string]int{}}
	src := sources(client)
	loaded := make([][]runtime.Object, len(src))
	var listErr error
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i, s := range src {
		wg.Add(1)
		go func() {
			defer wg.Done()
			objs, err := list(ctx, s.page)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				listErr = fmt.Errorf("%s: %w", s.name, err)
				return
			}
			loaded[i] = objs
		}()
	}
	wg.Wait()
	if listErr != nil {
		return nil, listErr
	}
	var all []loadedSource
	for i, s := range src {
		result.Objects[s.name] = len(loaded[i])
		all = append(all, loadedSource{register(factory, s.example), loaded[i]})
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
