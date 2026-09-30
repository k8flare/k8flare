package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type authority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  string
}

func newAuthority(t *testing.T, cn string) authority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
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
	return authority{cert, key, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))}
}

func (a authority) issue(t *testing.T, cn string, usage x509.ExtKeyUsage, ips ...string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"system:nodes"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	for _, ip := range ips {
		tmpl.IPAddresses = append(tmpl.IPAddresses, net.ParseIP(ip))
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, a.cert, &key.PublicKey, a.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

type seenHeaders struct{ blob, cert string }

func edgeServer(t *testing.T, serverAuth authority, clientAuth authority) (*httptest.Server, *seenHeaders) {
	t.Helper()
	seen := &seenHeaders{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.blob = r.Header.Get(cfBlobHeader)
		seen.cert = r.Header.Get(clientCertHeader)
	})
	pool := x509.NewCertPool()
	pool.AddCert(clientAuth.cert)
	srv := httptest.NewUnstartedServer(mtlsEdge(inner, pool))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverAuth.issue(t, "edge", x509.ExtKeyUsageServerAuth, "127.0.0.1")},
		ClientAuth:   tls.RequestClientCert,
		ClientCAs:    pool,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, seen
}

func call(t *testing.T, srv *httptest.Server, serverAuth authority, cert *tls.Certificate, headers map[string]string) {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(serverAuth.cert)
	cfg := &tls.Config{RootCAs: roots}
	if cert != nil {
		cfg.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return cert, nil }
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func tlsClientAuth(t *testing.T, blob string) map[string]any {
	t.Helper()
	var parsed struct {
		TLSClientAuth map[string]any `json:"tlsClientAuth"`
	}
	if err := json.Unmarshal([]byte(blob), &parsed); err != nil {
		t.Fatalf("blob %q: %v", blob, err)
	}
	return parsed.TLSClientAuth
}

func TestEdgeReportsAVerifiedClientCertificate(t *testing.T) {
	serverAuth, clientAuth := newAuthority(t, "server-ca"), newAuthority(t, "client-ca")
	srv, seen := edgeServer(t, serverAuth, clientAuth)
	leaf := clientAuth.issue(t, "system:node:n1", x509.ExtKeyUsageClientAuth)
	call(t, srv, serverAuth, &leaf, nil)
	got := tlsClientAuth(t, seen.blob)
	want := ":" + base64.StdEncoding.EncodeToString(leaf.Certificate[0]) + ":"
	if got["certPresented"] != "1" || got["certVerified"] != "SUCCESS" || got["certRFC9440"] != want {
		t.Fatalf("%v", got)
	}
	if seen.cert != "" {
		t.Fatalf("the certificate header reached the upstream: %q", seen.cert)
	}
}

func TestEdgeDoesNotVerifyACertificateFromAnotherCA(t *testing.T) {
	serverAuth, clientAuth, foreign := newAuthority(t, "server-ca"), newAuthority(t, "client-ca"), newAuthority(t, "foreign")
	srv, seen := edgeServer(t, serverAuth, clientAuth)
	leaf := foreign.issue(t, "system:node:n1", x509.ExtKeyUsageClientAuth)
	call(t, srv, serverAuth, &leaf, nil)
	got := tlsClientAuth(t, seen.blob)
	if got["certPresented"] != "1" || got["certVerified"] == "SUCCESS" {
		t.Fatalf("%v", got)
	}
	if _, ok := got["certRFC9440"]; ok {
		t.Fatalf("an unverified certificate was forwarded: %v", got)
	}
}

func TestEdgeReportsNoCertificateAndDropsForgedHeaders(t *testing.T) {
	serverAuth, clientAuth := newAuthority(t, "server-ca"), newAuthority(t, "client-ca")
	srv, seen := edgeServer(t, serverAuth, clientAuth)
	forged := `{"tlsClientAuth":{"certPresented":"1","certVerified":"SUCCESS","certRFC9440":":AAAA:"}}`
	call(t, srv, serverAuth, nil, map[string]string{cfBlobHeader: forged, clientCertHeader: ":AAAA:"})
	got := tlsClientAuth(t, seen.blob)
	if got["certPresented"] != "0" || got["certVerified"] != "NONE" {
		t.Fatalf("%v", got)
	}
	if seen.cert != "" {
		t.Fatalf("forged certificate header kept: %q", seen.cert)
	}
}

func TestFetchEdgeCertificate(t *testing.T) {
	var gotAuth string
	var gotBody struct {
		Hosts []string `json:"hosts"`
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if r.URL.Path != "/internal/edge-certificate" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"cert": "C", "key": "K", "serverCA": "S", "clientCA": "L"})
	}))
	t.Cleanup(upstream.Close)
	issued, err := fetchEdgeCertificate(upstream.URL, "admin-secret", []string{"localhost", "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if issued.Cert != "C" || issued.Key != "K" || issued.ServerCA != "S" || issued.ClientCA != "L" {
		t.Fatalf("%+v", issued)
	}
	if gotAuth != "Bearer admin-secret" || len(gotBody.Hosts) != 2 {
		t.Fatalf("%q %+v", gotAuth, gotBody)
	}
}
