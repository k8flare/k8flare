package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
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

type ReadonlyToken string

func (t ReadonlyToken) AuthenticateToken(_ context.Context, token string) (*authenticator.Response, bool, error) {
	if t == "" || subtle.ConstantTimeCompare([]byte(token), []byte(t)) != 1 {
		return nil, false, nil
	}
	return &authenticator.Response{User: &user.DefaultInfo{Name: "readonly", Groups: []string{user.AllAuthenticated}}}, true, nil
}

type VaultToken struct{ Vault *supervisor.Vault }

func (t VaultToken) AuthenticateToken(ctx context.Context, token string) (*authenticator.Response, bool, error) {
	if t.Vault == nil || token == "" {
		return nil, false, nil
	}
	if t.Vault.CheckToken(ctx, "admin", token) == nil {
		return &authenticator.Response{User: &user.DefaultInfo{Name: "admin", Groups: []string{user.SystemPrivilegedGroup, user.AllAuthenticated}}}, true, nil
	}
	if t.Vault.CheckToken(ctx, "readonly", token) == nil {
		return &authenticator.Response{User: &user.DefaultInfo{Name: "readonly", Groups: []string{user.AllAuthenticated}}}, true, nil
	}
	return nil, false, nil
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

func WithAuth(next http.Handler, requests authenticator.Request) http.Handler {
	return WithAuthentication(WithImpersonation(next), requests, nil)
}

func WithAuthentication(next http.Handler, requests authenticator.Request, decorateFailed func(http.Handler) http.Handler) http.Handler {
	failed := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responsewriters.ErrorNegotiated(apierrors.NewUnauthorized("Unauthorized"), scheme.Codecs, schema.GroupVersion{}, w, r)
	}))
	if decorateFailed != nil {
		failed = decorateFailed(failed)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, ok, err := requests.AuthenticateRequest(r)
		if err != nil {
			failed.ServeHTTP(w, r)
			return
		}
		if !ok {
			resp = &authenticator.Response{User: &user.DefaultInfo{Name: user.Anonymous, Groups: []string{user.AllUnauthenticated}}}
		}
		next.ServeHTTP(w, r.WithContext(genericapirequest.WithUser(r.Context(), resp.User)))
	})
}

func WithImpersonation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current, _ := genericapirequest.UserFrom(r.Context())
		u, err := impersonate(current, r)
		if err != nil {
			responsewriters.ErrorNegotiated(apierrors.NewForbidden(schema.GroupResource{Resource: "users"}, "", err), scheme.Codecs, schema.GroupVersion{}, w, r)
			return
		}
		if u != current {
			audit.LogImpersonatedUser(r.Context(), u, "")
		}
		next.ServeHTTP(w, r.WithContext(genericapirequest.WithUser(r.Context(), u)))
	})
}

func impersonate(u user.Info, r *http.Request) (user.Info, error) {
	want := r.Header.Get("Impersonate-User")
	if want == "" {
		return u, nil
	}
	privileged := false
	for _, g := range u.GetGroups() {
		if g == user.SystemPrivilegedGroup {
			privileged = true
			break
		}
	}
	if !privileged {
		return nil, fmt.Errorf("user %q cannot impersonate", u.GetName())
	}
	groups := impersonateGroups(r.Header.Values("Impersonate-Group"))
	if len(groups) == 0 {
		groups = []string{user.AllAuthenticated}
	}
	return &user.DefaultInfo{Name: want, UID: r.Header.Get("Impersonate-Uid"), Groups: groups}, nil
}

func impersonateGroups(raw []string) []string {
	var groups []string
	for _, item := range raw {
		for _, part := range strings.Split(item, ",") {
			name := strings.TrimSpace(part)
			if name != "" {
				groups = append(groups, name)
			}
		}
	}
	return groups
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

func WithAuthorization(next http.Handler, a authorizer.Authorizer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		attrs, err := authorizerAttributes(r)
		if err != nil {
			responsewriters.ErrorNegotiated(apierrors.NewInternalError(err), scheme.Codecs, schema.GroupVersion{}, w, r)
			return
		}
		decision, reason, err := a.Authorize(ctx, attrs)
		if decision != authorizer.DecisionAllow {
			if err != nil {
				reason = err.Error()
			}
			audit.AddAuditAnnotations(ctx, "authorization.k8s.io/decision", "forbid", "authorization.k8s.io/reason", reason)
			responsewriters.Forbidden(attrs, w, r, reason, scheme.Codecs)
			return
		}
		audit.AddAuditAnnotations(ctx, "authorization.k8s.io/decision", "allow", "authorization.k8s.io/reason", reason)
		next.ServeHTTP(w, r)
	})
}

func authorizerAttributes(r *http.Request) (authorizer.Attributes, error) {
	ctx := r.Context()
	attrs := authorizer.AttributesRecord{}
	if u, ok := genericapirequest.UserFrom(ctx); ok {
		attrs.User = u
	}
	info, ok := genericapirequest.RequestInfoFrom(ctx)
	if !ok {
		return nil, fmt.Errorf("no RequestInfo in context")
	}
	attrs.ResourceRequest = info.IsResourceRequest
	attrs.Path = info.Path
	attrs.Verb = info.Verb
	attrs.APIGroup = info.APIGroup
	attrs.APIVersion = info.APIVersion
	attrs.Resource = info.Resource
	attrs.Subresource = info.Subresource
	attrs.Namespace = info.Namespace
	attrs.Name = info.Name
	if raw := r.URL.Query().Get("fieldSelector"); raw != "" {
		sel, err := fields.ParseSelector(raw)
		if err != nil {
			attrs.FieldSelectorParsingErr = err
		} else {
			attrs.FieldSelectorRequirements = sel.Requirements()
		}
	}
	if raw := r.URL.Query().Get("labelSelector"); raw != "" {
		sel, err := labels.Parse(raw)
		if err != nil {
			attrs.LabelSelectorParsingErr = err
		} else {
			if reqs, ok := sel.Requirements(); ok {
				attrs.LabelSelectorRequirements = reqs
			}
		}
	}
	return attrs, nil
}

func WithRemoteUser(next http.Handler) http.Handler {
	return WithRequestInfo(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := &user.DefaultInfo{Name: r.Header.Get("X-Remote-User"), Groups: remoteGroups(r.Header)}
		next.ServeHTTP(w, r.WithContext(genericapirequest.WithUser(r.Context(), u)))
	}))
}

func ForwardRemoteUser(ctx context.Context, h http.Header) {
	h.Del("Authorization")
	h.Del("X-Remote-User")
	h.Del("X-Remote-Group")
	h.Del(ClientCertHeader)
	if u, ok := genericapirequest.UserFrom(ctx); ok {
		h.Set("X-Remote-User", u.GetName())
		for _, g := range u.GetGroups() {
			h.Add("X-Remote-Group", g)
		}
	}
}

func remoteGroups(h http.Header) []string {
	var groups []string
	for _, v := range h.Values("X-Remote-Group") {
		for _, g := range strings.Split(v, ",") {
			if g = strings.TrimSpace(g); g != "" {
				groups = append(groups, g)
			}
		}
	}
	return groups
}
