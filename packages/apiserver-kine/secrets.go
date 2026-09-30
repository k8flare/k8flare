package kine

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"k8s.io/apiserver/pkg/storage/value"
	aestransformer "k8s.io/apiserver/pkg/storage/value/encrypt/aes"
)

const (
	secretsPrefix    = registryPrefix + "/secrets/"
	encryptedPrefix  = "k8s:enc:"
	aesGCMPrefixV1   = "k8s:enc:aesgcm:v1:"
	secretsScanLimit = 500
)

var errNoSecretsKey = errors.New("SECRETS_ENCRYPTION_KEYS is not set: refusing to write a Secret in plaintext")

type SecretCipher struct {
	names       []string
	transformer value.Transformer
}

func ParseSecretKeys(spec string) (*SecretCipher, error) {
	cipher := &SecretCipher{}
	if strings.TrimSpace(spec) == "" {
		return cipher, nil
	}
	var transformers []value.PrefixTransformer
	seen := map[string]bool{}
	for _, entry := range strings.Split(spec, ",") {
		name, secret, ok := strings.Cut(strings.TrimSpace(entry), ":")
		if !ok || name == "" || secret == "" {
			return nil, fmt.Errorf("secrets encryption key %q: want name:base64-secret", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("secrets encryption key %q: duplicate name", name)
		}
		seen[name] = true
		raw, err := base64.StdEncoding.DecodeString(secret)
		if err != nil {
			return nil, fmt.Errorf("secrets encryption key %q: %w", name, err)
		}
		block, err := aes.NewCipher(raw)
		if err != nil {
			return nil, fmt.Errorf("secrets encryption key %q: %w", name, err)
		}
		gcm, err := aestransformer.NewGCMTransformer(block)
		if err != nil {
			return nil, fmt.Errorf("secrets encryption key %q: %w", name, err)
		}
		transformers = append(transformers, value.PrefixTransformer{Prefix: []byte(aesGCMPrefixV1 + name + ":"), Transformer: gcm})
		cipher.names = append(cipher.names, name)
	}
	cipher.transformer = value.NewPrefixTransformers(errors.New("no secrets encryption key matches the stored value"), transformers...)
	return cipher, nil
}

func (s *SecretCipher) Ready() error {
	if s != nil && s.transformer == nil {
		return errNoSecretsKey
	}
	return nil
}

func (s *SecretCipher) covers(key string) bool {
	return s != nil && strings.HasPrefix(key, secretsPrefix)
}

func (s *SecretCipher) Seal(key string, plain []byte) ([]byte, error) {
	if !s.covers(key) {
		return plain, nil
	}
	if s.transformer == nil {
		return nil, errNoSecretsKey
	}
	return s.transformer.TransformToStorage(context.Background(), plain, value.DefaultContext(key))
}

func (s *SecretCipher) Open(key string, stored []byte) ([]byte, bool, error) {
	if !s.covers(key) {
		return stored, false, nil
	}
	if !bytes.HasPrefix(stored, []byte(encryptedPrefix)) {
		return stored, s.transformer != nil, nil
	}
	if s.transformer == nil {
		return nil, false, fmt.Errorf("%s is encrypted but no secrets encryption key is configured", key)
	}
	return s.transformer.TransformFromStorage(context.Background(), stored, value.DefaultContext(key))
}

func (s *SecretCipher) openKV(kv *KV) error {
	if !s.covers(kv.Key) {
		return nil
	}
	plain, _, err := s.openBase64(kv.Key, kv.Value)
	if err != nil {
		return err
	}
	kv.Value = plain
	return nil
}

func (s *SecretCipher) openBase64(key, b64 string) (string, bool, error) {
	stored, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", false, err
	}
	plain, stale, err := s.Open(key, stored)
	if err != nil {
		return "", false, err
	}
	return base64.StdEncoding.EncodeToString(plain), stale, nil
}

type SecretsStatus struct {
	Enabled      bool     `json:"enabled"`
	ActiveKey    string   `json:"activeKey,omitempty"`
	InactiveKeys []string `json:"inactiveKeys,omitempty"`
	Total        int      `json:"total"`
	Current      int      `json:"current"`
	Stale        int      `json:"stale"`
}

func (c *Client) scanSecrets(ctx context.Context, visit func(kv KV, plain []byte, stale bool) error) error {
	from := ""
	for {
		page, _, more, err := c.listRaw(ctx, secretsPrefix, from, secretsScanLimit, 0)
		if err != nil {
			return err
		}
		for _, kv := range page {
			stored, err := base64.StdEncoding.DecodeString(kv.Value)
			if err != nil {
				return err
			}
			plain, stale, err := c.Secrets.Open(kv.Key, stored)
			if err != nil {
				return err
			}
			if err := visit(kv, plain, stale); err != nil {
				return err
			}
		}
		if !more || len(page) == 0 {
			return nil
		}
		from = page[len(page)-1].Key + "\x00"
	}
}

func (c *Client) SecretsStatus(ctx context.Context) (SecretsStatus, error) {
	status := SecretsStatus{}
	if c.Secrets != nil && len(c.Secrets.names) > 0 {
		status.Enabled = true
		status.ActiveKey = "aesgcm " + c.Secrets.names[0]
		for _, name := range c.Secrets.names[1:] {
			status.InactiveKeys = append(status.InactiveKeys, "aesgcm "+name)
		}
	}
	err := c.scanSecrets(ctx, func(_ KV, _ []byte, stale bool) error {
		status.Total++
		if stale {
			status.Stale++
		}
		return nil
	})
	status.Current = status.Total - status.Stale
	return status, err
}

func (c *Client) ReencryptSecrets(ctx context.Context) (int, error) {
	if err := c.Secrets.Ready(); err != nil {
		return 0, err
	}
	rewritten := 0
	err := c.scanSecrets(ctx, func(kv KV, plain []byte, stale bool) error {
		if !stale {
			return nil
		}
		_, err := c.Put(ctx, kv.Key, plain, kv.ModRevision)
		if err == ErrConflict || err == ErrNotFound {
			return nil
		}
		if err == nil {
			rewritten++
		}
		return err
	})
	return rewritten, err
}
