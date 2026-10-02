package admission

import (
	"context"
	"encoding/json"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	_ "k8s.io/kubernetes/pkg/apis/scheduling/install"
	"k8s.io/kubernetes/plugin/pkg/admission/priority"
)

func newPriorityPlugin(ctx context.Context, s *store) (*priority.Plugin, error) {
	p := priority.NewPlugin()
	p.SetExternalKubeClientSet(&dummyClient{})
	p.SetExternalKubeInformerFactory(newStoreInformerFactory(ctx, s))
	if err := p.ValidateInitialization(); err != nil {
		return nil, err
	}
	return p, nil
}

func applyPriority(ctx context.Context, s *store, req *admit.Request) error {
	p, err := newPriorityPlugin(ctx, s)
	if err != nil {
		return err
	}
	return runUpstreamPlugin(ctx, p, req)
}

func validatePriorityClass(ctx context.Context, s *store, req *admit.Request) error {
	p, err := newPriorityPlugin(ctx, s)
	if err != nil {
		return err
	}
	return runUpstreamPlugin(ctx, p, req)
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
