// Command devtls terminates TLS in front of `wrangler dev`, which only
// speaks plain HTTP. kubectl refuses to send a bearer token over plain
// HTTP and the k3s agent requires an https:// server URL, so local
// verification goes through this proxy. It writes a CA certificate that
// clients (and the OrbStack VM) trust.
//
//	devtls -listen :6443 -upstream http://127.0.0.1:18787 -dir .build/devtls -hosts localhost,host.orb.internal
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	listen := flag.String("listen", ":6443", "address to serve https on")
	upstream := flag.String("upstream", "http://127.0.0.1:18787", "wrangler dev URL")
	dir := flag.String("dir", ".build/devtls", "where ca.crt, ca.key, server.crt, server.key live")
	hosts := flag.String("hosts", "localhost,host.orb.internal", "extra DNS SANs (127.0.0.1 is always included)")
	adminToken := flag.String("admin-token", os.Getenv("ADMIN_TOKEN"), "when set, serve a certificate issued by the cluster's server CA, verify client certificates against its client CA and pass them the way Cloudflare mTLS does")
	flag.Parse()
	if *adminToken != "" {
		if err := runEdge(*listen, *upstream, *dir, *adminToken, strings.Split(*hosts, ",")); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := run(*listen, *upstream, *dir, strings.Split(*hosts, ",")); err != nil {
		log.Fatal(err)
	}
}

func run(listen, upstream, dir string, hosts []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	caCert, caKey, err := loadOrCreateCA(dir)
	if err != nil {
		return err
	}
	serverCert, err := issueServerCert(caCert, caKey, hosts)
	if err != nil {
		return err
	}
	target, err := url.Parse(upstream)
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = retryDropped{base: http.DefaultTransport}
	proxy.FlushInterval = -1
	srv := &http.Server{
		Addr:      listen,
		Handler:   accessLog(proxy, 5*time.Second),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{serverCert}, MinVersion: tls.VersionTLS12},
	}
	log.Printf("devtls: https://%s -> %s (CA %s)", listen, upstream, filepath.Join(dir, "ca.crt"))
	return srv.ListenAndServeTLS("", "")
}

func runEdge(listen, upstream, dir, adminToken string, hosts []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	issued, err := awaitEdgeCertificate(upstream, adminToken, append([]string{"127.0.0.1", "::1"}, hosts...), 120)
	if err != nil {
		return err
	}
	serverCert, err := tls.X509KeyPair([]byte(issued.Cert), []byte(issued.Key))
	if err != nil {
		return err
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM([]byte(issued.ClientCA)) {
		return errors.New("the client CA is not a certificate")
	}
	serverCAPath := filepath.Join(dir, "server-ca.crt")
	if err := os.WriteFile(serverCAPath, []byte(issued.ServerCA), 0o644); err != nil {
		return err
	}
	target, err := url.Parse(upstream)
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = retryDropped{base: http.DefaultTransport}
	proxy.FlushInterval = -1
	srv := &http.Server{
		Addr:    listen,
		Handler: mtlsEdge(accessLog(proxy, 5*time.Second), clientCAs),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientAuth:   tls.RequestClientCert,
			ClientCAs:    clientCAs,
			MinVersion:   tls.VersionTLS12,
		},
	}
	log.Printf("devtls: https://%s -> %s (server CA %s, client certificates verified against the cluster client CA)", listen, upstream, serverCAPath)
	return srv.ListenAndServeTLS("", "")
}

func loadOrCreateCA(dir string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPath, keyPath := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "ca.key")
	if certPEM, err := os.ReadFile(certPath); err == nil {
		keyPEM, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, nil, err
		}
		cb, _ := pem.Decode(certPEM)
		kb, _ := pem.Decode(keyPEM)
		cert, err := x509.ParseCertificate(cb.Bytes)
		if err != nil {
			return nil, nil, err
		}
		key, err := x509.ParseECPrivateKey(kb.Bytes)
		return cert, key, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "k8flare-dev-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	return cert, key, err
}

func issueServerCert(ca *x509.Certificate, caKey *ecdsa.PrivateKey, hosts []string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "k8flare-dev"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	for _, h := range hosts {
		if h = strings.TrimSpace(h); h != "" {
			if ip := net.ParseIP(h); ip != nil {
				tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
			} else {
				tmpl.DNSNames = append(tmpl.DNSNames, h)
			}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.Raw}, PrivateKey: key}, nil
}
