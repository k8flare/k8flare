package apiserver

import (
	"context"
	"net/http"
	"strings"
)

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
// TokenFunc is a function that returns the current auth token.
// This allows lazy token loading from Workers environment bindings.
type TokenFunc func() string

func AuthMiddleware(tokenFn TokenFunc, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := tokenFn()
		authHeader := r.Header.Get("Authorization")
		authenticated := false
		var user *UserInfo

		// Check Bearer token (kubectl, existing clients)
		if strings.HasPrefix(authHeader, "Bearer ") {
			bearerToken := strings.TrimPrefix(authHeader, "Bearer ")
			if bearerToken == token {
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
					user = &UserInfo{Name: "admin", Groups: []string{"system:masters"}}
				}
			}
		}

		// Check Basic Auth (k3s agent sends node:<password>)
		if !authenticated {
			if username, password, ok := r.BasicAuth(); ok && password == token {
				authenticated = true
				if username == "node" {
					user = &UserInfo{Name: "node", Groups: []string{"k3s:agent", "system:nodes"}}
				} else {
					user = &UserInfo{Name: username, Groups: []string{"system:masters"}}
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
