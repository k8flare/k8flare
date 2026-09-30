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
	"fmt"
	"math/big"
	mrand "math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	apiserverserviceaccount "k8s.io/apiserver/pkg/authentication/serviceaccount"
	authenticationtokenjwt "k8s.io/apiserver/pkg/authentication/token/jwt"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
)

const saIssuer = "https://kubernetes.default.svc.cluster.local"
const saKeyID = "sa"
const saLeeway = time.Minute

var APIAudiences = authenticator.Audiences{saIssuer, "k3s"}

type saClaims struct {
	jwt.RegisteredClaims
	Kubernetes saKubernetes `json:"kubernetes.io"`
}

type saKubernetes struct {
	Namespace      string               `json:"namespace"`
	ServiceAccount saServiceAccountRef  `json:"serviceaccount"`
	Pod            *saServiceAccountRef `json:"pod,omitempty"`
	Secret         *saServiceAccountRef `json:"secret,omitempty"`
	Node           *saServiceAccountRef `json:"node,omitempty"`
}

type saServiceAccountRef struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}

type BoundObject struct {
	Name string
	UID  string
}

type BoundObjects struct {
	Pod    *BoundObject
	Secret *BoundObject
	Node   *BoundObject
}

func (b BoundObjects) claims() (pod, secret, node *saServiceAccountRef) {
	ref := func(o *BoundObject) *saServiceAccountRef {
		if o == nil {
			return nil
		}
		return &saServiceAccountRef{Name: o.Name, UID: o.UID}
	}
	return ref(b.Pod), ref(b.Secret), ref(b.Node)
}

type ServiceAccountObjects interface {
	ServiceAccount(ctx context.Context, namespace, name string) (*corev1.ServiceAccount, error)
	Pod(ctx context.Context, namespace, name string) (*corev1.Pod, error)
	Secret(ctx context.Context, namespace, name string) (*corev1.Secret, error)
	Node(ctx context.Context, name string) (*corev1.Node, error)
}

type KineObjects struct {
	Client *kine.Client
}

func (k KineObjects) get(ctx context.Context, key string, obj runtime.Object) error {
	s := kine.NewStorage(k.Client, scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), func() runtime.Object { return obj })
	return s.Get(ctx, key, storage.GetOptions{}, obj)
}

func (k KineObjects) ServiceAccount(ctx context.Context, namespace, name string) (*corev1.ServiceAccount, error) {
	out := &corev1.ServiceAccount{}
	return out, k.get(ctx, "/serviceaccounts/"+namespace+"/"+name, out)
}

func (k KineObjects) Pod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	out := &corev1.Pod{}
	return out, k.get(ctx, "/pods/"+namespace+"/"+name, out)
}

func (k KineObjects) Secret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	out := &corev1.Secret{}
	return out, k.get(ctx, "/secrets/"+namespace+"/"+name, out)
}

func (k KineObjects) Node(ctx context.Context, name string) (*corev1.Node, error) {
	out := &corev1.Node{}
	return out, k.get(ctx, "/nodes/"+name, out)
}

type ServiceAccountToken struct {
	HMAC    []byte
	Objects ServiceAccountObjects
}

func IssueServiceAccountToken(hmac []byte, namespace, name, uid string, exp time.Time, audiences []string, bound BoundObjects) (string, error) {
	key, err := saPrivateKey(hmac)
	if err != nil {
		return "", err
	}
	if len(audiences) == 0 {
		audiences = APIAudiences
	}
	now := time.Now()
	pod, secret, node := bound.claims()
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, saClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        string(uuid.NewUUID()),
			Issuer:    saIssuer,
			Subject:   "system:serviceaccount:" + namespace + ":" + name,
			Audience:  audiences,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
		Kubernetes: saKubernetes{Namespace: namespace, ServiceAccount: saServiceAccountRef{Name: name, UID: uid}, Pod: pod, Secret: secret, Node: node},
	})
	t.Header["kid"] = saKeyID
	return t.SignedString(key)
}

