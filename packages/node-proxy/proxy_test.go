package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func poolOf(t *testing.T, srv *httptest.Server) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return pool
}

func startProxy(t *testing.T, upstream *httptest.Server) *httptest.Server {
	t.Helper()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewUnstartedServer(newProxy(target, poolOf(t, upstream)))
	front.EnableHTTP2 = true
	front.StartTLS()
	t.Cleanup(front.Close)
	return front
}

func clientOf(front *httptest.Server) *http.Client {
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: poolOf(nil, front)}, ForceAttemptHTTP2: true}}
}

func TestProxyForwardsBearerTokenAndHost(t *testing.T) {
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Header.Get("Authorization")+"|"+r.Host+"|"+r.URL.RequestURI())
	}))
	upstream.EnableHTTP2 = true
	upstream.StartTLS()
	defer upstream.Close()
	front := startProxy(t, upstream)
	req, _ := http.NewRequest(http.MethodGet, front.URL+"/api/v1/pods?limit=1", nil)
	req.Header.Set("Authorization", "Bearer pod-token")
	resp, err := clientOf(front).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	upstreamHost := strings.TrimPrefix(upstream.URL, "https://")
	want := "Bearer pod-token|" + upstreamHost + "|/api/v1/pods?limit=1"
	if string(body) != want {
		t.Fatalf("got %q want %q", body, want)
	}
}

func TestProxyAddsNoCredentialsToUnauthenticatedRequests(t *testing.T) {
	type seen struct {
		authorization string
		peerCerts     int
	}
	got := make(chan seen, 1)
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{r.Header.Get("Authorization"), len(r.TLS.PeerCertificates)}
	}))
	upstream.TLS = &tls.Config{ClientAuth: tls.RequestClientCert}
	upstream.StartTLS()
	defer upstream.Close()
	front := startProxy(t, upstream)
	resp, err := clientOf(front).Get(front.URL + "/api")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if s := <-got; s.authorization != "" || s.peerCerts != 0 {
		t.Fatalf("upstream saw credentials: %#v", s)
	}
}

func TestProxyStreamsWatchResponsesWithoutBuffering(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "event-1\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
		_, _ = io.WriteString(w, "event-2\n")
	}))
	defer upstream.Close()
	defer close(release)
	front := startProxy(t, upstream)
	resp, err := clientOf(front).Get(front.URL + "/api/v1/pods?watch=true")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	line := make(chan string, 1)
	go func() {
		s, _ := bufio.NewReader(resp.Body).ReadString('\n')
		line <- s
	}()
	select {
	case s := <-line:
		if s != "event-1\n" {
			t.Fatalf("got %q", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first watch event was buffered")
	}
}

func TestProxyTunnelsUpgradedConnections(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: SPDY/3.1\r\n\r\n")
		_ = rw.Flush()
		line, _ := rw.ReadString('\n')
		_, _ = rw.WriteString("echo:" + line)
		_ = rw.Flush()
	}))
	defer upstream.Close()
	front := startProxy(t, upstream)
	conn, err := tls.Dial("tcp", strings.TrimPrefix(front.URL, "https://"), &tls.Config{RootCAs: poolOf(t, front), NextProtos: []string{"http/1.1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = io.WriteString(conn, "POST /api/v1/namespaces/default/pods/p/exec HTTP/1.1\r\nHost: k\r\nConnection: Upgrade\r\nUpgrade: SPDY/3.1\r\n\r\n")
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status %d", resp.StatusCode)
	}
	_, _ = io.WriteString(conn, "ping\n")
	line, _ := br.ReadString('\n')
	if line != "echo:ping\n" {
		t.Fatalf("got %q", line)
	}
}

func TestFetchServingCertificateSendsTokenAndCSR(t *testing.T) {
	var auth, contentType string
	var body []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")
		body, _ = io.ReadAll(r.Body)
		if r.URL.Path != "/v1-k3s/serving-node-proxy.crt" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("pem"))
	}))
	defer server.Close()
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("sa-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse(server.URL)
	f := certFetcher{target: target, pool: poolOf(t, server), tokenFile: tokenFile}
	got, key, err := f.request(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pem" {
		t.Fatalf("got %q", got)
	}
	if auth != "Bearer sa-token" {
		t.Fatalf("authorization %q", auth)
	}
	if contentType == "" {
		t.Fatal("missing content type")
	}
	csr, err := x509.ParseCertificateRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatal(err)
	}
	if key == nil {
		t.Fatal("no key")
	}
}

func TestFetchServingCertificateRejectsFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))
	defer server.Close()
	tokenFile := filepath.Join(t.TempDir(), "token")
	_ = os.WriteFile(tokenFile, []byte("t"), 0o600)
	target, _ := url.Parse(server.URL)
	f := certFetcher{target: target, pool: poolOf(t, server), tokenFile: tokenFile}
	if _, _, err := f.request(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestRenewDelayIsTwoThirdsOfLifetime(t *testing.T) {
	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cert := &x509.Certificate{NotBefore: notBefore, NotAfter: notBefore.Add(300 * time.Hour)}
	if got := renewDelay(cert, notBefore.Add(10*time.Hour)); got != 190*time.Hour {
		t.Fatalf("got %s", got)
	}
	if got := renewDelay(cert, notBefore.Add(299*time.Hour)); got != time.Minute {
		t.Fatalf("got %s", got)
	}
}

func TestLoadKubeconfigReadsServerAndCA(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kc")
	content := "apiVersion: v1\nkind: Config\nclusters:\n- name: local\n  cluster:\n    server: https://k8flare.example.com:443\n    certificate-authority: /var/lib/rancher/k3s/agent/server-ca.crt\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	server, ca, err := loadKubeconfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if server.Host != "k8flare.example.com:443" || ca != "/var/lib/rancher/k3s/agent/server-ca.crt" {
		t.Fatalf("got %v %q", server, ca)
	}
}

func TestCertStoreServesLatestCertificate(t *testing.T) {
	var s certStore
	if _, err := s.get(&tls.ClientHelloInfo{}); err == nil {
		t.Fatal("expected an error before a certificate is stored")
	}
	c := &tls.Certificate{}
	s.set(c)
	got, err := s.get(&tls.ClientHelloInfo{})
	if err != nil || got != c {
		t.Fatal(got, err)
	}
}
