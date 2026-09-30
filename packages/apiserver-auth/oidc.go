package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const oidcDiscoveryPath = "/.well-known/openid-configuration"

type OIDC struct {
	IssuerURL      string
	ClientID       string
	UsernameClaim  string
	UsernamePrefix string
	GroupsClaim    string
	GroupsPrefix   string
	RequiredClaims map[string]string
	HTTP           *http.Client
}

var oidcSigningMethods = []string{"RS256", "RS384", "RS512", "PS256", "ES256", "ES384", "ES512"}

func (o OIDC) AuthenticateToken(_ context.Context, raw string) (*authenticator.Response, bool, error) {
	if o.IssuerURL == "" || o.ClientID == "" || strings.Count(raw, ".") != 2 {
		return nil, false, nil
	}
	issuer := strings.TrimSuffix(o.IssuerURL, "/")
	unverified := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(raw, unverified); err != nil {
		return nil, false, nil
	}
	if iss, _ := unverified["iss"].(string); iss != o.IssuerURL {
		return nil, false, nil
	}
	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(tok *jwt.Token) (any, error) {
		kid, _ := tok.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("missing kid")
		}
		jwksURL, err := o.jwksURL(issuer)
		if err != nil {
			return nil, err
		}
		return lookupKey(o.client(), jwksURL, kid)
	}, jwt.WithIssuer(o.IssuerURL), jwt.WithAudience(o.ClientID), jwt.WithValidMethods(oidcSigningMethods), jwt.WithExpirationRequired())
	if err != nil || !tok.Valid {
		return nil, false, nil
	}
	for name, want := range o.RequiredClaims {
		if got, _ := claims[name].(string); got != want {
			return nil, false, nil
		}
	}
	name, ok := o.username(claims)
	if !ok {
		return nil, false, nil
	}
	groups := append(o.groups(claims), user.AllAuthenticated)
	return &authenticator.Response{User: &user.DefaultInfo{Name: name, Groups: groups}}, true, nil
}

func (o OIDC) username(claims jwt.MapClaims) (string, bool) {
	claim := o.UsernameClaim
	if claim == "" {
		claim = "sub"
	}
	value, _ := claims[claim].(string)
	if value == "" {
		return "", false
	}
	if claim == "email" {
		if verified, ok := claims["email_verified"].(bool); ok && !verified {
			return "", false
		}
	}
	switch {
	case o.UsernamePrefix == "-":
		return value, true
	case o.UsernamePrefix != "":
		return o.UsernamePrefix + value, true
	case claim == "email":
		return value, true
	}
	return strings.TrimSuffix(o.IssuerURL, "/") + "#" + value, true
}

func (o OIDC) groups(claims jwt.MapClaims) []string {
	if o.GroupsClaim == "" {
		return nil
	}
	var names []string
	switch v := claims[o.GroupsClaim].(type) {
	case string:
		names = []string{v}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				names = append(names, s)
			}
		}
	}
	var groups []string
	for _, name := range names {
		if name == "" || strings.HasPrefix(name, "system:") {
			continue
		}
		groups = append(groups, o.GroupsPrefix+name)
	}
	return groups
}

func (o OIDC) client() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	return http.DefaultClient
}

type oidcDiscoveryEntry struct {
	jwksURL string
	expires time.Time
}

var (
	oidcDiscoveryMu    sync.Mutex
	oidcDiscoveryCache = map[string]oidcDiscoveryEntry{}
)

func (o OIDC) jwksURL(issuer string) (string, error) {
	oidcDiscoveryMu.Lock()
	entry, ok := oidcDiscoveryCache[issuer]
	oidcDiscoveryMu.Unlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.jwksURL, nil
	}
	resp, err := o.client().Get(issuer + oidcDiscoveryPath)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("discovery status %d", resp.StatusCode)
	}
	var doc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", err
	}
	if strings.TrimSuffix(doc.Issuer, "/") != issuer || doc.JWKSURI == "" {
		return "", fmt.Errorf("discovery document does not describe %s", issuer)
	}
	oidcDiscoveryMu.Lock()
	oidcDiscoveryCache[issuer] = oidcDiscoveryEntry{jwksURL: doc.JWKSURI, expires: time.Now().Add(10 * time.Minute)}
	oidcDiscoveryMu.Unlock()
	return doc.JWKSURI, nil
}

func ParseRequiredClaims(raw string) (map[string]string, error) {
	claims := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, value, ok := strings.Cut(pair, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid required claim %q, want name=value", pair)
		}
		claims[name] = value
	}
	return claims, nil
}
