package certificates

import (
	"context"
	"testing"

	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

func TestPrepareForCreateStampsRequester(t *testing.T) {
	csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "node"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username: "forged",
			UID:      "forged-uid",
			Groups:   []string{"forged"},
			Extra:    map[string]certificatesv1.ExtraValue{"k": {"v"}},
			Request:  testCSRPEM(t),
		},
		Status: certificatesv1.CertificateSigningRequestStatus{Certificate: []byte("pre")},
	}
	ctx := genericapirequest.WithUser(context.Background(), &user.DefaultInfo{
		Name:   "system:node:foo",
		UID:    "u1",
		Groups: []string{"system:nodes", "system:authenticated"},
		Extra:  map[string][]string{"pod": {"p"}},
	})
	(csrCreateStrategy{}).PrepareForCreate(ctx, csr)
	if csr.Spec.Username != "system:node:foo" || csr.Spec.UID != "u1" {
		t.Fatalf("user %q uid %q", csr.Spec.Username, csr.Spec.UID)
	}
	if len(csr.Spec.Groups) != 2 || csr.Spec.Groups[0] != "system:nodes" {
		t.Fatalf("groups %v", csr.Spec.Groups)
	}
	if got := []string(csr.Spec.Extra["pod"]); len(got) != 1 || got[0] != "p" {
		t.Fatalf("extra %+v", csr.Spec.Extra)
	}
	if len(csr.Status.Certificate) != 0 {
		t.Fatalf("status %+v", csr.Status)
	}
}

func TestPrepareForUpdateKeepsSpec(t *testing.T) {
	oldCSR := &certificatesv1.CertificateSigningRequest{
		Spec: certificatesv1.CertificateSigningRequestSpec{Username: "system:node:foo", SignerName: "kubernetes.io/kube-apiserver-client-kubelet"},
		Status: certificatesv1.CertificateSigningRequestStatus{Certificate: []byte("cert")},
	}
	newCSR := oldCSR.DeepCopy()
	newCSR.Spec.Username = "admin"
	newCSR.Spec.SignerName = "changed"
	newCSR.Status.Certificate = []byte("other")
	(csrUpdateStrategy{}).PrepareForUpdate(context.Background(), newCSR, oldCSR)
	if newCSR.Spec.Username != "system:node:foo" || newCSR.Spec.SignerName != oldCSR.Spec.SignerName {
		t.Fatalf("spec %+v", newCSR.Spec)
	}
	if string(newCSR.Status.Certificate) != "cert" {
		t.Fatalf("status %+v", newCSR.Status)
	}
}
