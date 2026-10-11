package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

type oidcIssuer struct {
	server  *httptest.Server
	rsa     *rsa.PrivateKey
	ec      *ecdsa.PrivateKey
	next    *rsa.PrivateKey
	delay   time.Duration
	fail    atomic.Bool
	stall   int32
	release chan struct{}
	hits    atomic.Int32
	keyHits atomic.Int32
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
		i.hits.Add(1)
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": i.server.URL, "jwks_uri": i.server.URL + "/keys"})
		case "/keys":
			keys := []jwk{
				{Kid: "r1", Kty: "RSA", N: base64.RawURLEncoding.EncodeToString(rsaKey.N.Bytes()), E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(rsaKey.E)).Bytes())},
				{Kid: "e1", Kty: "EC", Crv: "P-256", X: base64.RawURLEncoding.EncodeToString(ecKey.X.Bytes()), Y: base64.RawURLEncoding.EncodeToString(ecKey.Y.Bytes())},
			}
			if i.next != nil {
				keys = append(keys, jwk{Kid: "r2", Kty: "RSA", N: base64.RawURLEncoding.EncodeToString(i.next.N.Bytes()), E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(i.next.E)).Bytes())})
			}
			if i.keyHits.Add(1) == i.stall {
				<-i.release
			}
			time.Sleep(i.delay)
			if i.fail.Load() {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(jwkSet{Keys: keys})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(i.server.Close)
	jwksMu.Lock()
	jwksCache = map[string]jwksEntry{}
	jwksRefetchStarted = map[string]time.Time{}
	jwksMu.Unlock()
	return i
}

func (i *oidcIssuer) expireRefetchInterval() {
	jwksMu.Lock()
	defer jwksMu.Unlock()
	for url, started := range jwksRefetchStarted {
		jwksRefetchStarted[url] = started.Add(-oidcKeyRefetchInterval)
	}
}

func (i *oidcIssuer) expireCache() {
	jwksMu.Lock()
	defer jwksMu.Unlock()
	for url, entry := range jwksCache {
		entry.expires = time.Now().Add(-time.Second)
		jwksCache[url] = entry
	}
}

func (i *oidcIssuer) stallKeyHit(t *testing.T, hit int32) func() {
	t.Helper()
	i.stall, i.release = hit, make(chan struct{})
	release := sync.OnceFunc(func() { close(i.release) })
	t.Cleanup(release)
	return release
}

