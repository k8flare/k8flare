package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"k8s.io/kubernetes/plugin/pkg/admission/podtopologylabels"
)

func applyPodTopologyLabels(ctx context.Context, s *store, req *admit.Request) error {
	f := newStoreInformerFactory(ctx, s)
	p, err := initUpstreamPlugin(podtopologylabels.Register, podtopologylabels.PluginName, f)
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}
