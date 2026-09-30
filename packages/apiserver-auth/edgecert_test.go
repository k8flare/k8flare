package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newTestCA(t *testing.T, cn string) testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return testCA{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func (ca testCA) leaf(t *testing.T, cn string, orgs []string, usage x509.ExtKeyUsage) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn, Organization: orgs},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func rfc9440(der []byte) string { return ":" + base64.StdEncoding.EncodeToString(der) + ":" }

func edgeRequest(header string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "https://api.example.com/api", nil)
	if header != "" {
		r.Header.Set(ClientCertHeader, header)
	}
	return r
}

func TestEdgeClientCert(t *testing.T) {
	clientCA := newTestCA(t, "client-ca")
	foreign := newTestCA(t, "foreign")
	a := EdgeClientCert{ClientCA: func(context.Context) ([]byte, error) { return clientCA.pem, nil }}

	t.Run("a certificate from the client CA becomes its CN and O", func(t *testing.T) {
		resp, ok, err := a.AuthenticateRequest(edgeRequest(rfc9440(clientCA.leaf(t, "system:node:n1", []string{"system:nodes"}, x509.ExtKeyUsageClientAuth))))
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if resp.User.GetName() != "system:node:n1" {
			t.Fatalf("name %q", resp.User.GetName())
		}
		groups := resp.User.GetGroups()
		if len(groups) != 2 || groups[0] != "system:nodes" || groups[1] != "system:authenticated" {
			t.Fatalf("groups %v", groups)
		}
	})
	t.Run("a certificate from another CA is rejected", func(t *testing.T) {
		if _, ok, err := a.AuthenticateRequest(edgeRequest(rfc9440(foreign.leaf(t, "system:node:n1", []string{"system:nodes"}, x509.ExtKeyUsageClientAuth)))); ok || err == nil {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
	t.Run("a server certificate is rejected", func(t *testing.T) {
		if _, ok, _ := a.AuthenticateRequest(edgeRequest(rfc9440(clientCA.leaf(t, "system:node:n1", nil, x509.ExtKeyUsageServerAuth)))); ok {
			t.Fatal("server certificate authenticated")
		}
	})
	t.Run("a malformed header is an error", func(t *testing.T) {
		for _, header := range []string{"garbage", ":not base64:", ":AAAA:"} {
			if _, ok, err := a.AuthenticateRequest(edgeRequest(header)); ok || err == nil {
				t.Fatalf("%q: ok=%v err=%v", header, ok, err)
			}
		}
	})
	t.Run("no header makes no decision", func(t *testing.T) {
		if resp, ok, err := a.AuthenticateRequest(edgeRequest("")); resp != nil || ok || err != nil {
			t.Fatalf("resp=%v ok=%v err=%v", resp, ok, err)
		}
	})
}

func TestEdgeClientCertAcceptsBothCAsDuringRotation(t *testing.T) {
	oldCA := newTestCA(t, "old-client-ca")
	newCA := newTestCA(t, "new-client-ca")
	foreign := newTestCA(t, "foreign")
	bundle := append(append([]byte(nil), newCA.pem...), oldCA.pem...)
	a := EdgeClientCert{ClientCA: func(context.Context) ([]byte, error) { return bundle, nil }}
	for name, ca := range map[string]testCA{"old": oldCA, "new": newCA} {
		if _, ok, err := a.AuthenticateRequest(edgeRequest(rfc9440(ca.leaf(t, "system:node:n1", []string{"system:nodes"}, x509.ExtKeyUsageClientAuth)))); err != nil || !ok {
			t.Errorf("%s CA leaf: ok=%v err=%v", name, ok, err)
		}
	}
	if _, ok, _ := a.AuthenticateRequest(edgeRequest(rfc9440(foreign.leaf(t, "system:node:n1", []string{"system:nodes"}, x509.ExtKeyUsageClientAuth)))); ok {
		t.Error("a certificate from a CA outside the bundle was accepted")
	}
}
