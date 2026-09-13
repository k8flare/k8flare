package auth

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

type AdminToken string

func (t AdminToken) AuthenticateToken(_ context.Context, token string) (*authenticator.Response, bool, error) {
	if t == "" || subtle.ConstantTimeCompare([]byte(token), []byte(t)) != 1 {
		return nil, false, nil
	}
	return &authenticator.Response{User: &user.DefaultInfo{Name: "admin", Groups: []string{user.SystemPrivilegedGroup, user.AllAuthenticated}}}, true, nil
}

type NodeToken struct{ Vault *supervisor.Vault }

func (n NodeToken) AuthenticateToken(ctx context.Context, token string) (*authenticator.Response, bool, error) {
	rest, ok := strings.CutPrefix(token, "node:")
	if !ok {
		return nil, false, nil
	}
	name, password, ok := strings.Cut(rest, ":")
	if !ok || name == "" || password == "" {
		return nil, false, nil
	}
	if err := n.Vault.CheckNodePassword(ctx, name, password); err != nil {
		return nil, false, nil
	}
	return &authenticator.Response{User: &user.DefaultInfo{Name: "system:node:" + name, Groups: []string{user.NodesGroup, user.AllAuthenticated}}}, true, nil
}

func WithAuth(next http.Handler, tokens authenticator.Token) http.Handler {
	requests := bearertoken.New(tokens)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, ok, err := requests.AuthenticateRequest(r)
		if err != nil || !ok {
			responsewriters.ErrorNegotiated(apierrors.NewUnauthorized("Unauthorized"), scheme.Codecs, schema.GroupVersion{}, w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(genericapirequest.WithUser(r.Context(), resp.User)))
	})
}

var requestInfoResolver = &genericapirequest.RequestInfoFactory{APIPrefixes: sets.NewString("api", "apis"), GrouplessAPIPrefixes: sets.NewString("api")}

func WithRequestInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, err := requestInfoResolver.NewRequestInfo(r)
		if err != nil {
			responsewriters.ErrorNegotiated(apierrors.NewBadRequest(err.Error()), scheme.Codecs, schema.GroupVersion{}, w, r)
			return
		}
		ctx := genericapirequest.WithRequestInfo(r.Context(), info)
		next.ServeHTTP(w, r.WithContext(audit.WithAuditContext(ctx)))
	})
}

func WithRemoteUser(next http.Handler) http.Handler {
	return WithRequestInfo(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := &user.DefaultInfo{Name: r.Header.Get("X-Remote-User"), Groups: r.Header.Values("X-Remote-Group")}
		next.ServeHTTP(w, r.WithContext(genericapirequest.WithUser(r.Context(), u)))
	}))
}

func ForwardRemoteUser(ctx context.Context, h http.Header) {
	h.Del("Authorization")
	h.Del("X-Remote-User")
	h.Del("X-Remote-Group")
	if u, ok := genericapirequest.UserFrom(ctx); ok {
		h.Set("X-Remote-User", u.GetName())
		for _, g := range u.GetGroups() {
			h.Add("X-Remote-Group", g)
		}
	}
}
