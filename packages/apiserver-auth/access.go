package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const accessAssertion = "Cf-Access-Jwt-Assertion"

type Access struct {
	Team     string
	Audience string
	HTTP     *http.Client
}

type accessClaims struct {
	Email      string `json:"email"`
	CommonName string `json:"common_name"`
	jwt.RegisteredClaims
}

func (a Access) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	return a.authenticate(req.Header.Get(accessAssertion))
}

func (a Access) AuthenticateToken(_ context.Context, token string) (*authenticator.Response, bool, error) {
	return a.authenticate(token)
}

func (a Access) authenticate(raw string) (*authenticator.Response, bool, error) {
	if raw == "" || a.Team == "" || a.Audience == "" {
		return nil, false, nil
	}
	iss := accessIssuer(a.Team)
	tok, err := jwt.ParseWithClaims(raw, &accessClaims{}, a.key, jwt.WithIssuer(iss), jwt.WithAudience(a.Audience))
	if err != nil || !tok.Valid {
		return nil, false, nil
	}
	claims, ok := tok.Claims.(*accessClaims)
	if !ok {
		return nil, false, nil
	}
	name := claims.Email
	if name == "" {
		name = claims.CommonName
	}
	if name == "" {
		return nil, false, nil
	}
	return &authenticator.Response{User: &user.DefaultInfo{Name: name, Groups: []string{user.AllAuthenticated}}}, true, nil
}

func (a Access) key(tok *jwt.Token) (any, error) {
	kid, _ := tok.Header["kid"].(string)
	if kid == "" {
		return nil, fmt.Errorf("missing kid")
	}
	return lookupAccessKey(a.client(), accessIssuer(a.Team), kid)
}

func (a Access) client() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return http.DefaultClient
}

func accessIssuer(team string) string {
	host := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(team, "https://"), "http://"), "/")
	if !strings.Contains(host, ".") {
		host += ".cloudflareaccess.com"
	}
	return "https://" + host
}

type jwkSet struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksEntry struct {
	keys    map[string]any
	expires time.Time
}

var (
	jwksMu    sync.Mutex
	jwksCache = map[string]jwksEntry{}
)

func lookupAccessKey(client *http.Client, iss, kid string) (any, error) {
	jwksMu.Lock()
	entry, ok := jwksCache[iss]
	if ok && time.Now().Before(entry.expires) {
		key, found := entry.keys[kid]
		jwksMu.Unlock()
		if !found {
			return nil, fmt.Errorf("unknown kid")
		}
		return key, nil
	}
	jwksMu.Unlock()
	keys, err := fetchJWKS(client, iss)
	if err != nil {
		return nil, err
	}
	jwksMu.Lock()
	jwksCache[iss] = jwksEntry{keys: keys, expires: time.Now().Add(10 * time.Minute)}
	key, found := keys[kid]
	jwksMu.Unlock()
	if !found {
		return nil, fmt.Errorf("unknown kid")
	}
	return key, nil
}

func fetchJWKS(client *http.Client, iss string) (map[string]any, error) {
	resp, err := client.Get(iss + "/cdn-cgi/access/certs")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	var set jwkSet
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return nil, err
	}
	keys := map[string]any{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		pub, err := rsaPublic(k.N, k.E)
		if err != nil {
			return nil, err
		}
		keys[k.Kid] = pub
	}
	return keys, nil
}

func rsaPublic(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	var eInt int
	for _, b := range eBytes {
		eInt = eInt<<8 + int(b)
	}
	if eInt == 0 {
		return nil, fmt.Errorf("invalid exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: eInt}, nil
}
