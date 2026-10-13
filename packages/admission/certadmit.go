package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	_ "k8s.io/kubernetes/pkg/apis/certificates/install"
	"k8s.io/kubernetes/plugin/pkg/admission/certificates/approval"
	"k8s.io/kubernetes/plugin/pkg/admission/certificates/signing"
)

func applyCertificateApproval(ctx context.Context, authz authorizer.Authorizer, req *admit.Request) error {
	return runAuthorizedUpstreamPlugin(ctx, approval.Register, approval.PluginName, authz, req)
}

func applyCertificateSigning(ctx context.Context, authz authorizer.Authorizer, req *admit.Request) error {
	return runAuthorizedUpstreamPlugin(ctx, signing.Register, signing.PluginName, authz, req)
}

func runAuthorizedUpstreamPlugin(ctx context.Context, register func(*admission.Plugins), name string, authz authorizer.Authorizer, req *admit.Request) error {
	if authz == nil {
		authz = abstainAuthorizer{}
	}
	plugin, err := initUpstreamPluginWithAuthorizer(register, name, newStoreInformerFactory(ctx, nil), authz)
	if err != nil {
		return err
	}
	return runUpstreamPlugin(ctx, plugin, req)
}
