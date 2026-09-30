package supervisor

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"time"
)

var caNames = []string{"server-ca", "client-ca"}

type CAStatus struct {
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	NotBefore time.Time `json:"notBefore"`
	NotAfter  time.Time `json:"notAfter"`
	Current   bool      `json:"current"`
	Expired   bool      `json:"expired"`
}

// RotateCAs replaces the server and client CAs. The previous certificates
// stay in the bundles the supervisor serves, and each new CA is cross-signed
// by the one it replaces, so nodes that still trust only the old CA accept
// leaves issued by the new one and nothing has to re-join.
func (v *Vault) RotateCAs(ctx context.Context) ([]CAStatus, error) {
	for _, name := range caNames {
		if err := v.rotateCA(ctx, name); err != nil {
			return nil, fmt.Errorf("rotate %s: %w", name, err)
		}
	}
	return v.CAStatuses(ctx)
}

func (v *Vault) rotateCA(ctx context.Context, name string) error {
	if _, err := v.ca(ctx, name); err != nil {
		return err
	}
	key := "/vault/ca/" + name
	kv, _, err := v.kine.Get(ctx, key)
	if err != nil {
		return err
	}
	old, err := parseCA(kv)
	if err != nil {
		return err
	}
	record, err := generateCA("k8flare-" + name)
	if err != nil {
		return err
	}
	next, err := newCA(record)
	if err != nil {
		return err
	}
	cross, err := old.crossSign(next)
	if err != nil {
		return err
	}
	record.Previous = append([]string{string(old.certPEM)}, old.previous...)
	record.Cross = append([]string{cross}, old.cross...)
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := v.kine.Put(ctx, key, data, kv.ModRevision); err != nil {
		if err == kine.ErrConflict {
			return fmt.Errorf("the CA changed while rotating, retry")
		}
		return err
	}
	v.mu.Lock()
	delete(v.cas, name)
	v.mu.Unlock()
	return nil
}

func (v *Vault) CAStatuses(ctx context.Context) ([]CAStatus, error) {
	now := v.now()
	var out []CAStatus
	for _, name := range caNames {
		c, err := v.ca(ctx, name)
		if err != nil {
			return nil, err
		}
		out = append(out, caStatus(name, c.cert, true, now))
		for _, previous := range c.previous {
			block, _ := pem.Decode([]byte(previous))
			if block == nil {
				continue
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				continue
			}
			out = append(out, caStatus(name, cert, false, now))
		}
	}
	return out, nil
}

func caStatus(name string, cert *x509.Certificate, current bool, now time.Time) CAStatus {
	return CAStatus{Name: name, Subject: cert.Subject.CommonName, NotBefore: cert.NotBefore, NotAfter: cert.NotAfter, Current: current, Expired: !cert.NotAfter.After(now)}
}
