package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"k8s.io/kubernetes/plugin/pkg/admission/serviceaccount"
)

func applyServiceAccount(ctx context.Context, s *store, req *admit.Request) error {
	return runServiceAccount(ctx, s, req)
}

func validateServiceAccount(ctx context.Context, s *store, req *admit.Request) error {
	return runServiceAccount(ctx, s, req)
}

func runServiceAccount(ctx context.Context, s *store, req *admit.Request) error {
	f := newStoreInformerFactory(ctx, s)
	p, err := initUpstreamPluginWithClient(serviceaccount.Register, serviceaccount.PluginName, f, &storeClient{ctx: ctx, store: s})
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}