func (s ServiceAccountToken) AuthenticateToken(ctx context.Context, token string) (*authenticator.Response, bool, error) {
	if len(s.HMAC) == 0 || token == "" {
		return nil, false, nil
	}
	key, err := saPrivateKey(s.HMAC)
	if err != nil {
		return nil, false, nil
	}
	claims := &saClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return &key.PublicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}), jwt.WithoutClaimsValidation())
	if err != nil || !parsed.Valid {
		return nil, false, nil
	}
	if claims.Issuer != saIssuer {
		return nil, false, nil
	}
	if err := validateSATimes(claims, time.Now()); err != nil {
		return nil, false, err
	}
	ns := claims.Kubernetes.Namespace
	name := claims.Kubernetes.ServiceAccount.Name
	if ns == "" || name == "" {
		return nil, false, nil
	}
	auds, err := tokenAudiences(ctx, claims.Audience)
	if err != nil {
		return nil, false, err
	}
	if err := s.validateObjects(ctx, claims); err != nil {
		return nil, false, err
	}
	sa := &apiserverserviceaccount.ServiceAccountInfo{
		Namespace:    ns,
		Name:         name,
		UID:          claims.Kubernetes.ServiceAccount.UID,
		CredentialID: authenticationtokenjwt.CredentialIDForJTI(claims.ID),
	}
	if pod := claims.Kubernetes.Pod; pod != nil {
		sa.PodName, sa.PodUID = pod.Name, pod.UID
	}
	if node := claims.Kubernetes.Node; node != nil {
		sa.NodeName, sa.NodeUID = node.Name, node.UID
	}
	return &authenticator.Response{User: sa.UserInfo(), Audiences: auds}, true, nil
}

func validateSATimes(claims *saClaims, now time.Time) error {
	if claims.ExpiresAt == nil || now.After(claims.ExpiresAt.Add(saLeeway)) {
		return errors.New("service account token has expired")
	}
	if claims.NotBefore != nil && now.Add(saLeeway).Before(claims.NotBefore.Time) {
		return errors.New("service account token is not valid yet")
	}
	if claims.IssuedAt != nil && now.Add(saLeeway).Before(claims.IssuedAt.Time) {
		return errors.New("service account token is issued in the future")
	}
	return nil
}

func tokenAudiences(ctx context.Context, claimed []string) (authenticator.Audiences, error) {
	tokenAuds := authenticator.Audiences(claimed)
	if len(tokenAuds) == 0 {
		tokenAuds = APIAudiences
	}
	requested, ok := authenticator.AudiencesFrom(ctx)
	if !ok {
		requested = APIAudiences
	}
	auds := tokenAuds.Intersect(requested)
	if len(auds) == 0 {
		return nil, fmt.Errorf("token audiences %q is invalid for the target audiences %q", tokenAuds, requested)
	}
	return auds, nil
}

var errSATokenInvalidated = errors.New("service account token has been invalidated")

func deletedBefore(deleted *metav1.Time, now time.Time) bool {
	return deleted != nil && deleted.Time.Before(now.Add(-saLeeway))
}

func (s ServiceAccountToken) validateObjects(ctx context.Context, claims *saClaims) error {
	if s.Objects == nil {
		return errors.New("service account lookups are not configured")
	}
	now := time.Now()
	ns := claims.Kubernetes.Namespace
	saRef := claims.Kubernetes.ServiceAccount
	sa, err := s.Objects.ServiceAccount(ctx, ns, saRef.Name)
	if err != nil {
		return err
	}
	if string(sa.UID) != saRef.UID {
		return fmt.Errorf("service account UID (%s) does not match claim (%s)", sa.UID, saRef.UID)
	}
	if deletedBefore(sa.DeletionTimestamp, now) {
		return fmt.Errorf("service account %s/%s has been deleted", ns, saRef.Name)
	}
	if ref := claims.Kubernetes.Secret; ref != nil {
		secret, err := s.Objects.Secret(ctx, ns, ref.Name)
		if err != nil {
			return errSATokenInvalidated
		}
		if string(secret.UID) != ref.UID {
			return fmt.Errorf("secret UID (%s) does not match service account secret ref claim (%s)", secret.UID, ref.UID)
		}
		if deletedBefore(secret.DeletionTimestamp, now) {
			return errSATokenInvalidated
		}
	}
	pod := claims.Kubernetes.Pod
	if pod != nil {
		obj, err := s.Objects.Pod(ctx, ns, pod.Name)
		if err != nil {
			return errSATokenInvalidated
		}
		if string(obj.UID) != pod.UID {
			return fmt.Errorf("pod UID (%s) does not match service account pod ref claim (%s)", obj.UID, pod.UID)
		}
		if deletedBefore(obj.DeletionTimestamp, now) {
			return errSATokenInvalidated
		}
	}
	if ref := claims.Kubernetes.Node; ref != nil && pod == nil {
		node, err := s.Objects.Node(ctx, ref.Name)
		if err != nil {
			return errSATokenInvalidated
		}
		if string(node.UID) != ref.UID {
			return fmt.Errorf("node UID (%s) does not match service account node ref claim (%s)", node.UID, ref.UID)
		}
		if deletedBefore(node.DeletionTimestamp, now) {
			return errSATokenInvalidated
		}
	}
	return nil
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
