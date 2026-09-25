package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestServiceAccountTokenRoundTrip(t *testing.T) {
	hmac := []byte("test-hmac")
	tok, err := IssueServiceAccountToken(hmac, "kube-system", "coredns", "uid-1", time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, ok, err := ServiceAccountToken{HMAC: hmac}.AuthenticateToken(context.Background(), tok)
	if err != nil || !ok {
		t.Fatalf("auth %v %v", ok, err)
	}
	if resp.User.GetName() != "system:serviceaccount:kube-system:coredns" {
		t.Fatalf("name %s", resp.User.GetName())
	}
	if resp.User.GetUID() != "uid-1" {
		t.Fatalf("uid %s", resp.User.GetUID())
	}
	got := resp.User.GetGroups()
	want := map[string]bool{"system:serviceaccounts": true, "system:serviceaccounts:kube-system": true, user.AllAuthenticated: true}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("group %s", g)
		}
	}
	ids := resp.User.GetExtra()[user.CredentialIDKey]
	if len(ids) != 1 || len(ids[0]) < 4 || ids[0][:4] != "JTI=" {
		t.Fatalf("credential-id %v", ids)
	}
}

func TestServiceAccountTokenOIDCIssuer(t *testing.T) {
	secret := []byte("test-hmac")
	tok, err := IssueServiceAccountToken(secret, "default", "sa", "u", time.Now().Add(time.Hour), []string{"oidc-discovery-test"})
	if err != nil {
		t.Fatal(err)
	}
	resp, ok, err := ServiceAccountToken{HMAC: secret}.AuthenticateToken(context.Background(), tok)
	if err != nil || !ok {
		t.Fatalf("auth %v %v", ok, err)
	}
	if resp.User.GetName() != "system:serviceaccount:default:sa" {
		t.Fatalf("name %s", resp.User.GetName())
	}
	parsed, _, err := jwt.NewParser().ParseUnverified(tok, &jwt.MapClaims{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Method.Alg() != "RS256" {
		t.Fatalf("alg %s", parsed.Method.Alg())
	}
	if (*parsed.Claims.(*jwt.MapClaims))["iss"] != "https://kubernetes.default.svc.cluster.local" {
		t.Fatalf("iss %v", parsed.Claims)
	}
}

func TestServiceAccountTokenRejectsBadHMAC(t *testing.T) {
	tok, err := IssueServiceAccountToken([]byte("a"), "default", "sa", "u", time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, err := ServiceAccountToken{HMAC: []byte("b")}.AuthenticateToken(context.Background(), tok)
	if err != nil || ok {
		t.Fatalf("expected reject %v %v", ok, err)
	}
}
