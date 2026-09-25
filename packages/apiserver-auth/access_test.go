package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestAccessAuthenticateRequest(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	team := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cdn-cgi/access/certs" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(jwkSet{Keys: []jwk{{
			Kid: "k1",
			Kty: "RSA",
			N:   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(team.Close)
	jwksMu.Lock()
	jwksCache = map[string]jwksEntry{}
	jwksMu.Unlock()

	a := Access{Team: strings.TrimPrefix(strings.TrimPrefix(team.URL, "https://"), "http://"), Audience: "app-aud", HTTP: team.Client()}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, accessClaims{
		Email: "ada@kooffice.jp",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://" + a.Team,
			Audience:  jwt.ClaimStrings{"app-aud"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	tok.Header["kid"] = "k1"
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "https://api.k8flare.com/version", nil)
	req.Header.Set(accessAssertion, raw)
	resp, ok, err := a.AuthenticateRequest(req)
	if err != nil || !ok {
		t.Fatalf("auth: ok=%v err=%v", ok, err)
	}
	if resp.User.GetName() != "ada@kooffice.jp" {
		t.Fatalf("name: %s", resp.User.GetName())
	}
	if got := resp.User.GetGroups(); len(got) != 1 || got[0] != user.AllAuthenticated {
		t.Fatalf("groups: %v", got)
	}

	if _, ok, err := (Access{Audience: "app-aud", HTTP: team.Client()}).AuthenticateRequest(req); err != nil || ok {
		t.Fatalf("empty team: ok=%v err=%v", ok, err)
	}
	if _, ok, err := a.AuthenticateRequest(httptest.NewRequest(http.MethodGet, "/", nil)); err != nil || ok {
		t.Fatalf("missing header: ok=%v err=%v", ok, err)
	}

	resp, ok, err = a.AuthenticateToken(t.Context(), raw)
	if err != nil || !ok {
		t.Fatalf("bearer: ok=%v err=%v", ok, err)
	}
	if resp.User.GetName() != "ada@kooffice.jp" {
		t.Fatalf("bearer name: %s", resp.User.GetName())
	}
	if _, ok, err := a.AuthenticateToken(t.Context(), "not-a-jwt"); err != nil || ok {
		t.Fatalf("junk bearer: ok=%v err=%v", ok, err)
	}
}

func TestAccessIssuer(t *testing.T) {
	if got := accessIssuer("kooffice"); got != "https://kooffice.cloudflareaccess.com" {
		t.Fatalf("short: %s", got)
	}
	if got := accessIssuer("https://kooffice.cloudflareaccess.com/"); got != "https://kooffice.cloudflareaccess.com" {
		t.Fatalf("url: %s", got)
	}
}

func TestImpersonateRequiresPrivileged(t *testing.T) {
	admin := &user.DefaultInfo{Name: "admin", Groups: []string{user.SystemPrivilegedGroup}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Impersonate-User", "system:node:k8flare-c1")
	r.Header.Add("Impersonate-Group", user.NodesGroup)
	got, err := impersonate(admin, r)
	if err != nil || got.GetName() != "system:node:k8flare-c1" {
		t.Fatalf("admin: %v %v", got, err)
	}
	ro := &user.DefaultInfo{Name: "readonly", Groups: []string{user.AllAuthenticated}}
	if _, err := impersonate(ro, r); err == nil {
		t.Fatal("readonly impersonated")
	}
}

func TestImpersonateSplitsCombinedGroups(t *testing.T) {
	admin := &user.DefaultInfo{Name: "admin", Groups: []string{user.SystemPrivilegedGroup}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Impersonate-User", "system:node:k8flare-c1")
	r.Header.Set("Impersonate-Group", user.NodesGroup+", "+user.AllAuthenticated)
	got, err := impersonate(admin, r)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{user.NodesGroup, user.AllAuthenticated}
	if len(got.GetGroups()) != 2 || got.GetGroups()[0] != want[0] || got.GetGroups()[1] != want[1] {
		t.Fatalf("groups = %v", got.GetGroups())
	}
}
