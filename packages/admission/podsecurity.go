package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"k8s.io/kubernetes/plugin/pkg/admission/security/podsecurity"
)

func applyPodSecurity(ctx context.Context, s *store, req *admit.Request) error {
	f := newStoreInformerFactory(ctx, s)
	p, err := initUpstreamPluginWithClient(podsecurity.Register, podsecurity.PluginName, f, &storeClient{ctx: ctx, store: s})
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}
