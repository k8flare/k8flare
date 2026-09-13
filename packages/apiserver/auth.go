package apiserver

import (
	"context"
	"crypto/subtle"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/bearertoken"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/kubernetes/scheme"
)

// adminToken authenticates kubectl.
type adminToken string

func (t adminToken) AuthenticateToken(_ context.Context, token string) (*authenticator.Response, bool, error) {
	if t == "" || subtle.ConstantTimeCompare([]byte(token), []byte(t)) != 1 {
		return nil, false, nil
	}
	return &authenticator.Response{User: &user.DefaultInfo{Name: "admin", Groups: []string{user.SystemPrivilegedGroup, user.AllAuthenticated}}}, true, nil
}

// nodeToken accepts the token packages/agent writes into the kubelet's
// kubeconfig, "node:<name>:<node password>": the same secret the supervisor
// verified when it signed the node's certificates. It never registers a
// node; only the supervisor does.
type nodeToken struct{ vault *supervisor.Vault }

func (n nodeToken) AuthenticateToken(ctx context.Context, token string) (*authenticator.Response, bool, error) {
	rest, ok := strings.CutPrefix(token, "node:")
	if !ok {
		return nil, false, nil
	}
	name, password, ok := strings.Cut(rest, ":")
	if !ok || name == "" || password == "" {
		return nil, false, nil
	}
	if err := n.vault.CheckNodePassword(ctx, name, password); err != nil {
		return nil, false, nil
	}
	return &authenticator.Response{User: &user.DefaultInfo{Name: "system:node:" + name, Groups: []string{user.NodesGroup, user.AllAuthenticated}}}, true, nil
}

// withAuth requires a bearer token one of the authenticators knows. Every
// authenticated user is authorized for everything; RBAC comes later.
func withAuth(next http.Handler, tokens authenticator.Token) http.Handler {
	requests := bearertoken.New(tokens)
	resolver := &genericapirequest.RequestInfoFactory{APIPrefixes: sets.NewString("api", "apis"), GrouplessAPIPrefixes: sets.NewString("api")}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, ok, err := requests.AuthenticateRequest(r)
		if err != nil || !ok {
			responsewriters.ErrorNegotiated(apierrors.NewUnauthorized("Unauthorized"), scheme.Codecs, schema.GroupVersion{}, w, r)
			return
		}
		info, err := resolver.NewRequestInfo(r)
		if err != nil {
			responsewriters.ErrorNegotiated(apierrors.NewBadRequest(err.Error()), scheme.Codecs, schema.GroupVersion{}, w, r)
			return
		}
		ctx := genericapirequest.WithRequestInfo(genericapirequest.WithUser(r.Context(), resp.User), info)
		next.ServeHTTP(w, r.WithContext(audit.WithAuditContext(ctx)))
	})
}
