package admission

import (
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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
)

var (
	csrResource = schema.GroupVersionResource{Group: "certificates.k8s.io", Version: "v1", Resource: "certificatesigningrequests"}
	csrKind     = schema.GroupVersionKind{Group: "certificates.k8s.io", Version: "v1", Kind: "CertificateSigningRequest"}
)

func csrPEM(t *testing.T, organizations ...string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "cn", Organization: organizations}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func diffCSR(signer string, request []byte, mutate func(*certificatesv1.CertificateSigningRequest)) *certificatesv1.CertificateSigningRequest {
	csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "csr"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:    request,
			SignerName: signer,
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth},
		},
	}
	if mutate != nil {
		mutate(csr)
	}
	return csr
}

func csrCase(plugin, name, op, subresource string, csr, old *certificatesv1.CertificateSigningRequest) diffCase {
	c := diffCase{
		plugin: plugin, name: name, phase: "validate",
		resource: csrResource, kind: csrKind, objName: "csr",
		operation: admissionOperation(op), subresource: subresource,
		object: csr,
	}
	if old != nil {
		c.oldObject = old
	}
	return c
}

func TestDiffCertificateSubjectRestriction(t *testing.T) {
	const plugin = "CertificateSubjectRestriction"
	client := certificatesv1.KubeAPIServerClientSignerName
	masters := csrPEM(t, "system:masters")
	mixed := csrPEM(t, "dev", "system:masters")
	plain := csrPEM(t, "dev")
	none := csrPEM(t)
	badBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("x")})
	badDER := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: []byte("not der")})
	create := func(name string, signer string, request []byte) diffCase {
		return csrCase(plugin, name, "CREATE", "", diffCSR(signer, request, nil), nil)
	}
	cases := []diffCase{
		create("client signer with system:masters", client, masters),
		create("client signer with system:masters among others", client, mixed),
		create("client signer with another organization", client, plain),
		create("client signer with no organization", client, none),
		create("other signer with system:masters", certificatesv1.KubeAPIServerClientKubeletSignerName, masters),
		create("custom signer with system:masters", "example.com/custom", masters),
		create("empty signer with system:masters", "", masters),
		create("client signer with unparsable request", client, []byte("garbage")),
		create("client signer with an empty request", client, nil),
		create("client signer with the wrong PEM block type", client, badBlock),
		create("client signer with an unparsable DER body", client, badDER),
		create("other signer with unparsable request", "example.com/custom", []byte("garbage")),
		csrCase(plugin, "update is ignored", "UPDATE", "", diffCSR(client, masters, nil), diffCSR(client, plain, nil)),
		csrCase(plugin, "approval subresource is ignored", "UPDATE", "approval", diffCSR(client, masters, nil), diffCSR(client, plain, nil)),
		csrCase(plugin, "other resource is ignored", "CREATE", "", diffCSR(client, masters, nil), nil),
	}
	cases[len(cases)-1].resource.Resource = "configmaps"
	runDiffCases(t, cases)
}

func TestDiffCertificateApproval(t *testing.T) {
	const plugin = "CertificateApproval"
	alice := &user.DefaultInfo{Name: "alice"}
	request := csrPEM(t, "dev")
	signer := "example.com/custom"
	approving := func(name, op string, csr, old *certificatesv1.CertificateSigningRequest, allow allowSigners) diffCase {
		c := csrCase(plugin, name, op, "approval", csr, old)
		c.user = alice
		c.authorizer = allow
		return c
	}
	csr := diffCSR(signer, request, nil)
	cases := []diffCase{
		approving("authorized for the exact signer", "UPDATE", csr, csr, allowSigners{"alice/approve/" + signer: true}),
		approving("authorized through the domain wildcard", "UPDATE", csr, csr, allowSigners{"alice/approve/example.com/*": true}),
		approving("not authorized", "UPDATE", csr, csr, allowSigners{}),
		approving("authorized to sign only", "UPDATE", csr, csr, allowSigners{"alice/sign/" + signer: true}),
		approving("authorized for a different signer", "UPDATE", csr, csr, allowSigners{"alice/approve/other.com/x": true}),
		approving("signer changed in the new object is judged by the old one", "UPDATE", diffCSR("example.com/other", request, nil), csr, allowSigners{"alice/approve/" + signer: true}),
		approving("signer changed to the allowed one is still judged by the old one", "UPDATE", csr, diffCSR("example.com/other", request, nil), allowSigners{"alice/approve/" + signer: true}),
		approving("empty signer on the old object", "UPDATE", csr, diffCSR("", request, nil), allowSigners{}),
		approving("approval of an object with conditions", "UPDATE", diffCSR(signer, request, func(c *certificatesv1.CertificateSigningRequest) {
			c.Status.Conditions = []certificatesv1.CertificateSigningRequestCondition{{Type: certificatesv1.CertificateApproved, Status: corev1.ConditionTrue, Reason: "r"}}
		}), csr, allowSigners{}),
		approving("create is ignored", "CREATE", csr, nil, allowSigners{}),
		approving("approval without an old object", "UPDATE", csr, nil, allowSigners{}),
	}
	status := approving("status subresource is ignored", "UPDATE", csr, csr, allowSigners{})
	status.subresource = "status"
	plain := approving("plain update is ignored", "UPDATE", csr, csr, allowSigners{})
	plain.subresource = ""
	cases = append(cases, status, plain)
	cases = applyKnown(t, cases, map[string]*knownDifference{
		"empty signer on the old object": {reason: "ours falls back to the new object's signerName when the old one is empty and names it in the message, upstream always judges and names the old object's (approval/admission.go Validate)", messageOnly: true},
		"approval without an old object": {reason: "upstream rejects a missing old object as a type error, ours judges the new object's signerName (approval/admission.go Validate)", messageOnly: true},
	})
	runDiffCases(t, cases)
}

