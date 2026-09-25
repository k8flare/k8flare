package attachdetach

import (
	"context"
	"fmt"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	attachdetachctrl "k8s.io/kubernetes/pkg/controller/volume/attachdetach"
	"k8s.io/kubernetes/pkg/volume/csi"
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
	core := client.CoreV1()
	storage := client.StorageV1()
	return []source{
		{"nodes", &v1.Node{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Nodes().List(ctx, o)
		}},
		{"persistentvolumes", &v1.PersistentVolume{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.PersistentVolumes().List(ctx, o)
		}},
		{"persistentvolumeclaims", &v1.PersistentVolumeClaim{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.PersistentVolumeClaims("").List(ctx, o)
		}},
		{"csinodes", &storagev1.CSINode{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return storage.CSINodes().List(ctx, o)
		}},
		{"csidrivers", &storagev1.CSIDriver{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return storage.CSIDrivers().List(ctx, o)
		}},
		{"volumeattachments", &storagev1.VolumeAttachment{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return storage.VolumeAttachments().List(ctx, o)
		}},
		{"pods", &v1.Pod{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Pods("").List(ctx, o)
		}},
	}
}

func Sync(ctx context.Context, client kubernetes.Interface) (*Result, error) {
	return syncFor(ctx, client, maxDrain)
}

func syncFor(ctx context.Context, client kubernetes.Interface, drainFor time.Duration) (*Result, error) {
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

	core := factory.Core().V1()
	storage := factory.Storage().V1()
	adc, err := attachdetachctrl.NewAttachDetachController(
		ctx,
		client,
		core.Pods(),
		core.Nodes(),
		core.PersistentVolumeClaims(),
		core.PersistentVolumes(),
		storage.CSINodes(),
		storage.CSIDrivers(),
		storage.VolumeAttachments(),
		csi.ProbeVolumePlugins(),
		nil,
		false,
		time.Second,
		false,
		attachdetachctrl.TimerConfig{
			ReconcilerLoopPeriod:                              100 * time.Millisecond,
			ReconcilerMaxWaitForUnmountDuration:               6 * time.Second,
			DesiredStateOfWorldPopulatorLoopSleepPeriod:       time.Second,
			DesiredStateOfWorldPopulatorListPodsRetryDuration: 3 * time.Second,
		},
	)
	if err != nil {
		return nil, err
	}
	for _, l := range all {
		l.informer.fill(l.objs)
	}
	for _, l := range all {
		l.informer.replay(l.objs)
	}
	done := make(chan struct{})
	go func() { adc.Run(ctx); close(done) }()
	time.Sleep(drainFor)
	result.Drained = true
	cancel()
	<-done
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
