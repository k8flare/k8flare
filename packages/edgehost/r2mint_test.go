package edgehost

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMintLocal(t *testing.T) {
	got, err := MintLocal("parent-key", "parent-secret", "acct1", "k8flare-pods", "clusters/default/pods/ns/pod/", time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessKeyID != "parent-key" || len(got.SecretAccessKey) != 64 {
		t.Fatal(got.AccessKeyID, got.SecretAccessKey)
	}
	raw, err := base64.StdEncoding.DecodeString(got.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	jwt := strings.TrimPrefix(string(raw), "jwt/")
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 || !strings.HasPrefix(string(raw), "jwt/") {
		t.Fatal(string(raw))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		Bucket string `json:"bucket"`
		Scope  string `json:"scope"`
		Paths  struct {
			PrefixPaths []string `json:"prefixPaths"`
		} `json:"paths"`
		Sub string `json:"sub"`
		Iss string `json:"iss"`
		Aud string `json:"aud"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Bucket != "k8flare-pods" || claims.Scope != "object-read-write" || len(claims.Paths.PrefixPaths) != 1 || claims.Paths.PrefixPaths[0] != "clusters/default/pods/ns/pod/" || claims.Sub != "acct1" || claims.Iss != "parent-key" || claims.Aud != "acct1.r2.cloudflarestorage.com" {
		t.Fatalf("%+v", claims)
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || !strings.Contains(string(header), `"alg":"HS256"`) {
		t.Fatal(string(header), err)
	}
}
