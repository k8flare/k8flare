package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"sigs.k8s.io/yaml"
)

const servingCertPath = "/v1-k3s/serving-node-proxy.crt"

func upstreamTransport(pool *x509.CertPool) *http.Transport {
	return &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:     &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		TLSNextProto:        map[string]func(string, *tls.Conn) http.RoundTripper{},
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     90 * time.Second,
	}
}

func newProxy(target *url.URL, pool *x509.CertPool) http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = target.Host
		},
		Transport:     upstreamTransport(pool),
		FlushInterval: -1,
	}
}

type certStore struct {
	cert atomic.Pointer[tls.Certificate]
}

func (s *certStore) set(c *tls.Certificate) { s.cert.Store(c) }

func (s *certStore) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c := s.cert.Load()
	if c == nil {
		return nil, errors.New("no serving certificate yet")
	}
	return c, nil
}

type certFetcher struct {
	target    *url.URL
	pool      *x509.CertPool
	tokenFile string
}

func (f certFetcher) request(ctx context.Context) ([]byte, *ecdsa.PrivateKey, error) {
	token, err := os.ReadFile(f.tokenFile)
	if err != nil {
		return nil, nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "node-proxy"}}, key)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.target.JoinPath(servingCertPath).String(), bytes.NewReader(csr))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	req.Header.Set("Content-Type", "application/pkcs10")
	client := &http.Client{Transport: upstreamTransport(f.pool), Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("%s: %s: %s", servingCertPath, resp.Status, strings.TrimSpace(string(body)))
	}
	return body, key, nil
}

func (f certFetcher) fetch(ctx context.Context) (*tls.Certificate, *x509.Certificate, error) {
	certPEM, key, err := f.request(ctx)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	pair, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		return nil, nil, err
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, nil, err
	}
	return &pair, leaf, nil
}

func renewDelay(cert *x509.Certificate, now time.Time) time.Duration {
	lifetime := cert.NotAfter.Sub(cert.NotBefore)
	delay := cert.NotBefore.Add(lifetime * 2 / 3).Sub(now)
	if delay < time.Minute {
		return time.Minute
	}
	return delay
}

func (f certFetcher) keep(ctx context.Context, store *certStore, logf func(string, ...any)) {
	wait := time.Duration(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		pair, leaf, err := f.fetch(ctx)
		if err != nil {
			logf("serving certificate: %v", err)
			wait = 10 * time.Second
			continue
		}
		store.set(pair)
		wait = renewDelay(leaf, time.Now())
		logf("serving certificate installed; renewing in %s", wait.Round(time.Second))
	}
}

type kubeconfig struct {
	Clusters []struct {
		Cluster struct {
			Server               string `json:"server"`
			CertificateAuthority string `json:"certificate-authority"`
		} `json:"cluster"`
	} `json:"clusters"`
}

func loadKubeconfig(path string) (*url.URL, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var kc kubeconfig
	if err := yaml.Unmarshal(raw, &kc); err != nil {
		return nil, "", err
	}
	if len(kc.Clusters) == 0 || kc.Clusters[0].Cluster.Server == "" {
		return nil, "", fmt.Errorf("%s has no cluster server", path)
	}
	server, err := url.Parse(kc.Clusters[0].Cluster.Server)
	if err != nil {
		return nil, "", err
	}
	return server, kc.Clusters[0].Cluster.CertificateAuthority, nil
}
