package supervisor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func TestIssueEdgeCertificate(t *testing.T) {
	v := NewVault(fakeKine(t))
	issued, err := v.IssueEdgeCertificate(context.Background(), []string{"api.example.com", "127.0.0.1"}, 48*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair([]byte(issued.Cert), []byte(issued.Key))
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(issued.ServerCA)) {
		t.Fatal("server CA is not a certificate")
	}
	serverCA, _ := v.CAPEM(context.Background(), "server-ca")
	if issued.ServerCA != string(serverCA) {
		t.Fatal("the returned CA is not the one /cacerts serves")
	}
	for _, host := range []string{"api.example.com", "127.0.0.1"} {
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: host, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			t.Fatalf("%s: %v", host, err)
		}
	}
	if until := time.Until(leaf.NotAfter); until < 47*time.Hour || until > 49*time.Hour {
		t.Fatalf("valid for %v", until)
	}
	block, _ := pem.Decode([]byte(issued.ClientCA))
	clientCA, _ := v.CAPEM(context.Background(), "client-ca")
	if block == nil || issued.ClientCA != string(clientCA) {
		t.Fatal("the returned client CA is not the vault's client CA")
	}
}

func TestIssueEdgeCertificateNeedsAHost(t *testing.T) {
	v := NewVault(fakeKine(t))
	if _, err := v.IssueEdgeCertificate(context.Background(), nil, time.Hour); err == nil {
		t.Fatal("issued a certificate for no host")
	}
}
