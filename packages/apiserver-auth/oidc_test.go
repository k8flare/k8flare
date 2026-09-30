package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
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

type oidcIssuer struct {
	server *httptest.Server
	rsa    *rsa.PrivateKey
	ec     *ecdsa.PrivateKey
	hits   int
}

func newOIDCIssuer(t *testing.T) *oidcIssuer {
	t.Helper()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	i := &oidcIssuer{rsa: rsaKey, ec: ecKey}
	i.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.hits++
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": i.server.URL, "jwks_uri": i.server.URL + "/keys"})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jwkSet{Keys: []jwk{
				{Kid: "r1", Kty: "RSA", N: base64.RawURLEncoding.EncodeToString(rsaKey.N.Bytes()), E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(rsaKey.E)).Bytes())},
				{Kid: "e1", Kty: "EC", Crv: "P-256", X: base64.RawURLEncoding.EncodeToString(ecKey.X.Bytes()), Y: base64.RawURLEncoding.EncodeToString(ecKey.Y.Bytes())},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(i.server.Close)
	jwksMu.Lock()
	jwksCache = map[string]jwksEntry{}
	jwksMu.Unlock()
	return i
}

func (i *oidcIssuer) sign(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	base := jwt.MapClaims{"iss": i.server.URL, "aud": "k8flare", "exp": time.Now().Add(time.Hour).Unix()}
	for k, v := range claims {
		if v == nil {
			delete(base, k)
			continue
		}
		base[k] = v
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, base)
	tok.Header["kid"] = "r1"
	raw, err := tok.SignedString(i.rsa)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (i *oidcIssuer) options() OIDC {
	return OIDC{IssuerURL: i.server.URL, ClientID: "k8flare", HTTP: i.server.Client()}
}

func TestOIDCUsernameAndGroups(t *testing.T) {
	i := newOIDCIssuer(t)
	cases := []struct {
		name       string
		oidc       func(*OIDC)
		claims     jwt.MapClaims
		wantUser   string
		wantGroups []string
	}{
		{"sub gets the issuer prefix", func(*OIDC) {}, jwt.MapClaims{"sub": "u1"}, i.server.URL + "#u1", nil},
		{"dash disables the prefix", func(o *OIDC) { o.UsernamePrefix = "-" }, jwt.MapClaims{"sub": "u1"}, "u1", nil},
		{"explicit prefix", func(o *OIDC) { o.UsernamePrefix = "oidc:" }, jwt.MapClaims{"sub": "u1"}, "oidc:u1", nil},
		{"email claim is unprefixed", func(o *OIDC) { o.UsernameClaim = "email" }, jwt.MapClaims{"email": "ada@kooffice.jp", "email_verified": true}, "ada@kooffice.jp", nil},
		{"groups array", func(o *OIDC) { o.GroupsClaim = "groups"; o.GroupsPrefix = "oidc:" }, jwt.MapClaims{"sub": "u1", "groups": []string{"eng", "ops"}}, i.server.URL + "#u1", []string{"oidc:eng", "oidc:ops"}},
		{"groups string", func(o *OIDC) { o.GroupsClaim = "groups" }, jwt.MapClaims{"sub": "u1", "groups": "eng"}, i.server.URL + "#u1", []string{"eng"}},
	}
	for _, c := range cases {
		o := i.options()
		c.oidc(&o)
		resp, ok, err := o.AuthenticateToken(t.Context(), i.sign(t, c.claims))
		if err != nil || !ok {
			t.Fatalf("%s: ok=%v err=%v", c.name, ok, err)
		}
		if resp.User.GetName() != c.wantUser {
			t.Errorf("%s: name=%q want %q", c.name, resp.User.GetName(), c.wantUser)
		}
		want := strings.Join(append(append([]string{}, c.wantGroups...), user.AllAuthenticated), ",")
		if got := strings.Join(resp.User.GetGroups(), ","); got != want {
			t.Errorf("%s: groups=%q want %q", c.name, got, want)
		}
	}
}

func TestOIDCRejects(t *testing.T) {
	i := newOIDCIssuer(t)
	cases := []struct {
		name   string
		oidc   func(*OIDC)
		claims jwt.MapClaims
	}{
		{"wrong audience", func(*OIDC) {}, jwt.MapClaims{"sub": "u1", "aud": "other"}},
		{"wrong issuer", func(*OIDC) {}, jwt.MapClaims{"sub": "u1", "iss": "https://elsewhere.example"}},
		{"expired", func(*OIDC) {}, jwt.MapClaims{"sub": "u1", "exp": time.Now().Add(-time.Hour).Unix()}},
		{"no expiry", func(*OIDC) {}, jwt.MapClaims{"sub": "u1", "exp": nil}},
		{"missing username claim", func(*OIDC) {}, jwt.MapClaims{}},
		{"unverified email", func(o *OIDC) { o.UsernameClaim = "email" }, jwt.MapClaims{"email": "a@b.c", "email_verified": false}},
		{"required claim differs", func(o *OIDC) { o.RequiredClaims = map[string]string{"hd": "kooffice.jp"} }, jwt.MapClaims{"sub": "u1", "hd": "other.jp"}},
		{"system group", func(o *OIDC) { o.GroupsClaim = "groups"; o.GroupsPrefix = "-" }, jwt.MapClaims{"sub": "u1", "groups": []string{"system:masters"}}},
	}
	for _, c := range cases {
		o := i.options()
		c.oidc(&o)
		resp, ok, err := o.AuthenticateToken(t.Context(), i.sign(t, c.claims))
		if c.name == "system group" {
			if !ok || err != nil {
				t.Fatalf("%s: ok=%v err=%v", c.name, ok, err)
			}
			for _, g := range resp.User.GetGroups() {
				if g == user.SystemPrivilegedGroup {
					t.Errorf("%s: kept %s", c.name, g)
				}
			}
			continue
		}
		if ok || err != nil {
			t.Errorf("%s: ok=%v err=%v", c.name, ok, err)
		}
	}
}

func TestOIDCAcceptsRequiredClaimAndES256(t *testing.T) {
	i := newOIDCIssuer(t)
	o := i.options()
	o.RequiredClaims = map[string]string{"hd": "kooffice.jp"}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{"iss": i.server.URL, "aud": "k8flare", "sub": "u1", "hd": "kooffice.jp", "exp": time.Now().Add(time.Hour).Unix()})
	tok.Header["kid"] = "e1"
	raw, err := tok.SignedString(i.ec)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := o.AuthenticateToken(t.Context(), raw); !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestOIDCIgnoresForeignTokensWithoutFetching(t *testing.T) {
	i := newOIDCIssuer(t)
	o := i.options()
	for _, raw := range []string{"admin-secret", "a.b.c", i.sign(t, jwt.MapClaims{"iss": "https://elsewhere.example"})} {
		if _, ok, err := o.AuthenticateToken(t.Context(), raw); ok || err != nil {
			t.Fatalf("%q: ok=%v err=%v", raw, ok, err)
		}
	}
	if i.hits != 0 {
		t.Fatalf("issuer contacted %d times", i.hits)
	}
}

func TestOIDCDisabledWithoutIssuer(t *testing.T) {
	if _, ok, err := (OIDC{ClientID: "x"}).AuthenticateToken(t.Context(), "a.b.c"); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestParseRequiredClaims(t *testing.T) {
	got, err := ParseRequiredClaims("hd=kooffice.jp, tier=gold")
	if err != nil || got["hd"] != "kooffice.jp" || got["tier"] != "gold" || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := ParseRequiredClaims("nokey"); err == nil {
		t.Fatal("accepted malformed pair")
	}
	if got, err := ParseRequiredClaims(""); err != nil || len(got) != 0 {
		t.Fatalf("%v %v", got, err)
	}
}
