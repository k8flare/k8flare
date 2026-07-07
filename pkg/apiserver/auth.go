package apiserver

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"k8s.io/apiserver/pkg/authentication/authenticator"
)

// currentSAAuthenticator is the real JWT ServiceAccount token
// authenticator (serviceaccounttoken.go), installed via
// SetServiceAccountAuthenticator once CAManager.Initialize has run --
// same settable-func-var pattern as r2.go's currentR2Config, for the
// same reason (this apiserver's per-request instantiation means the key
// is only readable during request handling, not at package init). Nil
// until installed, in which case AuthMiddleware simply never tries it
// (e.g. supervisor/discovery endpoints registered before main's SA
// authenticator wiring runs, or a build that never calls it).
var currentSAAuthenticator authenticator.Token

// SetServiceAccountAuthenticator installs authn as the second identity
// source AuthMiddleware tries when a presented bearer token isn't the
// cluster token.
func SetServiceAccountAuthenticator(authn authenticator.Token) {
	currentSAAuthenticator = authn
}

// contextKey is an unexported type used for context value keys to avoid collisions.
type contextKey int

const userInfoKey contextKey = 0

// UserInfo holds the authenticated user's identity.
type UserInfo struct {
	Name   string
	Groups []string
}

// statusResponse represents a Kubernetes Status response object.
type statusResponse struct {
	Kind       string            `json:"kind"`
	APIVersion string            `json:"apiVersion"`
	Metadata   map[string]string `json:"metadata"`
	Status     string            `json:"status"`
	Message    string            `json:"message"`
	Reason     string            `json:"reason"`
	Code       int               `json:"code"`
}

// AuthMiddleware returns an http.Handler that validates Bearer token or Basic Auth authentication.
// Bearer token is used by kubectl and other clients.
// Basic Auth is used by k3s agent (username="node", password=token).
// Requests with valid credentials proceed to the next handler with UserInfo set in the context.
// Requests without valid credentials receive a 401 Unauthorized response.
// TokensFunc returns every currently-valid cluster token (multi-cluster:
// the per-cluster vault holds several concurrently-valid tokens so
// rotation is possible; the zero-config default cluster returns exactly
// its env token). Lazy so Workers env bindings / storage are only read
// during request handling.
type TokensFunc func() []string

// tokenMatches reports whether presented equals ANY currently-valid
// token, in constant time per candidate.
func tokenMatches(tokens []string, presented string) bool {
	ok := false
	for _, t := range tokens {
		if len(t) == len(presented) && subtle.ConstantTimeCompare([]byte(t), []byte(presented)) == 1 {
			ok = true
		}
	}
	return ok
}

func AuthMiddleware(tokensFn TokensFunc, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens := tokensFn()
		authHeader := r.Header.Get("Authorization")
		authenticated := false
		var user *UserInfo

		// Check Bearer token (kubectl, existing clients)
		if strings.HasPrefix(authHeader, "Bearer ") {
			bearerToken := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenMatches(tokens, bearerToken) {
				authenticated = true
				// When X-Remote-User is present (set by TLS proxy from client cert),
				// use it as the identity instead of the default admin user.
				if remoteUser := r.Header.Get("X-Remote-User"); remoteUser != "" {
					groups := []string{"system:authenticated"}
					if remoteGroups := r.Header.Get("X-Remote-Group"); remoteGroups != "" {
						groups = strings.Split(remoteGroups, ",")
						groups = append(groups, "system:authenticated")
					}
					user = &UserInfo{Name: remoteUser, Groups: groups}
				} else {
					user = &UserInfo{Name: "admin", Groups: []string{"system:masters", "system:authenticated"}}
				}
			} else if currentSAAuthenticator != nil {
				// Not the cluster token -- try it as a real
				// ServiceAccount JWT (TokenRequest, serviceaccounttoken.go).
				if u, ok := AuthenticateServiceAccountToken(r.Context(), currentSAAuthenticator, bearerToken); ok {
					authenticated = true
					user = u
				}
			}
		}

		// Check Basic Auth (k3s agent sends node:<password>)
		if !authenticated {
			if username, password, ok := r.BasicAuth(); ok && tokenMatches(tokens, password) {
				authenticated = true
				if username == "node" {
					user = &UserInfo{Name: "node", Groups: []string{"k3s:agent", "system:nodes", "system:authenticated"}}
				} else {
					user = &UserInfo{Name: username, Groups: []string{"system:masters", "system:authenticated"}}
				}
			}
		}

		if !authenticated {
			writeJSON(w, http.StatusUnauthorized, statusResponse{
				Kind:       "Status",
				APIVersion: "v1",
				Metadata:   map[string]string{},
				Status:     "Failure",
				Message:    "Unauthorized",
				Reason:     "Unauthorized",
				Code:       401,
			})
			return
		}

		ctx := context.WithValue(r.Context(), userInfoKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserFromContext extracts the UserInfo from the request context.
// Returns nil if no user information is present.
func UserFromContext(ctx context.Context) *UserInfo {
	user, _ := ctx.Value(userInfoKey).(*UserInfo)
	return user
}
