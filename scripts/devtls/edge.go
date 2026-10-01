package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	cfBlobHeader     = "MF-CF-Blob"
	clientCertHeader = "X-K8flare-Client-Cert"
)

type edgeCertificate struct {
	Cert     string `json:"cert"`
	Key      string `json:"key"`
	ServerCA string `json:"serverCA"`
	ClientCA string `json:"clientCA"`
}

func fetchEdgeCertificate(upstream, adminToken string, hosts []string) (edgeCertificate, error) {
	body, _ := json.Marshal(map[string]any{"hosts": hosts})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(upstream, "/")+"/internal/edge-certificate", bytes.NewReader(body))
	if err != nil {
		return edgeCertificate{}, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return edgeCertificate{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return edgeCertificate{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return edgeCertificate{}, fmt.Errorf("edge certificate: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var issued edgeCertificate
	return issued, json.Unmarshal(data, &issued)
}

func awaitEdgeCertificate(upstream, adminToken string, hosts []string, attempts int) (edgeCertificate, error) {
	var err error
	for i := 0; i < attempts; i++ {
		var issued edgeCertificate
		if issued, err = fetchEdgeCertificate(upstream, adminToken, hosts); err == nil {
			return issued, nil
		}
		time.Sleep(time.Second)
	}
	return edgeCertificate{}, err
}

func clientAuthBlob(state *tls.ConnectionState, clientCAs *x509.CertPool) string {
	auth := map[string]any{"certPresented": "0", "certVerified": "NONE"}
	if len(state.PeerCertificates) > 0 {
		leaf := state.PeerCertificates[0]
		intermediates := x509.NewCertPool()
		for _, c := range state.PeerCertificates[1:] {
			intermediates.AddCert(c)
		}
		_, err := leaf.Verify(x509.VerifyOptions{Roots: clientCAs, Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
		auth["certPresented"] = "1"
		if err != nil {
			auth["certVerified"] = "FAILED:" + err.Error()
		} else {
			auth["certVerified"] = "SUCCESS"
			auth["certRFC9440"] = ":" + base64.StdEncoding.EncodeToString(leaf.Raw) + ":"
			auth["certSubjectDN"] = leaf.Subject.String()
			auth["certIssuerDN"] = leaf.Issuer.String()
		}
	}
	blob, _ := json.Marshal(map[string]any{"tlsClientAuth": auth})
	return string(blob)
}

func mtlsEdge(next http.Handler, clientCAs *x509.CertPool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del(clientCertHeader)
		r.Header.Del(cfBlobHeader)
		if r.TLS != nil {
			r.Header.Set(cfBlobHeader, clientAuthBlob(r.TLS, clientCAs))
		}
		next.ServeHTTP(w, r)
	})
}

func filterHost(next http.Handler, hosts []string, strict bool) http.Handler {
	if !strict {
		return next
	}
	allowed := make(map[string]bool)
	for _, h := range hosts {
		if h = strings.TrimSpace(h); h != "" {
			if host, _, err := net.SplitHostPort(h); err == nil {
				allowed[strings.ToLower(host)] = true
			} else {
				allowed[strings.ToLower(h)] = true
			}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.ToLower(strings.Trim(host, "[]"))
		if !allowed[host] {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func edgeTLSConfig(certs []tls.Certificate, clientCAs *x509.CertPool, strict bool) *tls.Config {
	cfg := &tls.Config{
		Certificates: certs,
		MinVersion:   tls.VersionTLS12,
	}
	if clientCAs != nil {
		cfg.ClientAuth = tls.RequestClientCert
		cfg.ClientCAs = clientCAs
	}
	if strict {
		cfg.GetConfigForClient = func(info *tls.ClientHelloInfo) (*tls.Config, error) {
			if info.ServerName == "" {
				return nil, errors.New("SNI is required")
			}
			return nil, nil
		}
	}
	return cfg
}

