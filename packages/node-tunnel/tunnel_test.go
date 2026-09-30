//go:build js && wasm

package nodetunnel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func testPair(t *testing.T) (certPEM, keyPEM string) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,

		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
}

func TestKubeletTLSConfigVerifiesServerAgainstCA(t *testing.T) {
	certPEM, keyPEM := testPair(t)
	cfg, err := kubeletTLSConfig(certPEM, keyPEM, certPEM)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InsecureSkipVerify {
		t.Fatal("server certificate verification is disabled")
	}
	if len(cfg.Certificates) != 1 || cfg.RootCAs == nil || cfg.ServerName == "" {
		t.Fatalf("incomplete config: %+v", cfg)
	}
}

func TestKubeletTLSConfigRefusesWithoutCredentials(t *testing.T) {
	certPEM, keyPEM := testPair(t)
	for name, args := range map[string][3]string{
		"no cert":    {"", keyPEM, certPEM},
		"no key":     {certPEM, "", certPEM},
		"bad pair":   {certPEM, certPEM, certPEM},
		"no ca":      {certPEM, keyPEM, ""},
		"invalid ca": {certPEM, keyPEM, "not pem"},
	} {
		if _, err := kubeletTLSConfig(args[0], args[1], args[2]); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
