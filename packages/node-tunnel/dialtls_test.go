//go:build js && wasm

package nodetunnel

import "testing"

func TestDialTLSConfigPresentsProxyClientCertificate(t *testing.T) {
	certPEM, keyPEM := testPair(t)
	cfg, err := dialTLSConfig("ext.ns.svc", "", certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Certificates) != 1 || cfg.ServerName != "ext.ns.svc" {
		t.Fatalf("incomplete config: %+v", cfg)
	}
	if !cfg.InsecureSkipVerify {
		t.Fatal("without a CA bundle the dial keeps skipping verification")
	}
}

func TestDialTLSConfigVerifiesAgainstCABundleAndKeepsClientCertificate(t *testing.T) {
	certPEM, keyPEM := testPair(t)
	cfg, err := dialTLSConfig("ext.ns.svc", certPEM, certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InsecureSkipVerify || cfg.RootCAs == nil || len(cfg.Certificates) != 1 {
		t.Fatalf("incomplete config: %+v", cfg)
	}
}

func TestDialTLSConfigWithoutProxyClientCertificate(t *testing.T) {
	cfg, err := dialTLSConfig("ext.ns.svc", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Certificates) != 0 {
		t.Fatalf("no certificate is configured: %+v", cfg)
	}
}

func TestDialTLSConfigRejectsMalformedProxyClientCertificate(t *testing.T) {
	if _, err := dialTLSConfig("ext.ns.svc", "", "bad", "bad"); err == nil {
		t.Fatal("malformed proxy client certificate must be an error")
	}
}
