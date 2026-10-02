package admission

import (
	"context"
	"encoding/json"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	_ "k8s.io/kubernetes/pkg/apis/scheduling/install"
	schedhelpers "k8s.io/kubernetes/pkg/apis/scheduling/v1"
	"k8s.io/kubernetes/plugin/pkg/admission/priority"
)

type systemPriorityClassIndexer struct {
	cache.Indexer
}

func (idx systemPriorityClassIndexer) GetByKey(key string) (any, bool, error) {
	obj, ok, err := idx.Indexer.GetByKey(key)
	if err != nil || ok {
		return obj, ok, err
	}
	for _, pc := range schedhelpers.SystemPriorityClasses() {
		if pc.Name == key {
			return pc, true, nil
		}
	}
	return nil, false, nil
}

func newPriorityPlugin(ctx context.Context, s *store) (*priority.Plugin, *storeInformerFactory, error) {
	p := priority.NewPlugin()
	f := newStoreInformerFactory(ctx, s)
	f.priorityClass = systemPriorityClassIndexer{f.priorityClass}
	p.SetExternalKubeClientSet(&dummyClient{})
	p.SetExternalKubeInformerFactory(f)
	if err := p.ValidateInitialization(); err != nil {
		return nil, nil, err
	}
	return p, f, nil
}

func applyPriority(ctx context.Context, s *store, req *admit.Request) error {
	p, f, err := newPriorityPlugin(ctx, s)
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}

func validatePriorityClass(ctx context.Context, s *store, req *admit.Request) error {
	p, f, err := newPriorityPlugin(ctx, s)
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}

func decodePod(obj map[string]any) (*corev1.Pod, error) {
	if obj == nil {
		return &corev1.Pod{}, nil
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	pod := &corev1.Pod{}
	if err := json.Unmarshal(raw, pod); err != nil {
		return nil, err
	}
	return pod, nil
}
