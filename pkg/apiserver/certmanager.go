package apiserver

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"
)

// CAManager manages two CA keypairs (client CA and server CA) used for signing
// kubelet certificates. CA materials are persisted in durable storage and cached
// in memory after first load.
type CAManager struct {
	storage     *Storage
	mu          sync.Mutex
	initialized bool
	clientCA    *x509.Certificate
	clientKey   *ecdsa.PrivateKey
	serverCA    *x509.Certificate
	serverKey   *ecdsa.PrivateKey
}

// NewCAManager creates a new CAManager that uses the given storage for
// persisting CA certificates and keys.
func NewCAManager(storage *Storage) *CAManager {
	return &CAManager{
		storage: storage,
	}
}

const (
	clientCACertKey = "/ca/client-ca.crt"
	clientCAKeyKey  = "/ca/client-ca.key"
	serverCACertKey = "/ca/server-ca.crt"
	serverCAKeyKey  = "/ca/server-ca.key"

	clientCACN = "k3s-client-ca"
	serverCACN = "k3s-server-ca"

	caValidityDuration   = 10 * 365 * 24 * time.Hour // 10 years
	certValidityDuration = 365 * 24 * time.Hour       // 1 year

	// serialNumberMax is the upper bound for random serial numbers.
	// Using 128-bit random serial numbers as recommended by CA/Browser Forum.
	serialNumberBits = 128
)

// Initialize loads or generates the client and server CA keypairs. It is safe
// to call concurrently; only the first call performs initialization. If CA
// materials do not exist in storage, they are generated and stored atomically
// (using create-if-not-exists semantics to handle races between instances).
func (m *CAManager) Initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.initialized {
		return nil
	}

	clientCert, clientKey, err := m.loadOrCreateCA(ctx, clientCACertKey, clientCAKeyKey, clientCACN)
	if err != nil {
		return fmt.Errorf("initialize client CA: %w", err)
	}

	serverCert, serverKey, err := m.loadOrCreateCA(ctx, serverCACertKey, serverCAKeyKey, serverCACN)
	if err != nil {
		return fmt.Errorf("initialize server CA: %w", err)
	}

	m.clientCA = clientCert
	m.clientKey = clientKey
	m.serverCA = serverCert
	m.serverKey = serverKey
	m.initialized = true
	return nil
}

// loadOrCreateCA attempts to load a CA from storage. If not found, it generates
// a new CA and stores it. If a concurrent instance creates the CA between our
// check and our create, we re-read the stored value.
func (m *CAManager) loadOrCreateCA(ctx context.Context, certKey, keyKey, cn string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	// Try to load existing CA from storage.
	certObj, certErr := m.storage.Get(ctx, certKey)
	keyObj, keyErr := m.storage.Get(ctx, keyKey)

	if certErr == nil && keyErr == nil {
		return parseCertAndKey(certObj.Value, keyObj.Value)
	}

	// If one exists but not the other, something is corrupt.
	if (certErr == nil) != (keyErr == nil) {
		if !errors.Is(certErr, ErrNotFound) && certErr != nil {
			return nil, nil, fmt.Errorf("get cert: %w", certErr)
		}
		if !errors.Is(keyErr, ErrNotFound) && keyErr != nil {
			return nil, nil, fmt.Errorf("get key: %w", keyErr)
		}
		// One found, one not found — partial state. Try to read the missing one
		// again in case it was a transient issue, or generate both.
	}

	// Return unexpected errors (not ErrNotFound).
	if certErr != nil && !errors.Is(certErr, ErrNotFound) {
		return nil, nil, fmt.Errorf("get cert: %w", certErr)
	}
	if keyErr != nil && !errors.Is(keyErr, ErrNotFound) {
		return nil, nil, fmt.Errorf("get key: %w", keyErr)
	}

	// Generate new CA.
	certPEM, keyPEM, err := generateCA(cn)
	if err != nil {
		return nil, nil, fmt.Errorf("generate CA: %w", err)
	}

	// Try to store cert first, then key.
	_, certCreateErr := m.storage.Create(ctx, certKey, certPEM)
	if certCreateErr != nil && !errors.Is(certCreateErr, ErrKeyExists) {
		return nil, nil, fmt.Errorf("store cert: %w", certCreateErr)
	}

	_, keyCreateErr := m.storage.Create(ctx, keyKey, keyPEM)
	if keyCreateErr != nil && !errors.Is(keyCreateErr, ErrKeyExists) {
		return nil, nil, fmt.Errorf("store key: %w", keyCreateErr)
	}

	// If either already existed (race condition), re-read from storage.
	if errors.Is(certCreateErr, ErrKeyExists) || errors.Is(keyCreateErr, ErrKeyExists) {
		certObj, err = m.storage.Get(ctx, certKey)
		if err != nil {
			return nil, nil, fmt.Errorf("re-read cert after race: %w", err)
		}
		keyObj, err = m.storage.Get(ctx, keyKey)
		if err != nil {
			return nil, nil, fmt.Errorf("re-read key after race: %w", err)
		}
		return parseCertAndKey(certObj.Value, keyObj.Value)
	}

	return parseCertAndKey(certPEM, keyPEM)
}

