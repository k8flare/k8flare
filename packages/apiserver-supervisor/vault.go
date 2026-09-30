package supervisor

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"math/big"
	"sync"
	"time"
)

// vault keeps the cluster's certificate authorities and node passwords in
// the Cluster DO, outside the /registry keyspace kubectl can reach.
type Vault struct {
	kine     *kine.Client
	mu       sync.Mutex
	cas      map[string]*ca
	verified map[string]string
	now      func() time.Time
}

type ca struct {
	cert    *x509.Certificate
	key     crypto.Signer
	certPEM []byte
}

type caRecord struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

var errNodePasswordMismatch = errors.New("node password does not match the stored one")

func NewVault(kine *kine.Client) *Vault {
	return &Vault{kine: kine, cas: map[string]*ca{}, verified: map[string]string{}, now: time.Now}
}

func (v *Vault) CAPEM(ctx context.Context, name string) ([]byte, error) {
	c, err := v.ca(ctx, name)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), c.certPEM...), nil
}

func (v *Vault) ca(ctx context.Context, name string) (*ca, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if c, ok := v.cas[name]; ok {
		return c, nil
	}
	key := "/vault/ca/" + name
	for {
		kv, _, err := v.kine.Get(ctx, key)
		if err == nil {
			c, err := parseCA(kv)
			if err != nil {
				return nil, err
			}
			v.cas[name] = c
			return c, nil
		}
		if err != kine.ErrNotFound {
			return nil, err
		}
		record, err := generateCA("k8flare-" + name)
		if err != nil {
			return nil, err
		}
		data, _ := json.Marshal(record)
		if _, err := v.kine.Put(ctx, key, data, 0); err != nil && err != kine.ErrConflict {
			return nil, err
		}
	}
}

func parseCA(kv *kine.KV) (*ca, error) {
	data, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return nil, err
	}
	var record caRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	certBlock, _ := pem.Decode([]byte(record.Cert))
	keyBlock, _ := pem.Decode([]byte(record.Key))
	if certBlock == nil || keyBlock == nil {
		return nil, fmt.Errorf("vault: malformed CA record")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	return &ca{cert: cert, key: key, certPEM: []byte(record.Cert)}, nil
}

func generateCA(cn string) (caRecord, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return caRecord{}, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: fmt.Sprintf("%s@%d", cn, now.Unix())},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return caRecord{}, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return caRecord{}, err
	}
	return caRecord{
		Cert: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		Key:  string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})),
	}, nil
}

// sign issues a certificate for the CSR's public key. The subject and the
// usages come from the server, never from the CSR, as in k3s.
func (c *ca) sign(csr *x509.CertificateRequest, tmpl *x509.Certificate) ([]byte, error) {
	tmpl.SerialNumber = big.NewInt(time.Now().UnixNano())
	tmpl.NotBefore = time.Now().Add(-time.Hour)
	if tmpl.NotAfter.IsZero() {
		tmpl.NotAfter = time.Now().Add(365 * 24 * time.Hour)
	}
	tmpl.BasicConstraintsValid = true
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, csr.PublicKey, c.key)
	if err != nil {
		return nil, err
	}
	out := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return append(out, c.certPEM...), nil
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

var errNodeUnknown = errors.New("node has not joined")

// registerNodePassword records a node's password on first sight and
// rejects a different one afterwards, which is what stops a second machine
// from taking over an existing node name. Only the supervisor, behind the
// join token, may call it.
func (v *Vault) RegisterNodePassword(ctx context.Context, node, password string) error {
	for {
		err := v.CheckNodePassword(ctx, node, password)
		if !errors.Is(err, errNodeUnknown) {
			return err
		}
		if _, err := v.kine.Put(ctx, "/vault/node/"+node, []byte(hashPassword(password)), 0); err != nil && err != kine.ErrConflict {
			return err
		}
	}
}

// checkNodePassword verifies a password against a node that has joined and
// never registers one. A password is immutable once registered, so a hash
// that verified once is kept for the isolate's lifetime and the kubelet's
// heartbeats do not each cost a store read.
const tokenPrefix = "/vault/tokens/"

func (v *Vault) EnsureToken(ctx context.Context, kind, seed string) (string, error) {
	key := tokenPrefix + kind
	for {
		kv, _, err := v.kine.Get(ctx, key)
		if err == nil {
			raw, err := base64.StdEncoding.DecodeString(kv.Value)
			if err != nil {
				return "", err
			}
			return string(raw), nil
		}
		if err != kine.ErrNotFound {
			return "", err
		}
		value := seed
		if value == "" {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				return "", err
			}
			value = hex.EncodeToString(b)
		}
		if _, err := v.kine.Put(ctx, key, []byte(value), 0); err != nil && err != kine.ErrConflict {
			return "", err
		}
	}
}

func (v *Vault) CheckToken(ctx context.Context, kind, token string) error {
	if token == "" {
		return errNodeUnknown
	}
	kv, _, err := v.kine.Get(ctx, tokenPrefix+kind)
	if err == kine.ErrNotFound {
		return errNodeUnknown
	}
	if err != nil {
		return err
	}
	stored, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(stored, []byte(token)) != 1 {
		return errNodePasswordMismatch
	}
	return nil
}

func (v *Vault) CheckNodePassword(ctx context.Context, node, password string) error {
	want := hashPassword(password)
	v.mu.Lock()
	cached, ok := v.verified[node]
	v.mu.Unlock()
	if ok && subtle.ConstantTimeCompare([]byte(cached), []byte(want)) == 1 {
		return nil
	}
	kv, _, err := v.kine.Get(ctx, "/vault/node/"+node)
	if err == kine.ErrNotFound {
		return errNodeUnknown
	}
	if err != nil {
		return err
	}
	stored, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(stored, []byte(want)) != 1 {
		return errNodePasswordMismatch
	}
	v.mu.Lock()
	v.verified[node] = want
	v.mu.Unlock()
	return nil
}
