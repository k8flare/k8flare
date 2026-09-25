package certificates

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"testing"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testCSRPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "node"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func TestApprovalPrepareKeepsSpecAndCertificate(t *testing.T) {
	oldCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "node", ResourceVersion: "3"},
		Spec:       certificatesv1.CertificateSigningRequestSpec{SignerName: "kubernetes.io/kube-apiserver-client", Request: testCSRPEM(t)},
		Status:     certificatesv1.CertificateSigningRequestStatus{Certificate: []byte("cert")},
	}
	newCSR := oldCSR.DeepCopy()
	newCSR.Spec.SignerName = "changed"
	newCSR.Status.Certificate = []byte("other")
	newCSR.Status.Conditions = []certificatesv1.CertificateSigningRequestCondition{{
		Type:   certificatesv1.CertificateApproved,
		Status: corev1.ConditionTrue,
		Reason: "Test",
	}}
	approvalStrategy{}.PrepareForUpdate(context.Background(), newCSR, oldCSR)
	if newCSR.Spec.SignerName != oldCSR.Spec.SignerName {
		t.Fatalf("spec %q", newCSR.Spec.SignerName)
	}
	if string(newCSR.Status.Certificate) != "cert" {
		t.Fatalf("certificate %q", newCSR.Status.Certificate)
	}
	if len(newCSR.Status.Conditions) != 1 || newCSR.Status.Conditions[0].Type != certificatesv1.CertificateApproved {
		t.Fatalf("conditions %+v", newCSR.Status.Conditions)
	}
	if newCSR.Status.Conditions[0].LastUpdateTime.IsZero() || newCSR.Status.Conditions[0].LastTransitionTime.IsZero() {
		t.Fatal("timestamps")
	}
}

func TestApprovalValidateRejectsApprovedAndDenied(t *testing.T) {
	oldCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "node", ResourceVersion: "3"},
		Spec:       certificatesv1.CertificateSigningRequestSpec{SignerName: "kubernetes.io/kube-apiserver-client", Request: testCSRPEM(t), Usages: []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth}},
	}
	newCSR := oldCSR.DeepCopy()
	newCSR.Status.Conditions = []certificatesv1.CertificateSigningRequestCondition{
		{Type: certificatesv1.CertificateApproved, Status: corev1.ConditionTrue, Reason: "A"},
		{Type: certificatesv1.CertificateDenied, Status: corev1.ConditionTrue, Reason: "D"},
	}
	if errs := (approvalStrategy{}).ValidateUpdate(context.Background(), newCSR, oldCSR); len(errs) == 0 {
		t.Fatal("expected validation error")
	}
}

func TestApprovalValidateAcceptsApproved(t *testing.T) {
	oldCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "node", ResourceVersion: "3"},
		Spec:       certificatesv1.CertificateSigningRequestSpec{SignerName: "kubernetes.io/kube-apiserver-client", Request: testCSRPEM(t), Usages: []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth}},
	}
	newCSR := oldCSR.DeepCopy()
	newCSR.Status.Conditions = []certificatesv1.CertificateSigningRequestCondition{{
		Type:   certificatesv1.CertificateApproved,
		Status: corev1.ConditionTrue,
		Reason: "Approved",
	}}
	if errs := (approvalStrategy{}).ValidateUpdate(context.Background(), newCSR, oldCSR); len(errs) != 0 {
		t.Fatalf("%v", errs)
	}
}
