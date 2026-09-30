package supervisor

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

const (
	joinTokenPrefix = "/vault/join-tokens/"
	joinTokenIDLen  = 6
	joinTokenSecLen = 16
	tokenAlphabet   = "abcdefghijklmnopqrstuvwxyz0123456789"
)

var (
	ErrJoinTokenNotFound = errors.New("join token not found")
	errJoinTokenInvalid  = errors.New("join token is invalid or expired")
)

type JoinToken struct {
	ID          string     `json:"id"`
	Description string     `json:"description,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

type joinTokenRecord struct {
	JoinToken
	Hash string `json:"hash"`
}

func randomString(n int) (string, error) {
	out := make([]byte, n)
	max := big.NewInt(int64(len(tokenAlphabet)))
	for i := range out {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = tokenAlphabet[idx.Int64()]
	}
	return string(out), nil
}

func (v *Vault) readJoinToken(ctx context.Context, id string) (*joinTokenRecord, int64, error) {
	kv, _, err := v.kine.Get(ctx, joinTokenPrefix+id)
	if err == kine.ErrNotFound {
		return nil, 0, ErrJoinTokenNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	return decodeJoinToken(kv)
}

func decodeJoinToken(kv *kine.KV) (*joinTokenRecord, int64, error) {
	raw, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return nil, 0, err
	}
	var record joinTokenRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, 0, err
	}
	return &record, kv.ModRevision, nil
}

func (v *Vault) CreateJoinToken(ctx context.Context, description string, ttl time.Duration) (JoinToken, string, error) {
	for {
		id, err := randomString(joinTokenIDLen)
		if err != nil {
			return JoinToken{}, "", err
		}
		secret, err := randomString(joinTokenSecLen)
		if err != nil {
			return JoinToken{}, "", err
		}
		now := v.now()
		record := joinTokenRecord{
			JoinToken: JoinToken{ID: id, Description: description, CreatedAt: now.UTC()},
			Hash:      hashPassword(secret),
		}
		if ttl > 0 {
			expires := now.Add(ttl).UTC()
			record.ExpiresAt = &expires
		}
		data, _ := json.Marshal(record)
		if _, err := v.kine.Put(ctx, joinTokenPrefix+id, data, 0); err == kine.ErrConflict {
			continue
		} else if err != nil {
			return JoinToken{}, "", err
		}
		return record.JoinToken, id + "." + secret, nil
	}
}

func (v *Vault) ListJoinTokens(ctx context.Context) ([]JoinToken, error) {
	kvs, _, _, err := v.kine.List(ctx, joinTokenPrefix, "", 0)
	if err != nil {
		return nil, err
	}
	out := make([]JoinToken, 0, len(kvs))
	for i := range kvs {
		record, _, err := decodeJoinToken(&kvs[i])
		if err != nil {
			return nil, err
		}
		out = append(out, record.JoinToken)
	}
	return out, nil
}

func (v *Vault) DeleteJoinToken(ctx context.Context, id string) error {
	if _, err := v.kine.Delete(ctx, joinTokenPrefix+id, 0); err == kine.ErrNotFound {
		return ErrJoinTokenNotFound
	} else if err != nil {
		return err
	}
	return nil
}

func (v *Vault) RotateJoinToken(ctx context.Context, id string) (JoinToken, string, error) {
	record, revision, err := v.readJoinToken(ctx, id)
	if err != nil {
		return JoinToken{}, "", err
	}
	secret, err := randomString(joinTokenSecLen)
	if err != nil {
		return JoinToken{}, "", err
	}
	record.Hash = hashPassword(secret)
	data, _ := json.Marshal(record)
	if _, err := v.kine.Put(ctx, joinTokenPrefix+id, data, revision); err != nil {
		return JoinToken{}, "", err
	}
	return record.JoinToken, id + "." + secret, nil
}

func (v *Vault) CheckJoinToken(ctx context.Context, token string) error {
	id, secret, ok := strings.Cut(token, ".")
	if !ok || len(id) != joinTokenIDLen || secret == "" {
		return errJoinTokenInvalid
	}
	record, _, err := v.readJoinToken(ctx, id)
	if err == ErrJoinTokenNotFound {
		return errJoinTokenInvalid
	}
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(record.Hash), []byte(hashPassword(secret))) != 1 {
		return errJoinTokenInvalid
	}
	if record.ExpiresAt != nil && !v.now().Before(*record.ExpiresAt) {
		return errJoinTokenInvalid
	}
	return nil
}
