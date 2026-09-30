package supervisor

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

func nodeProxyCSR(t *testing.T) *strings.Reader {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "node-proxy"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReader(string(der))
}

func TestServingNodeProxyCertificate(t *testing.T) {
	as := func(name string) authenticator.Request {
		return authenticator.RequestFunc(func(r *http.Request) (*authenticator.Response, bool, error) {
			if r.Header.Get("Authorization") != "Bearer sa-token" {
				return nil, false, nil
			}
			return &authenticator.Response{User: &user.DefaultInfo{Name: name}}, true, nil
		})
	}
	cases := []struct {
		name  string
		auth  authenticator.Request
		token string
		code  int
	}{
		{"the node-proxy service account", as(nodeProxyUser), "Bearer sa-token", http.StatusOK},
		{"another service account", as("system:serviceaccount:kube-system:coredns"), "Bearer sa-token", http.StatusForbidden},
		{"an unauthenticated request", as(nodeProxyUser), "", http.StatusUnauthorized},
		{"no authenticator configured", nil, "Bearer sa-token", http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := New(NewVault(fakeKine(t)), "join")
			s.ServiceAccounts = c.auth
			mux := http.NewServeMux()
			s.Register(mux)
			req := httptest.NewRequest(http.MethodPost, "/v1-k3s/serving-node-proxy.crt", nodeProxyCSR(t))
			if c.token != "" {
				req.Header.Set("Authorization", c.token)
			}
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code != c.code {
				t.Fatal(rr.Code, rr.Body.String())
			}
			if c.code != http.StatusOK {
				return
			}
			block, _ := pem.Decode(rr.Body.Bytes())
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			if err := cert.VerifyHostname("kubernetes.default.svc"); err != nil {
				t.Fatal(err)
			}
			if err := cert.VerifyHostname("10.43.0.1"); err != nil {
				t.Fatal(err)
			}
			if len(cert.IPAddresses) != 1 {
				t.Fatalf("got ips %v", cert.IPAddresses)
			}
			if len(cert.ExtKeyUsage) != 1 || cert.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
				t.Fatalf("got usages %v", cert.ExtKeyUsage)
			}
		})
	}
}

func TestAPIServersIncludesPort(t *testing.T) {
	cases := map[string]string{"k8flare.example.com": "k8flare.example.com:443", "k8flare.example.com:8443": "k8flare.example.com:8443"}
	for host, want := range cases {
		s := New(NewVault(fakeKine(t)), "join")
		mux := http.NewServeMux()
		s.Register(mux)
		req := httptest.NewRequest(http.MethodGet, "/v1-k3s/apiservers", nil)
		req.Host = host
		req.Header.Set("Authorization", "Bearer join")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if strings.TrimSpace(rr.Body.String()) != `["`+want+`"]` {
			t.Fatalf("host %s: got %s", host, rr.Body.String())
		}
	}
}
