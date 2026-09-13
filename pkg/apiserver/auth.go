package apiserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

// Authenticator resolves a bearer token to a user, or nil when unknown.
type Authenticator func(ctx context.Context, token string) *user.DefaultInfo

func adminAuthenticator(adminToken string) Authenticator {
	return func(_ context.Context, token string) *user.DefaultInfo {
		if adminToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) == 1 {
			return &user.DefaultInfo{Name: "admin", Groups: []string{user.SystemPrivilegedGroup, user.AllAuthenticated}}
		}
		return nil
	}
}

func writeStatus(w http.ResponseWriter, code int, reason, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"kind": "Status", "apiVersion": "v1", "metadata": map[string]any{}, "status": "Failure",
		"message": message, "reason": reason, "code": code,
	})
}

// withAuth requires a bearer token every authenticator knows. Every
// authenticated user is authorized for everything; RBAC comes later.
func withAuth(next http.Handler, authenticators ...Authenticator) http.Handler {
	resolver := &genericapirequest.RequestInfoFactory{APIPrefixes: sets.NewString("api", "apis"), GrouplessAPIPrefixes: sets.NewString("api")}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeStatus(w, http.StatusUnauthorized, "Unauthorized", "Unauthorized")
			return
		}
		for _, a := range authenticators {
			u := a(r.Context(), token)
			if u == nil {
				continue
			}
			info, err := resolver.NewRequestInfo(r)
			if err != nil {
				writeStatus(w, http.StatusBadRequest, "BadRequest", err.Error())
				return
			}
			ctx := genericapirequest.WithRequestInfo(genericapirequest.WithUser(r.Context(), u), info)
			ctx = audit.WithAuditContext(ctx)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		writeStatus(w, http.StatusUnauthorized, "Unauthorized", "Unauthorized")
	})
}
