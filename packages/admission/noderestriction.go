package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	_ "k8s.io/kubernetes/pkg/apis/authentication/install"
	_ "k8s.io/kubernetes/pkg/apis/certificates/install"
	_ "k8s.io/kubernetes/pkg/apis/coordination/install"
	_ "k8s.io/kubernetes/pkg/apis/core/install"
	_ "k8s.io/kubernetes/pkg/apis/policy/install"
	_ "k8s.io/kubernetes/pkg/apis/resource/install"
	_ "k8s.io/kubernetes/pkg/apis/storage/install"
	"k8s.io/kubernetes/plugin/pkg/admission/noderestriction"
)

const mirrorPodAnnotationKey = "kubernetes.io/config.mirror"

func applyNodeRestriction(ctx context.Context, s *store, authz authorizer.Authorizer, req *admit.Request) error {
	if authz == nil {
		authz = abstainAuthorizer{}
	}
	f := newStoreInformerFactory(ctx, s)
	p, err := initUpstreamPluginWithAuthorizer(noderestriction.Register, noderestriction.PluginName, f, authz)
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}

type abstainAuthorizer struct{}

func (abstainAuthorizer) Authorize(context.Context, authorizer.Attributes) (authorizer.Decision, string, error) {
	return authorizer.DecisionNoOpinion, "", nil
}
