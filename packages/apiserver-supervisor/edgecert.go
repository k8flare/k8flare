package supervisor

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"net"
	"strings"
	"time"
)

type EdgeCertificate struct {
	Cert     string `json:"cert"`
	Key      string `json:"key"`
	ServerCA string `json:"serverCA"`
	ClientCA string `json:"clientCA"`
}

func (v *Vault) IssueEdgeCertificate(ctx context.Context, hosts []string, ttl time.Duration) (EdgeCertificate, error) {
	tmpl := &x509.Certificate{
		Subject:     pkix.Name{CommonName: "k8flare-edge"},
		NotAfter:    time.Now().Add(ttl),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, host := range hosts {
		if host = strings.TrimSpace(host); host == "" {
			continue
		}
		if ip := net.ParseIP(host); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, host)
		}
	}
	if len(tmpl.DNSNames)+len(tmpl.IPAddresses) == 0 {
		return EdgeCertificate{}, errors.New("at least one host is required")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return EdgeCertificate{}, err
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		return EdgeCertificate{}, err
	}
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return EdgeCertificate{}, err
	}
	serverCA, err := v.ca(ctx, "server-ca")
	if err != nil {
		return EdgeCertificate{}, err
	}
	cert, err := serverCA.sign(csr, tmpl)
	if err != nil {
		return EdgeCertificate{}, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return EdgeCertificate{}, err
	}
	clientCA, err := v.CAPEM(ctx, "client-ca")
	if err != nil {
		return EdgeCertificate{}, err
	}
	return EdgeCertificate{
		Cert:     string(cert),
		Key:      string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})),
		ServerCA: string(serverCA.certPEM),
		ClientCA: string(clientCA),
	}, nil
}
