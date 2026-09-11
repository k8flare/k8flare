package apiserver

import (
	"context"
	"io"
	"net/http"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/serviceaccount"
)

// ServiceAccount TokenRequest + JWT authentication, built from upstream's
// real signing/validation code (k8s.io/kubernetes/pkg/serviceaccount --
// the exact package kube-apiserver's own --service-account-signing-key-file
// uses), not a reimplementation (inviolable rule #3). The signing key is
// an ECDSA keypair persisted in the ca-vault facet alongside the existing
// client/server CAs (certmanager.go).
//
// Only Pod-bound tokens are supported (BoundObjectRef.Kind == "Pod" or
// unset) -- the shape kubelet itself requests for a projected
// serviceAccountToken volume, and the common case generally. Secret/Node
// bound tokens are not implemented; requesting one is a 400, not a
// silently-wrong token.

const defaultSATokenExpirationSeconds = 3600 * 24 // 1 day, matches kubelet's default requested duration

// saTokenGetter adapts this apiserver's ResourceStores to
// serviceaccount.ServiceAccountTokenGetter, the interface both token
// generation (bound-object existence, implicitly via the caller already
// having fetched them) and validation (bound-object liveness, checked on
// every authenticated request) go through.
type saTokenGetter struct {
	stores map[string]*ResourceStore // corev1 stores: serviceaccounts, pods, secrets, nodes
}

func (g *saTokenGetter) GetServiceAccount(ctx context.Context, namespace, name string) (*corev1.ServiceAccount, error) {
	obj, err := g.stores["serviceaccounts"].Get(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	sa, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		return nil, &StatusError{Status: notFoundStatus(rServiceAccounts, name)}
	}
	return sa, nil
}

func (g *saTokenGetter) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	obj, err := g.stores["pods"].Get(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, &StatusError{Status: notFoundStatus(rPods, name)}
	}
	return pod, nil
}

func (g *saTokenGetter) GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	obj, err := g.stores["secrets"].Get(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil, &StatusError{Status: notFoundStatus(rSecrets, name)}
	}
	return secret, nil
}

func (g *saTokenGetter) GetNode(ctx context.Context, name string) (*corev1.Node, error) {
	obj, err := g.stores["nodes"].Get(ctx, "", name)
	if err != nil {
		return nil, err
	}
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil, &StatusError{Status: notFoundStatus(rNodes, name)}
	}
	return node, nil
}

// resource name constants for notFoundStatus -- matching apidef.Table's
// Resource field for these types.
const (
	rServiceAccounts = "serviceaccounts"
	rPods            = "pods"
	rSecrets         = "secrets"
	rNodes           = "nodes"
)

func minimalServiceAccount(sa *corev1.ServiceAccount) core.ServiceAccount {
	return core.ServiceAccount{
		ObjectMeta: sa.ObjectMeta,
	}
}

func minimalPod(pod *corev1.Pod) *core.Pod {
	if pod == nil {
		return nil
	}
	return &core.Pod{ObjectMeta: pod.ObjectMeta}
}

// RegisterServiceAccountTokenHandler registers POST
// /api/v1/namespaces/{namespace}/serviceaccounts/{name}/token -- the
// TokenRequest subresource kubelet itself calls to mint projected
// ServiceAccount tokens for Pods, and anything else following the
// standard client-go TokenRequest flow.
func RegisterServiceAccountTokenHandler(mux *http.ServeMux, cam *CAManager, coreStores map[string]*ResourceStore, tokensFn TokensFunc) {
	getter := &saTokenGetter{stores: coreStores}
	mux.Handle(
		"POST /api/v1/namespaces/{namespace}/serviceaccounts/{name}/token",
		AuthMiddleware(tokensFn, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handleTokenRequest(w, r, cam, getter)
		})),
	)
}

