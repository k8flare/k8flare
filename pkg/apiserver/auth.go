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
// SetServiceAccountAuthenticator once CAManager.Initialize has run -- a
// settable-func-var, for the same reason (this apiserver's per-request
// instantiation means the key is only readable during request handling,
// not at package init). Nil
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

// TokenRole is how much a cluster token is allowed to be. Until
// 2026-09-13 there was only one: every valid token authenticated as
// system:masters, so reading a node's config file made you a cluster
// administrator (TODO.md P0-8).
type TokenRole string

const (
	// RoleAdmin is the cluster's root credential, and the role of every
	// token that carries none -- which is what keeps vaults written
	// before roles existed, the K3S_TOKEN secret and the dev fallback
	// working unchanged.
	RoleAdmin TokenRole = "admin"
	// RoleAgent is a node's credential: system:nodes and nothing else.
	RoleAgent TokenRole = "agent"
)

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
//
// An element is either a bare secret (RoleAdmin) or TokenEntry(role,
// secret). The role rides INSIDE the list rather than in a lookup beside
// it because the list is the unit callers refresh atomically: a
// separately-refreshed role table would, for the length of one cache
// window, answer "no role recorded" -- i.e. administrator -- about a
// token the cached list still accepts.
type TokensFunc func() []string

// tokenEntrySep separates an encoded entry's role from its secret. Only
// two things build a token list -- DecodeVaultTokens, which encodes the
// role explicitly, and cmd/apiserver-wasm, which appends the K3S_TOKEN
// secret and the dev fallback verbatim -- so no bare secret is read as
// role-carrying unless it contains a NUL itself, and such a secret fails
// closed (an unrecognized role, 401) rather than escalating.
const tokenEntrySep = "\x00"

// TokenEntry encodes one role-carrying token for a TokensFunc list.
func TokenEntry(role TokenRole, secret string) string {
	return string(role) + tokenEntrySep + secret
}

func splitTokenEntry(entry string) (TokenRole, string) {
	sep := strings.Index(entry, tokenEntrySep)
	if sep < 0 {
		return RoleAdmin, entry
	}
	return TokenRole(entry[:sep]), entry[sep+len(tokenEntrySep):]
}

// matchToken reports the role of the entry whose secret equals
// presented, comparing in constant time per candidate.
func matchToken(tokens []string, presented string) (TokenRole, bool) {
	var role TokenRole
	ok := false
	for _, t := range tokens {
		r, secret := splitTokenEntry(t)
		if len(secret) == len(presented) && subtle.ConstantTimeCompare([]byte(secret), []byte(presented)) == 1 {
			role, ok = r, true
		}
	}
	return role, ok
}

// tokenMatches reports whether presented equals ANY currently-valid
// token, whatever its role.
func tokenMatches(tokens []string, presented string) bool {
	_, ok := matchToken(tokens, presented)
	return ok
}

func nodeUser() *UserInfo {
	return &UserInfo{Name: "node", Groups: []string{"k3s:agent", "system:nodes", "system:authenticated"}}
}

func adminUser() *UserInfo {
	return &UserInfo{Name: "admin", Groups: []string{"system:masters", "system:authenticated"}}
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
			role, matched := matchToken(tokens, bearerToken)
			switch {
			case matched && role == RoleAdmin:
				authenticated = true
				// When X-Remote-User is present (set by TLS proxy from client cert),
				// use it as the identity instead of the default admin user.
				// Administrators only: the gateway strips inbound
				// X-Remote-* now, but a token that names its own identity
				// is an escalation the moment roles exist, so the two
				// defenses are not allowed to depend on each other.
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
			case matched && role == RoleAgent:
				authenticated = true
				user = nodeUser()
			case matched:
				// A role this build doesn't know fails closed. Anything
				// else would make "admin" the default for a vault written
				// by a NEWER control plane.
			case currentSAAuthenticator != nil:
				// Not a cluster token -- try it as a real
				// ServiceAccount JWT (TokenRequest, serviceaccounttoken.go).
				if u, ok := AuthenticateServiceAccountToken(r.Context(), currentSAAuthenticator, bearerToken); ok {
					authenticated = true
					user = u
				}
			}
		}

		// Check Basic Auth (k3s agent sends node:<password>)
		if !authenticated {
			if username, password, ok := r.BasicAuth(); ok {
				role, matched := matchToken(tokens, password)
				switch {
				case matched && role == RoleAdmin:
					authenticated = true
					if username == "node" {
						user = nodeUser()
					} else {
						user = &UserInfo{Name: username, Groups: []string{"system:masters", "system:authenticated"}}
					}
				case matched && role == RoleAgent:
					// The username is the caller's own claim, so an agent
					// token gets the node identity whatever it says.
					authenticated = true
					user = nodeUser()
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