func (i *oidcIssuer) waitForKeyHits(t *testing.T, want int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for i.keyHits.Load() < want {
		if time.Now().After(deadline) {
			t.Fatalf("key set fetched %d times, waited for %d", i.keyHits.Load(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func authenticateAsync(ctx context.Context, o OIDC, raw string) <-chan bool {
	result := make(chan bool, 1)
	go func() {
		_, ok, _ := o.AuthenticateToken(ctx, raw)
		result <- ok
	}()
	return result
}

func awaitAuthentication(t *testing.T, result <-chan bool, what string) bool {
	t.Helper()
	select {
	case ok := <-result:
		return ok
	case <-time.After(5 * time.Second):
		t.Fatalf("%s waited for another caller's key fetch", what)
		return false
	}
}

func (i *oidcIssuer) signWithKid(t *testing.T, kid string, key *rsa.PrivateKey) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": i.server.URL, "aud": "k8flare", "sub": "u1", "exp": time.Now().Add(time.Hour).Unix()})
	tok.Header["kid"] = kid
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
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
		{"string email_verified", func(o *OIDC) { o.UsernameClaim = "email" }, jwt.MapClaims{"email": "a@b.c", "email_verified": "true"}},
		{"null email_verified", func(o *OIDC) { o.UsernameClaim = "email" }, jwt.MapClaims{"email": "a@b.c", "email_verified": json.RawMessage("null")}},
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

func TestOIDCRefetchesKeysOnUnknownKidAfterInterval(t *testing.T) {
	i := newOIDCIssuer(t)
	o := i.options()
	if _, ok, err := o.AuthenticateToken(t.Context(), i.sign(t, jwt.MapClaims{"sub": "u1"})); !ok || err != nil {
		t.Fatalf("before rotation: ok=%v err=%v", ok, err)
	}
	next, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	i.next = next
	raw := i.signWithKid(t, "r2", next)
	if _, ok, _ := o.AuthenticateToken(t.Context(), raw); ok {
		t.Fatal("refetched keys within the refetch interval")
	}
	if got := i.keyHits.Load(); got != 1 {
		t.Fatalf("key set fetched %d times within the interval, want 1", got)
	}
	i.expireRefetchInterval()
	if _, ok, err := o.AuthenticateToken(t.Context(), raw); !ok || err != nil {
		t.Fatalf("after rotation: ok=%v err=%v", ok, err)
	}
	if got := i.keyHits.Load(); got != 2 {
		t.Fatalf("key set fetched %d times, want 2", got)
	}
}

func TestOIDCRandomKidsFetchKeysOncePerInterval(t *testing.T) {
	i := newOIDCIssuer(t)
	o := i.options()
	if _, ok, err := o.AuthenticateToken(t.Context(), i.sign(t, jwt.MapClaims{"sub": "u1"})); !ok || err != nil {
		t.Fatalf("warm up: ok=%v err=%v", ok, err)
	}
	i.expireRefetchInterval()
	i.delay = 50 * time.Millisecond
	var wg sync.WaitGroup
	for n := range 20 {
		raw := i.signWithKid(t, fmt.Sprintf("random-%d", n), i.rsa)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok, _ := o.AuthenticateToken(t.Context(), raw); ok {
				t.Error("accepted a token with an unknown kid")
			}
		}()
	}
	wg.Wait()
	for n := range 20 {
		if _, ok, _ := o.AuthenticateToken(t.Context(), i.signWithKid(t, fmt.Sprintf("later-%d", n), i.rsa)); ok {
			t.Fatal("accepted a token with an unknown kid")
		}
	}
	if got := i.keyHits.Load(); got != 2 {
		t.Fatalf("key set fetched %d times for 40 unknown kids, want 2 (warm up and one refetch)", got)
	}
	if _, ok, err := o.AuthenticateToken(t.Context(), i.sign(t, jwt.MapClaims{"sub": "u1"})); !ok || err != nil {
		t.Fatalf("known kid after refetch: ok=%v err=%v", ok, err)
	}
}

func TestOIDCUnknownKidDoesNotWaitForAnotherCallersRefetch(t *testing.T) {
	i := newOIDCIssuer(t)
	o := i.options()
	if _, ok, err := o.AuthenticateToken(t.Context(), i.sign(t, jwt.MapClaims{"sub": "u1"})); !ok || err != nil {
		t.Fatalf("warm up: ok=%v err=%v", ok, err)
	}
	i.expireRefetchInterval()
	release := i.stallKeyHit(t, 2)
	first := authenticateAsync(t.Context(), o, i.signWithKid(t, "random-a", i.rsa))
	i.waitForKeyHits(t, 2)
	if awaitAuthentication(t, authenticateAsync(t.Context(), o, i.signWithKid(t, "random-b", i.rsa)), "an unknown kid") {
		t.Fatal("accepted a token with an unknown kid")
	}
	if got := i.keyHits.Load(); got != 2 {
		t.Fatalf("key set fetched %d times, want 2", got)
	}
	release()
	if awaitAuthentication(t, first, "the refetching caller") {
		t.Fatal("accepted a token with an unknown kid")
	}
}

func TestOIDCExpiredCacheIsRefreshedInsideRefetchInterval(t *testing.T) {
	for _, refetchFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("refetch fails=%v", refetchFails), func(t *testing.T) {
			i := newOIDCIssuer(t)
			o := i.options()
			if _, ok, err := o.AuthenticateToken(t.Context(), i.sign(t, jwt.MapClaims{"sub": "u1"})); !ok || err != nil {
				t.Fatalf("warm up: ok=%v err=%v", ok, err)
			}
			i.expireRefetchInterval()
			i.fail.Store(refetchFails)
			if _, ok, _ := o.AuthenticateToken(t.Context(), i.signWithKid(t, "random", i.rsa)); ok {
				t.Fatal("accepted a token with an unknown kid")
			}
			i.fail.Store(false)
			i.expireCache()
			if _, ok, err := o.AuthenticateToken(t.Context(), i.sign(t, jwt.MapClaims{"sub": "u1"})); !ok || err != nil {
				t.Fatalf("known kid after the cache expired: ok=%v err=%v", ok, err)
			}
			if got := i.keyHits.Load(); got != 3 {
				t.Fatalf("key set fetched %d times, want 3 (warm up, refetch, refresh of the expired cache)", got)
			}
		})
	}
}

func TestOIDCOlderFetchDoesNotReplaceNewerKeys(t *testing.T) {
	i := newOIDCIssuer(t)
	o := i.options()
	if _, ok, err := o.AuthenticateToken(t.Context(), i.sign(t, jwt.MapClaims{"sub": "u1"})); !ok || err != nil {
		t.Fatalf("warm up: ok=%v err=%v", ok, err)
	}
	i.expireRefetchInterval()
	release := i.stallKeyHit(t, 2)
	older := authenticateAsync(t.Context(), o, i.signWithKid(t, "random", i.rsa))
	i.waitForKeyHits(t, 2)
	next, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	i.next = next
	i.expireCache()
	rotated := i.signWithKid(t, "r2", next)
	if !awaitAuthentication(t, authenticateAsync(t.Context(), o, rotated), "a known kid with an expired cache") {
		t.Fatal("rejected the rotated key after refreshing the expired cache")
	}
	release()
	if awaitAuthentication(t, older, "the older refetch") {
		t.Fatal("accepted a token with an unknown kid")
	}
	if _, ok, err := o.AuthenticateToken(t.Context(), rotated); !ok || err != nil {
		t.Fatalf("rotated key after the older fetch landed: ok=%v err=%v", ok, err)
	}
	if got := i.keyHits.Load(); got != 3 {
		t.Fatalf("key set fetched %d times, want 3", got)
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
	if got := i.hits.Load(); got != 0 {
		t.Fatalf("issuer contacted %d times", got)
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
