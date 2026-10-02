package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"k8s.io/apiserver/pkg/admission"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/kubernetes/plugin/pkg/admission/defaulttolerationseconds"
)

var defaultTolerationSecondsPlugin = func() admission.Interface {
	p := defaulttolerationseconds.NewDefaultTolerationSeconds()
	p.InspectFeatureGates(utilfeature.DefaultFeatureGate)
	return p
}()

func applyDefaultTolerationSeconds(ctx context.Context, _ *store, req *admit.Request) error {
	return runUpstreamPlugin(ctx, defaultTolerationSecondsPlugin, req)
}
