//go:build linux

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
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

func serveKubernetesAPI(ctx context.Context, server, nodeName, token string) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := runKubernetesAPI(ctx, server, nodeName, token); err != nil {
			log.Printf("kubernetes api proxy: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func runKubernetesAPI(ctx context.Context, server, nodeName, token string) error {
	password, err := os.ReadFile("/etc/rancher/node/password")
	if err != nil {
		return err
	}
	target, err := url.Parse(server)
	if err != nil {
		return err
	}
	cert, err := requestKubernetesServingCert(ctx, server, nodeName, strings.TrimSpace(string(password)), token)
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	base := proxy.Director
	proxy.Director = func(req *http.Request) {
		base(req)
		req.Host = target.Host
	}
	srv := &http.Server{
		Addr:    ":6443",
		Handler: proxy,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{*cert},
			MinVersion:   tls.VersionTLS12,
		},
	}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	log.Printf("kubernetes api proxy listening on :6443")
	err = srv.ListenAndServeTLS("", "")
	if err == http.ErrServerClosed {
		return ctx.Err()
	}
	return err
}

func requestKubernetesServingCert(ctx context.Context, server, nodeName, password, token string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: nodeName}}, key)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(server, "/")+"/v1-k3s/serving-kubernetes.crt", bytes.NewReader(der))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("k3s-Node-Name", nodeName)
	req.Header.Set("k3s-Node-Password", password)
	if ips := localNodeIPs(); ips != "" {
		req.Header.Set("k3s-Node-IP", ips)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errStatus(resp.StatusCode)
	}
	pemCerts, err := readAll(resp)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	tlsCert, err := tls.X509KeyPair(pemCerts, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		return nil, err
	}
	return &tlsCert, nil
}

func readAll(resp *http.Response) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(resp.Body)
	return buf.Bytes(), err
}

type statusError int

func (e statusError) Error() string { return "serving-kubernetes.crt: " + http.StatusText(int(e)) }

func errStatus(code int) error { return statusError(code) }

func localNodeIPs() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	var ips []string
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP.IsLoopback() || n.IP.IsLinkLocalUnicast() || n.IP.IsLinkLocalMulticast() {
			continue
		}
		ips = append(ips, n.IP.String())
	}
	return strings.Join(ips, ",")
}
