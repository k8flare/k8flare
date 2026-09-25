package admission

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"net/http/httptest"
	"strings"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCertificateSubjectRestrictionDeniesMastersClientCSR(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	t.Cleanup(kineSrv.Close)
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, csrAdmitReq("pooh", certificatesv1.KubeAPIServerClientSignerName, pemWithOrg("system:masters")))
	if out.Allowed {
		t.Fatal("expected deny")
	}
	want := "use of kubernetes.io/kube-apiserver-client signer with system:masters group is not allowed"
	if !strings.Contains(out.Message, want) {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestCertificateSubjectRestrictionAllowsOtherGroup(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	t.Cleanup(kineSrv.Close)
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, csrAdmitReq("ok", certificatesv1.KubeAPIServerClientSignerName, pemWithOrg("system:admin")))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
}

func TestCertificateSubjectRestrictionIgnoresOtherSigner(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	t.Cleanup(kineSrv.Close)
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, csrAdmitReq("kubelet", certificatesv1.KubeAPIServerClientKubeletSignerName, pemWithOrg("system:masters")))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
}

func TestCertificateSubjectRestrictionDeniesMastersFromBase64(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	t.Cleanup(kineSrv.Close)
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	pemReq := pemWithOrg("system:masters")
	out := postAdmit(t, h, csrAdmitReq("pooh-b64", certificatesv1.KubeAPIServerClientSignerName, base64.StdEncoding.EncodeToString([]byte(pemReq))))
	if out.Allowed {
		t.Fatal("expected deny")
	}
	if !strings.Contains(out.Message, "use of kubernetes.io/kube-apiserver-client signer with system:masters group is not allowed") {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestCertificateSubjectRestrictionRejectsInvalidPEM(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	t.Cleanup(kineSrv.Close)
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, csrAdmitReq("bear", certificatesv1.KubeAPIServerClientSignerName, "this is not a CSR"))
	if out.Allowed {
		t.Fatal("expected parse deny")
	}
	if !strings.Contains(out.Message, "failed to parse CSR: PEM block type must be CERTIFICATE REQUEST") {
		t.Fatalf("message = %q", out.Message)
	}
}

func csrAdmitReq(name, signer, request string) admit.Request {
	return admit.Request{
		Phase:     "validate",
		Name:      name,
		Resource:  schema.GroupVersionResource{Group: "certificates.k8s.io", Version: "v1", Resource: "certificatesigningrequests"},
		Kind:      schema.GroupVersionKind{Group: "certificates.k8s.io", Version: "v1", Kind: "CertificateSigningRequest"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "certificates.k8s.io/v1",
			"kind":       "CertificateSigningRequest",
			"metadata":   map[string]any{"name": name},
			"spec": map[string]any{
				"request":    request,
				"signerName": signer,
				"usages":     []any{"client auth"},
			},
		},
	}
}

func pemWithOrg(org string) string {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{Organization: []string{org}},
	}, key)
	if err != nil {
		panic(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}