func TestDiffCertificateSigning(t *testing.T) {
	const plugin = "CertificateSigning"
	alice := &user.DefaultInfo{Name: "alice"}
	request := csrPEM(t, "dev")
	signer := "example.com/custom"
	signing := func(name string, csr, old *certificatesv1.CertificateSigningRequest, allow allowSigners) diffCase {
		c := csrCase(plugin, name, "UPDATE", "status", csr, old)
		c.user = alice
		c.authorizer = allow
		return c
	}
	base := diffCSR(signer, request, nil)
	withCert := diffCSR(signer, request, func(c *certificatesv1.CertificateSigningRequest) { c.Status.Certificate = []byte("cert") })
	otherCert := diffCSR(signer, request, func(c *certificatesv1.CertificateSigningRequest) { c.Status.Certificate = []byte("cert2") })
	approved := diffCSR(signer, request, func(c *certificatesv1.CertificateSigningRequest) {
		c.Status.Conditions = []certificatesv1.CertificateSigningRequestCondition{{Type: certificatesv1.CertificateApproved, Status: corev1.ConditionTrue, Reason: "r"}}
	})
	approvedAgain := diffCSR(signer, request, func(c *certificatesv1.CertificateSigningRequest) {
		c.Status.Conditions = []certificatesv1.CertificateSigningRequestCondition{{Type: certificatesv1.CertificateApproved, Status: corev1.ConditionTrue, Reason: "r"}}
	})
	cases := []diffCase{
		signing("certificate added by an authorized signer", withCert, base, allowSigners{"alice/sign/" + signer: true}),
		signing("certificate added by an unauthorized user", withCert, base, allowSigners{}),
		signing("certificate added through the domain wildcard", withCert, base, allowSigners{"alice/sign/example.com/*": true}),
		signing("certificate added by a user who may only approve", withCert, base, allowSigners{"alice/approve/" + signer: true}),
		signing("certificate replaced", otherCert, withCert, allowSigners{}),
		signing("condition added", approved, base, allowSigners{}),
		signing("conditions unchanged", approvedAgain, approved, allowSigners{}),
		signing("nothing changed", base, base, allowSigners{}),
		signing("signer changed in the new object is judged by the old one", diffCSR("example.com/other", request, func(c *certificatesv1.CertificateSigningRequest) { c.Status.Certificate = []byte("cert") }), base, allowSigners{"alice/sign/" + signer: true}),
		signing("empty old signer", withCert, diffCSR("", request, nil), allowSigners{}),
		signing("certificate removed", base, withCert, allowSigners{}),
		signing("status update without an old object", withCert, nil, allowSigners{}),
	}
	approval := signing("approval subresource is ignored", withCert, base, allowSigners{})
	approval.subresource = "approval"
	create := signing("create is ignored", withCert, nil, allowSigners{})
	create.operation = "CREATE"
	cases = append(cases, approval, create)
	cases = applyKnown(t, cases, map[string]*knownDifference{
		"empty old signer":                    {reason: "ours falls back to the new object's signerName when the old one is empty and names it in the message, upstream always judges and names the old object's (signing/admission.go Validate)", messageOnly: true},
		"status update without an old object": {reason: "upstream rejects a missing old object as a type error, ours judges the new object's signerName (signing/admission.go Validate)", messageOnly: true},
	})
	runDiffCases(t, cases)
}

func csrPEMWithCN(t *testing.T, cn string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: cn}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}
