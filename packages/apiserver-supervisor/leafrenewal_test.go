package supervisor

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func requestNodeLeaf(t *testing.T, mux *http.ServeMux, path string) []*x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "ignored"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(csr))
	req.Header.Set("Authorization", "Bearer join")
	req.Header.Set("k3s-Node-Name", "n1")
	req.Header.Set("k3s-Node-Password", "secret")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatal(path, rr.Code, rr.Body.String())
	}
	chain := certsIn(t, rr.Body.Bytes())
	if !chain[0].PublicKey.(*ecdsa.PublicKey).Equal(&key.PublicKey) {
		t.Fatalf("%s: the leaf does not carry the requested public key", path)
	}
	return chain
}

func TestAgentRestartedInsideItsRenewalWindowGetsAFullLifetimeLeaf(t *testing.T) {
	v := NewVault(fakeKine(t))
	mux := http.NewServeMux()
	New(v, "join").Register(mux)
	for _, c := range []struct {
		path, authority string
		usage           x509.ExtKeyUsage
	}{
		{"/v1-k3s/client-kubelet.crt", "client-ca", x509.ExtKeyUsageClientAuth},
		{"/v1-k3s/serving-kubelet.crt", "server-ca", x509.ExtKeyUsageServerAuth},
	} {
		t.Run(c.path, func(t *testing.T) {
			bundle, err := v.CAPEM(t.Context(), c.authority)
			if err != nil {
				t.Fatal(err)
			}
			roots := poolOf(certsIn(t, bundle)...)
			first := requestNodeLeaf(t, mux, c.path)
			second := requestNodeLeaf(t, mux, c.path)
			if first[0].SerialNumber.Cmp(second[0].SerialNumber) == 0 {
				t.Fatal("a second request returned the same leaf")
			}
			if left := time.Until(second[0].NotAfter); left < 364*24*time.Hour {
				t.Fatalf("the reissued leaf is valid for only %v", left)
			}
			insideWindow := first[0].NotAfter.Add(-30 * 24 * time.Hour)
			if _, err := second[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: poolOf(second[1:]...), CurrentTime: insideWindow, KeyUsages: []x509.ExtKeyUsage{c.usage}}); err != nil {
				t.Fatalf("the reissued leaf 30 days before the first one expires: %v", err)
			}
		})
	}
}

func TestLeafLifetimeCanPutAgentsInsideTheK3SRenewalWindow(t *testing.T) {
	v := NewVault(fakeKine(t))
	mux := http.NewServeMux()
	NewWithLeafLifetime(v, "join", time.Hour).Register(mux)
	leaf := requestNodeLeaf(t, mux, "/v1-k3s/client-kubelet.crt")[0]
	if time.Until(leaf.NotAfter) > time.Hour+time.Minute {
		t.Fatalf("the test leaf expires in %v, want at most one hour", time.Until(leaf.NotAfter))
	}
}