func handleTokenRequest(w http.ResponseWriter, r *http.Request, cam *CAManager, getter *saTokenGetter) {
	if err := cam.Initialize(r.Context()); err != nil {
		writeStatusError(w, http.StatusInternalServerError, "InternalError", "initialize CA: "+err.Error())
		return
	}

	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
		return
	}
	// decodeBody auto-detects JSON vs protobuf -- client-go's typed
	// clientset sends CreateToken requests as protobuf by default.
	obj, err := decodeBody(body, nil)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode TokenRequest: "+err.Error())
		return
	}
	tr, ok := obj.(*authenticationv1.TokenRequest)
	if !ok {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "request body is not a TokenRequest")
		return
	}

	sa, err := getter.GetServiceAccount(r.Context(), namespace, name)
	if err != nil {
		writeStatusError(w, http.StatusNotFound, "NotFound", "serviceaccount not found: "+err.Error())
		return
	}

	expirationSeconds := int64(defaultSATokenExpirationSeconds)
	if tr.Spec.ExpirationSeconds != nil {
		expirationSeconds = *tr.Spec.ExpirationSeconds
	}

	var pod *core.Pod
	if ref := tr.Spec.BoundObjectRef; ref != nil {
		if ref.Kind != "" && ref.Kind != "Pod" {
			writeStatusError(w, http.StatusBadRequest, "BadRequest",
				"only Pod-bound tokens are supported (got boundObjectRef.kind="+ref.Kind+")")
			return
		}
		podObj, err := getter.GetPod(r.Context(), namespace, ref.Name)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "bound pod not found: "+err.Error())
			return
		}
		pod = minimalPod(podObj)
	}

	claims, privateClaims, err := serviceaccount.Claims(
		minimalServiceAccount(sa), pod, nil, nil, expirationSeconds, 0, tr.Spec.Audiences,
	)
	if err != nil {
		writeStatusError(w, http.StatusInternalServerError, "InternalError", "build claims: "+err.Error())
		return
	}

	generator, err := serviceaccount.JWTTokenGenerator(saTokenIssuer, cam.SAJWTSigningKey())
	if err != nil {
		writeStatusError(w, http.StatusInternalServerError, "InternalError", "build token generator: "+err.Error())
		return
	}
	token, err := generator.GenerateToken(r.Context(), claims, privateClaims)
	if err != nil {
		writeStatusError(w, http.StatusInternalServerError, "InternalError", "generate token: "+err.Error())
		return
	}

	tr.Status = authenticationv1.TokenRequestStatus{
		Token:               token,
		ExpirationTimestamp: metav1.NewTime(claims.Expiry.Time()),
	}
	tr.TypeMeta = metav1.TypeMeta{Kind: "TokenRequest", APIVersion: "authentication.k8s.io/v1"}
	writeRuntimeObject(w, http.StatusCreated, tr)
}

// saTokenIssuer is the "iss" claim -- an opaque identifier, not a real
// URL (this apiserver has no OIDC discovery document to back it, and
// nothing here validates issuers against a JWKS endpoint the way a real
// external consumer would).
const saTokenIssuer = "https://k8flare.internal"

// lazyServiceAccountAuthenticator defers CAManager.Initialize (and
// therefore the signing-key storage read) to the first actual
// authentication attempt -- AuthMiddleware only reaches this path when
// the presented bearer token already failed the cluster-token check, so
// the common case (admin token) never pays this cost, matching every
// other CAManager caller's lazy-init convention (supervisor.go).
type lazyServiceAccountAuthenticator struct {
	cam    *CAManager
	getter *saTokenGetter
}

func (a *lazyServiceAccountAuthenticator) AuthenticateToken(ctx context.Context, token string) (*authenticator.Response, bool, error) {
	if err := a.cam.Initialize(ctx); err != nil {
		return nil, false, err
	}
	keysGetter, err := serviceaccount.StaticPublicKeysGetter([]interface{}{a.cam.SAJWTSigningKey().Public()})
	if err != nil {
		return nil, false, err
	}
	validator := serviceaccount.NewValidator(a.getter)
	inner := serviceaccount.JWTTokenAuthenticator([]string{saTokenIssuer}, keysGetter, nil, validator)
	return inner.AuthenticateToken(ctx, token)
}

// NewServiceAccountTokenAuthenticator builds the real upstream JWT
// authenticator (validates signature, expiry, audience, and bound-object
// liveness against live stores) for use as the second identity source in
// AuthMiddleware, alongside the cluster token.
func NewServiceAccountTokenAuthenticator(cam *CAManager, coreStores map[string]*ResourceStore) authenticator.Token {
	return &lazyServiceAccountAuthenticator{cam: cam, getter: &saTokenGetter{stores: coreStores}}
}

// AuthenticateServiceAccountToken runs a bearer token through the real
// JWT authenticator. Returns (userInfo, true) on success -- callers
// treat this as a second identity source after the cluster token check
// fails, so an ordinary malformed/foreign bearer token here is just "not
// a match", not an error to surface.
func AuthenticateServiceAccountToken(ctx context.Context, authn authenticator.Token, bearerToken string) (*UserInfo, bool) {
	resp, ok, err := authn.AuthenticateToken(ctx, bearerToken)
	if err != nil || !ok || resp == nil {
		return nil, false
	}
	// serviceaccount.ServiceAccountInfo.UserInfo() (real upstream code)
	// attaches system:serviceaccounts and system:serviceaccounts:<ns> but
	// not system:authenticated -- real kube-apiserver adds that via a
	// separate union-authenticator wrapping layer we don't have, so it's
	// added here instead (every other identity source in auth.go already
	// includes it directly).
	groups := append(resp.User.GetGroups(), "system:authenticated")
	return &UserInfo{Name: resp.User.GetName(), Groups: groups}, true
}
