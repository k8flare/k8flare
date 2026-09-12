package apiserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Hermetic: AuthMiddleware is exercised in-process, so this file must
// never reach for setupWranglerDev (apiserver_test.go).

const (
	testAdminSecret = "admin-secret-0000"
	testAgentSecret = "agent-secret-1111"
)

func testVaultTokens() []string {
	return []string{
		testAdminSecret,
		TokenEntry(RoleAgent, testAgentSecret),
		"k8flare-dev-token",
	}
}

// authenticate runs one request through AuthMiddleware and reports the
// identity it established, or nil when the request was rejected.
func authenticate(t *testing.T, tokens []string, req *http.Request) *UserInfo {
	t.Helper()
	// The SA-JWT authenticator is a package-level var another test in
	// this binary may have installed; these cases are about cluster
	// tokens only.
	saved := currentSAAuthenticator
	currentSAAuthenticator = nil
	defer func() { currentSAAuthenticator = saved }()

	var got *UserInfo
	handler := AuthMiddleware(func() []string { return tokens }, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = UserFromContext(r.Context())
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		return nil
	}
	if got == nil {
		t.Fatalf("%s: authenticated with no UserInfo in the context (status %d)", req.URL.Path, rec.Code)
	}
	return got
}

func hasGroup(u *UserInfo, group string) bool {
	for _, g := range u.Groups {
		if g == group {
			return true
		}
	}
	return false
}

func bearerRequest(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/pods", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func basicRequest(username, password string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/pods", nil)
	req.SetBasicAuth(username, password)
	return req
}

func TestAgentTokenIsNodeOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  *http.Request
	}{
		{"bearer", bearerRequest(testAgentSecret)},
		// The k3s agent's own join shape.
		{"basic as node", basicRequest("node", testAgentSecret)},
		// The username is the caller's own claim, so it must not pick
		// the identity.
		{"basic as someone else", basicRequest("alice", testAgentSecret)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := authenticate(t, testVaultTokens(), tc.req)
			if user == nil {
				t.Fatal("agent token rejected; it must still authenticate")
			}
			if hasGroup(user, "system:masters") {
				t.Fatalf("agent token authenticated as %q with groups %v: system:masters is the whole defect", user.Name, user.Groups)
			}
			if !hasGroup(user, "system:nodes") {
				t.Fatalf("agent token groups %v: want system:nodes", user.Groups)
			}
			if user.Name != "node" {
				t.Fatalf("agent token name %q, want %q", user.Name, "node")
			}
		})
	}
}

func TestAgentTokenCannotNameItselfWithRemoteHeaders(t *testing.T) {
	req := bearerRequest(testAgentSecret)
	req.Header.Set("X-Remote-User", "impersonated-admin")
	req.Header.Set("X-Remote-Group", "system:masters")

	user := authenticate(t, testVaultTokens(), req)
	if user == nil {
		t.Fatal("agent token rejected; it must still authenticate")
	}
	if user.Name != "node" || hasGroup(user, "system:masters") {
		t.Fatalf("agent token + X-Remote-* authenticated as %q with groups %v, want the node identity", user.Name, user.Groups)
	}
}

func TestAdminTokenIsUnchanged(t *testing.T) {
	tokens := testVaultTokens()

	// Bearer, no headers: the root identity.
	user := authenticate(t, tokens, bearerRequest(testAdminSecret))
	if user == nil || user.Name != "admin" || !hasGroup(user, "system:masters") {
		t.Fatalf("admin bearer: got %+v, want admin/system:masters", user)
	}

	// Bearer + X-Remote-*: an administrator may still hand the apiserver
	// a derived identity (the TLS-proxy path).
	req := bearerRequest(testAdminSecret)
	req.Header.Set("X-Remote-User", "alice")
	req.Header.Set("X-Remote-Group", "developers")
	user = authenticate(t, tokens, req)
	if user == nil || user.Name != "alice" {
		t.Fatalf("admin bearer + X-Remote-User: got %+v, want alice", user)
	}
	if !hasGroup(user, "developers") || !hasGroup(user, "system:authenticated") || hasGroup(user, "system:masters") {
		t.Fatalf("admin bearer + X-Remote-Group: groups %v, want [developers system:authenticated]", user.Groups)
	}

	// Basic: "node" is the k3s agent's username, anything else is the
	// administrator itself.
	user = authenticate(t, tokens, basicRequest("node", testAdminSecret))
	if user == nil || user.Name != "node" || !hasGroup(user, "system:nodes") {
		t.Fatalf("admin basic as node: got %+v, want the node identity", user)
	}
	user = authenticate(t, tokens, basicRequest("alice", testAdminSecret))
	if user == nil || user.Name != "alice" || !hasGroup(user, "system:masters") {
		t.Fatalf("admin basic as alice: got %+v, want alice/system:masters", user)
	}
}

