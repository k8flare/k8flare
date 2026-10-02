package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"k8s.io/apiserver/pkg/admission"
	_ "k8s.io/kubernetes/pkg/apis/certificates/install"
	"k8s.io/kubernetes/plugin/pkg/admission/certificates/subjectrestriction"
)

var certificateSubjectRestrictionPlugin = admission.Interface(subjectrestriction.NewPlugin())

func applyCertificateSubjectRestriction(ctx context.Context, _ *store, req *admit.Request) error {
	return runUpstreamPlugin(ctx, certificateSubjectRestrictionPlugin, req)
}
