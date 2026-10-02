package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	_ "k8s.io/kubernetes/pkg/apis/node/install"
	"k8s.io/kubernetes/plugin/pkg/admission/runtimeclass"
)

func newRuntimeClassPlugin(ctx context.Context, s *store) (*runtimeclass.RuntimeClass, *storeInformerFactory, error) {
	p := runtimeclass.NewRuntimeClass()
	f := newStoreInformerFactory(ctx, s)
	p.SetExternalKubeClientSet(&runtimeClassIndexerClient{indexer: f.runtimeClass})
	p.SetExternalKubeInformerFactory(f)
	if err := p.ValidateInitialization(); err != nil {
		return nil, nil, err
	}
	return p, f, nil
}

func applyRuntimeClass(ctx context.Context, s *store, req *admit.Request) error {
	p, f, err := newRuntimeClassPlugin(ctx, s)
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}

func validateRuntimeClass(ctx context.Context, s *store, req *admit.Request) error {
	p, f, err := newRuntimeClassPlugin(ctx, s)
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}
