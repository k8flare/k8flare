package certificates

import (
	"testing"

	"github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "certificates.k8s.io", Version: "v1"}, metav1.APIResource{Name: "certificatesigningrequests", Kind: "CertificateSigningRequest"})
	obj := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "c"},
		Spec:       certificatesv1.CertificateSigningRequestSpec{Request: []byte("not a pem"), SignerName: "example.com/signer", Usages: []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth}},
	}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.request", "")
}
