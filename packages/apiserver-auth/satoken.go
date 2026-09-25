package auth

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	mrand "math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const saIssuer = "https://kubernetes.default.svc.cluster.local"
const saIssuerLegacy = "k8flare"
const saKeyID = "sa"

type saClaims struct {
	jwt.RegisteredClaims
	Kubernetes saKubernetes `json:"kubernetes.io"`
}

type saKubernetes struct {
	Namespace      string              `json:"namespace"`
	ServiceAccount saServiceAccountRef `json:"serviceaccount"`
}

type saServiceAccountRef struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}

type ServiceAccountToken struct {
	HMAC []byte
}

func IssueServiceAccountToken(hmac []byte, namespace, name, uid string, exp time.Time, audiences []string) (string, error) {
	key, err := saPrivateKey(hmac)
	if err != nil {
		return "", err
	}
	if len(audiences) == 0 {
		audiences = []string{"https://kubernetes.default.svc"}
	}
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, saClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        string(uuid.NewUUID()),
			Issuer:    saIssuer,
			Subject:   "system:serviceaccount:" + namespace + ":" + name,
			Audience:  audiences,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Kubernetes: saKubernetes{Namespace: namespace, ServiceAccount: saServiceAccountRef{Name: name, UID: uid}},
	})
	t.Header["kid"] = saKeyID
	return t.SignedString(key)
}

func (s ServiceAccountToken) AuthenticateToken(_ context.Context, token string) (*authenticator.Response, bool, error) {
	if len(s.HMAC) == 0 || token == "" {
		return nil, false, nil
	}
	key, err := saPrivateKey(s.HMAC)
	if err != nil {
		return nil, false, nil
	}
	claims := &saClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(tok *jwt.Token) (any, error) {
		if tok.Method.Alg() == jwt.SigningMethodRS256.Alg() {
			return &key.PublicKey, nil
		}
		return s.HMAC, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg(), jwt.SigningMethodHS256.Alg()}))
	if err != nil || !parsed.Valid {
		return nil, false, nil
	}
	if claims.Issuer != saIssuer && claims.Issuer != saIssuerLegacy {
		return nil, false, nil
	}
	ns := claims.Kubernetes.Namespace
	name := claims.Kubernetes.ServiceAccount.Name
	if ns == "" || name == "" {
		return nil, false, nil
	}
	info := &user.DefaultInfo{
		Name:   "system:serviceaccount:" + ns + ":" + name,
		UID:    claims.Kubernetes.ServiceAccount.UID,
		Groups: []string{"system:serviceaccounts", "system:serviceaccounts:" + ns, user.AllAuthenticated},
	}
	if claims.ID != "" {
		info.Extra = map[string][]string{user.CredentialIDKey: {"JTI=" + claims.ID}}
	}
	return &authenticator.Response{User: info}, true, nil
}

func (s ServiceAccountToken) ServeOpenID(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"issuer":                                saIssuer,
		"jwks_uri":                              saIssuer + "/openid/v1/jwks",
		"response_types_supported":              []string{"id_token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (s ServiceAccountToken) ServeJWKS(w http.ResponseWriter, _ *http.Request) {
	key, err := saPrivateKey(s.HMAC)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{"keys": []any{rsaJWK(&key.PublicKey)}})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

var saKeys sync.Map

const saSigningKey = "/vault/sa-signing-key"

func InstallServiceAccountKey(ctx context.Context, store *kine.Client, secret []byte) error {
	if store == nil || len(secret) == 0 {
		return nil
	}
	for {
		kv, _, err := store.Get(ctx, saSigningKey)
		if err == nil {
			raw, err := base64.StdEncoding.DecodeString(kv.Value)
			if err != nil {
				return err
			}
			block, _ := pem.Decode(raw)
			if block == nil {
				return errors.New("service account signing key is not pem")
			}
			key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return err
			}
			UseServiceAccountKey(secret, key)
			return nil
		}
		if err != kine.ErrNotFound {
			return err
		}
		key, err := NewServiceAccountKey(secret)
		if err != nil {
			return err
		}
		body := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
		if _, err := store.Put(ctx, saSigningKey, body, 0); err != nil && err != kine.ErrConflict {
			return err
		}
	}
}

func UseServiceAccountKey(secret []byte, key *rsa.PrivateKey) {
	if len(secret) == 0 || key == nil {
		return
	}
	saKeys.Store(string(secret), key)
}

func NewServiceAccountKey(secret []byte) (*rsa.PrivateKey, error) {
	if len(secret) == 0 {
		return nil, errors.New("service account key is empty")
	}
	sum := sha256.Sum256(secret)
	return rsa.GenerateKey(mrand.NewChaCha8(sum), 2048)
}

func saPrivateKey(secret []byte) (*rsa.PrivateKey, error) {
	if len(secret) == 0 {
		return nil, errors.New("service account key is empty")
	}
	if v, ok := saKeys.Load(string(secret)); ok {
		return v.(*rsa.PrivateKey), nil
	}
	key, err := NewServiceAccountKey(secret)
	if err != nil {
		return nil, err
	}
	actual, _ := saKeys.LoadOrStore(string(secret), key)
	return actual.(*rsa.PrivateKey), nil
}

func rsaJWK(pub *rsa.PublicKey) map[string]string {
	return map[string]string{
		"kty": "RSA",
		"use": "sig",
		"alg": "RS256",
		"kid": saKeyID,
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}
