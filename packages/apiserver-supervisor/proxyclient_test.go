package supervisor

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyClientIssuesAuthProxyCertFromRequestHeaderCA(t *testing.T) {
	v := newMemoryVault(t)
	rr := httptest.NewRecorder()
	New(v, "join").ProxyClient(rr, httptest.NewRequest(http.MethodPost, "/internal/proxy-client", nil))
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Code, rr.Body.String())
	}
	var out struct{ Cert, Key string }
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair([]byte(out.Cert), []byte(out.Key))
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if leaf.Subject.CommonName != "system:auth-proxy" || RequestHeaderCN != "system:auth-proxy" || len(leaf.Subject.Organization) != 0 {
		t.Fatalf("subject %v", leaf.Subject)
	}
	requestHeaderCA, err := v.CAPEM(t.Context(), RequestHeaderCAName)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(requestHeaderCA)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatal(err)
	}
	clientCA, err := v.CAPEM(t.Context(), "client-ca")
	if err != nil {
		t.Fatal(err)
	}
	other := x509.NewCertPool()
	other.AppendCertsFromPEM(clientCA)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: other, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err == nil {
		t.Fatal("proxy client cert must not chain to the client CA")
	}
}
