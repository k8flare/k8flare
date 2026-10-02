package admission

import (
	"context"
	"encoding/json"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	_ "k8s.io/kubernetes/pkg/apis/scheduling/install"
	"k8s.io/kubernetes/plugin/pkg/admission/priority"
)

func newPriorityPlugin(ctx context.Context, s *store) (*priority.Plugin, *storeInformerFactory, error) {
	p := priority.NewPlugin()
	f := newStoreInformerFactory(ctx, s)
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