// The secretless dev/CI posture (CLAUDE.md): every Go test lane and
// `wrangler dev` authenticate with this token and expect to be root.
func TestDevFallbackTokenIsAdmin(t *testing.T) {
	for _, req := range []*http.Request{
		bearerRequest("k8flare-dev-token"),
		basicRequest("kubectl", "k8flare-dev-token"),
	} {
		user := authenticate(t, []string{"k8flare-dev-token"}, req)
		if user == nil || !hasGroup(user, "system:masters") {
			t.Fatalf("dev fallback token: got %+v, want system:masters", user)
		}
	}
	if user := authenticate(t, testVaultTokens(), basicRequest("node", "k8flare-dev-token")); user == nil || user.Name != "node" {
		t.Fatalf("dev fallback token as node: got %+v, want the node identity", user)
	}
}

func TestUnknownAndUnrecognizedTokensAreRejected(t *testing.T) {
	if user := authenticate(t, testVaultTokens(), bearerRequest("not-a-token")); user != nil {
		t.Fatalf("unknown token authenticated as %+v", user)
	}
	// A role a newer control plane wrote and this build doesn't know
	// must fail closed rather than fall back to the administrator it
	// used to be.
	future := []string{TokenEntry(TokenRole("audit"), "future-secret")}
	if user := authenticate(t, future, bearerRequest("future-secret")); user != nil {
		t.Fatalf("unrecognized role authenticated as %+v", user)
	}
	if user := authenticate(t, future, basicRequest("node", "future-secret")); user != nil {
		t.Fatalf("unrecognized role authenticated over Basic as %+v", user)
	}
}

// End to end from the stored vault: what clusters/tokens.ts writes has
// to arrive at AuthMiddleware as the role it was written with.
func TestVaultRolesReachTheIdentity(t *testing.T) {
	vault := `{"tokens":[` +
		`{"tokenId":"a","secret":"vault-admin","createdAt":"2026-09-13T00:00:00Z"},` +
		`{"tokenId":"b","secret":"vault-agent","createdAt":"2026-09-13T00:00:00Z","role":"agent"},` +
		`{"tokenId":"c","secret":"vault-future","createdAt":"2026-09-13T00:00:00Z","role":"audit"}]}`
	body := `{"kv":{"value":"` + base64.StdEncoding.EncodeToString([]byte(vault)) + `"}}`

	tokens, err := DecodeVaultTokens(strings.NewReader(body))
	if err != nil {
		t.Fatalf("DecodeVaultTokens: %v", err)
	}
	// A role-less entry stays a plain secret, which is what keeps every
	// vault written before roles existed working untouched.
	if tokens[0] != "vault-admin" {
		t.Fatalf("role-less entry decoded to %q, want the bare secret", tokens[0])
	}

	if user := authenticate(t, tokens, bearerRequest("vault-admin")); user == nil || !hasGroup(user, "system:masters") {
		t.Fatalf("vault admin token: got %+v, want system:masters", user)
	}
	user := authenticate(t, tokens, bearerRequest("vault-agent"))
	if user == nil || hasGroup(user, "system:masters") || !hasGroup(user, "system:nodes") {
		t.Fatalf("vault agent token: got %+v, want system:nodes without system:masters", user)
	}
	if user := authenticate(t, tokens, bearerRequest("vault-future")); user != nil {
		t.Fatalf("vault token with an unrecognized role: got %+v, want a rejection", user)
	}
}

// An agent token must review back as a node. Found by review: the role split
// covered AuthMiddleware and not this endpoint, so a kubelet could ask the
// apiserver who it was and be told system:masters -- which made the whole
// split decorative for anyone holding a node's config file.
func TestTokenReviewAnswersTheTokensOwnRole(t *testing.T) {
	tokens := func() []string {
		return []string{"admin-secret", TokenEntry(RoleAgent, "agent-secret")}
	}
	mux := http.NewServeMux()
	RegisterAuthenticationHandlers(mux, tokens)

	review := func(secret string) authenticationv1.TokenReviewStatus {
		t.Helper()
		body, err := json.Marshal(&authenticationv1.TokenReview{
			TypeMeta: metav1.TypeMeta{APIVersion: "authentication.k8s.io/v1", Kind: "TokenReview"},
			Spec:     authenticationv1.TokenReviewSpec{Token: secret},
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/apis/authentication.k8s.io/v1/tokenreviews", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		// The endpoint is itself behind AuthMiddleware; the caller here is
		// the control plane asking about somebody else's token.
		req.Header.Set("Authorization", "Bearer admin-secret")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var out authenticationv1.TokenReview
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode %d %s: %v", rec.Code, rec.Body.String(), err)
		}
		return out.Status
	}

	if got := review("admin-secret"); !got.Authenticated || got.User.Username != "admin" {
		t.Errorf("admin token reviewed as %+v, want authenticated admin", got)
	}

	agent := review("agent-secret")
	if !agent.Authenticated {
		t.Fatalf("agent token not authenticated: %+v", agent)
	}
	if agent.User.Username != "node" {
		t.Errorf("agent token username = %q, want node", agent.User.Username)
	}
	for _, g := range agent.User.Groups {
		if g == "system:masters" {
			t.Errorf("agent token reviewed into system:masters, groups = %v", agent.User.Groups)
		}
	}
}
