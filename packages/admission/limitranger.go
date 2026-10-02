package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"k8s.io/kubernetes/plugin/pkg/admission/limitranger"
)

func newLimitRangerPlugin(ctx context.Context, s *store) (*limitranger.LimitRanger, *storeInformerFactory, error) {
	p, err := limitranger.NewLimitRanger(nil)
	if err != nil {
		return nil, nil, err
	}
	f := newStoreInformerFactory(ctx, s)
	p.SetExternalKubeClientSet(&limitRangeIndexerClient{indexer: f.limitRange})
	p.SetExternalKubeInformerFactory(f)
	if err := p.ValidateInitialization(); err != nil {
		return nil, nil, err
	}
	return p, f, nil
}

func applyLimitRanger(ctx context.Context, s *store, req *admit.Request) error {
	p, f, err := newLimitRangerPlugin(ctx, s)
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}

func validateLimitRanger(ctx context.Context, s *store, req *admit.Request) error {
	p, f, err := newLimitRangerPlugin(ctx, s)
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}
