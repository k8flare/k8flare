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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"k8s.io/apiserver/pkg/authentication/user"
)

func newAccessTeam(t *testing.T) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()
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
	jwksRefetchStarted = map[string]time.Time{}
	jwksMu.Unlock()
	return team, key
}

func TestAccessMapsGroupsClaim(t *testing.T) {
	team, key := newAccessTeam(t)
	sign := func(custom map[string]any) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, accessClaims{
			Email:  "ada@kooffice.jp",
			Custom: custom,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    team.URL,
				Audience:  jwt.ClaimStrings{"app-aud"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		})
		tok.Header["kid"] = "k1"
		raw, err := tok.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	host := strings.TrimPrefix(team.URL, "https://")
	cases := []struct {
		name   string
		access Access
		custom map[string]any
		want   []string
	}{
		{"default claim", Access{}, map[string]any{"groups": []any{"eng", "ops"}}, []string{"eng", "ops"}},
		{"prefix", Access{GroupsPrefix: "access:"}, map[string]any{"groups": []any{"eng"}}, []string{"access:eng"}},
		{"custom claim", Access{GroupsClaim: "roles"}, map[string]any{"roles": []any{"admin"}, "groups": []any{"eng"}}, []string{"admin"}},
		{"single string", Access{}, map[string]any{"groups": "eng"}, []string{"eng"}},
		{"system groups dropped", Access{}, map[string]any{"groups": []any{"system:masters", "eng"}}, []string{"eng"}},
		{"no claim", Access{}, nil, nil},
	}
	for _, c := range cases {
		a := c.access
		a.Team, a.Audience, a.HTTP = host, "app-aud", team.Client()
		resp, ok, err := a.AuthenticateToken(t.Context(), sign(c.custom))
		if err != nil || !ok {
			t.Fatalf("%s: ok=%v err=%v", c.name, ok, err)
		}
		want := append(append([]string{}, c.want...), user.AllAuthenticated)
		if got := resp.User.GetGroups(); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: groups=%v want %v", c.name, got, want)
		}
	}
}

func TestAccessAuthenticateRequest(t *testing.T) {
	team, key := newAccessTeam(t)
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

type countingTransport struct {
	next  http.RoundTripper
	count atomic.Int32
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.count.Add(1)
	return c.next.RoundTrip(r)
}

type gatheringTransport struct {
	countingTransport
	want int32
	all  chan struct{}
}

func (g *gatheringTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if g.count.Add(1) == g.want {
		close(g.all)
	}
	select {
	case <-g.all:
	case <-time.After(5 * time.Second):
	}
	return g.next.RoundTrip(r)
}

func signAccessToken(t *testing.T, team *httptest.Server, key *rsa.PrivateKey, kid string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, accessClaims{
		Email: "ada@kooffice.jp",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    team.URL,
			Audience:  jwt.ClaimStrings{"app-aud"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	tok.Header["kid"] = kid
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAccessRejectsUnknownKidFromCacheWithoutFetching(t *testing.T) {
	team, key := newAccessTeam(t)
	transport := &countingTransport{next: team.Client().Transport}
	a := Access{Team: strings.TrimPrefix(team.URL, "https://"), Audience: "app-aud", HTTP: &http.Client{Transport: transport}}
	if _, ok, err := a.AuthenticateToken(t.Context(), signAccessToken(t, team, key, "k1")); !ok || err != nil {
		t.Fatalf("known kid: ok=%v err=%v", ok, err)
	}
	for _, kid := range []string{"k2", "k3", "k4"} {
		if _, ok, _ := a.AuthenticateToken(t.Context(), signAccessToken(t, team, key, kid)); ok {
			t.Fatalf("accepted unknown kid %s", kid)
		}
	}
	if got := transport.count.Load(); got != 1 {
		t.Fatalf("certs fetched %d times, want 1", got)
	}
}

func TestAccessColdCacheCallersEachFetch(t *testing.T) {
	team, key := newAccessTeam(t)
	const callers = 5
	transport := &gatheringTransport{countingTransport: countingTransport{next: team.Client().Transport}, want: callers, all: make(chan struct{})}
	a := Access{Team: strings.TrimPrefix(team.URL, "https://"), Audience: "app-aud", HTTP: &http.Client{Transport: transport}}
	raw := signAccessToken(t, team, key, "k1")
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok, err := a.AuthenticateToken(t.Context(), raw); !ok || err != nil {
				t.Errorf("known kid: ok=%v err=%v", ok, err)
			}
		}()
	}
	wg.Wait()
	if got := transport.count.Load(); got != callers {
		t.Fatalf("certs fetched %d times by %d cold-cache callers, want %d", got, callers, callers)
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
