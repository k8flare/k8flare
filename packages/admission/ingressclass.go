package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	_ "k8s.io/kubernetes/pkg/apis/networking/install"
	"k8s.io/kubernetes/plugin/pkg/admission/network/defaultingressclass"
)

func applyDefaultIngressClass(ctx context.Context, s *store, req *admit.Request) error {
	f := newStoreInformerFactory(ctx, s)
	p, err := initUpstreamPlugin(defaultingressclass.Register, defaultingressclass.PluginName, f)
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}
