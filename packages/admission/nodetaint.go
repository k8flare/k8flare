package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/kubernetes/plugin/pkg/admission/nodetaint"
)

var taintNodesByConditionPlugin = admission.Interface(nodetaint.NewPlugin())

func applyTaintNodesByCondition(ctx context.Context, _ *store, req *admit.Request) error {
	return runUpstreamPlugin(ctx, taintNodesByConditionPlugin, req)
}