// ServerCACertPEM returns the PEM-encoded server CA certificate.
// Initialize must be called before this method.
func (m *CAManager) ServerCACertPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: m.serverCA.Raw,
	})
}

// ClientCACertPEM returns the PEM-encoded client CA certificate.
// Initialize must be called before this method.
func (m *CAManager) ClientCACertPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: m.clientCA.Raw,
	})
}

// SignServingCert signs a CSR with the server CA to produce a serving certificate.
// The certificate will have the given node name as CN and include SANs for the
// node name, localhost, loopback addresses, and any additional node IPs.
// The CSR must be provided as DER-encoded bytes.
func (m *CAManager) SignServingCert(csrDER []byte, nodeName string, nodeIPs []net.IP) ([]byte, error) {
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return nil, fmt.Errorf("parse CSR: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("invalid CSR signature: %w", err)
	}

	serialNumber, err := randomSerialNumber()
	if err != nil {
		return nil, err
	}

	// Build SAN lists.
	dnsNames := []string{nodeName, "localhost"}
	ipAddresses := []net.IP{
		net.ParseIP("127.0.0.1"),
		net.ParseIP("::1"),
	}
	for _, ip := range nodeIPs {
		ipAddresses = append(ipAddresses, ip)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: nodeName,
		},
		NotBefore:             now,
		NotAfter:              now.Add(certValidityDuration),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, m.serverCA, csr.PublicKey, m.serverKey)
	if err != nil {
		return nil, fmt.Errorf("sign serving cert: %w", err)
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	}), nil
}

// SignClientCert signs a CSR with the client CA to produce a client certificate.
// The certificate will have the given CN and organization values set.
// The CSR must be provided as DER-encoded bytes.
func (m *CAManager) SignClientCert(csrDER []byte, cn string, org []string) ([]byte, error) {
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return nil, fmt.Errorf("parse CSR: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("invalid CSR signature: %w", err)
	}

	serialNumber, err := randomSerialNumber()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: org,
		},
		NotBefore:             now,
		NotAfter:              now.Add(certValidityDuration),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, m.clientCA, csr.PublicKey, m.clientKey)
	if err != nil {
		return nil, fmt.Errorf("sign client cert: %w", err)
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	}), nil
}

// GenerateTLSCertAndKey generates a TLS server certificate and key signed by
// the server CA. This is used for TLS proxies that need to present a certificate
// trusted by k3s agents. Returns PEM-encoded cert+key concatenated.
func (m *CAManager) GenerateTLSCertAndKey(dnsNames []string, ips []net.IP) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate TLS key: %w", err)
	}

	serialNumber, err := randomSerialNumber()
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: dnsNames[0],
		},
		NotBefore:             now,
		NotAfter:              now.Add(certValidityDuration),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ips,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, m.serverCA, &key.PublicKey, m.serverKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create TLS cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	// Append CA cert for chain
	certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: m.serverCA.Raw})...)

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal TLS key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}

// GenerateToken produces a join token in the format:
//
//	K10<sha256hex(serverCACertPEM)>::node:<password>
//
// This token encodes the server CA fingerprint so that joining nodes can verify
// they are connecting to the correct cluster.
func (m *CAManager) GenerateToken(password string) string {
	caCertPEM := m.ServerCACertPEM()
	hash := sha256.Sum256(caCertPEM)
	return "K10" + hex.EncodeToString(hash[:]) + "::node:" + password
}

// generateCA creates a new self-signed CA certificate and private key using
// ECDSA P-256. Returns PEM-encoded certificate and key.
func generateCA(cn string) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ECDSA key: %w", err)
	}

	serialNumber, err := randomSerialNumber()
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore:             now,
		NotAfter:              now.Add(caValidityDuration),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create CA certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal EC private key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	return certPEM, keyPEM, nil
}

// parseCertAndKey decodes PEM-encoded certificate and key bytes into their
// parsed representations.
func parseCertAndKey(certPEM, keyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, errors.New("failed to decode certificate PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse certificate: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, errors.New("failed to decode private key PEM")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse EC private key: %w", err)
	}

	return cert, key, nil
}

// randomSerialNumber generates a cryptographically random serial number
// suitable for X.509 certificates.
func randomSerialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), serialNumberBits)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}
	return serialNumber, nil
}
