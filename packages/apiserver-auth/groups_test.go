package auth

import (
	"context"
	"slices"
	"testing"

	"k8s.io/apiserver/pkg/authentication/token/union"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestAuthenticatedGroupAddedToServiceAccountToken(t *testing.T) {
	tok := issue(t, nil, BoundObjects{})
	tokens := WithAuthenticatedGroup(ServiceAccountToken{HMAC: testHMAC, Objects: objectsWithSA("kube-system", "coredns", "uid-1")})
	resp, ok, err := tokens.AuthenticateToken(context.Background(), tok)
	if err != nil || !ok {
		t.Fatalf("auth %v %v", ok, err)
	}
	got := resp.User.GetGroups()
	for _, want := range []string{"system:serviceaccounts", "system:serviceaccounts:kube-system", user.AllAuthenticated} {
		if !slices.Contains(got, want) {
			t.Fatalf("groups %v lack %s", got, want)
		}
	}
	if resp.User.GetUID() != "uid-1" || len(resp.User.GetExtra()[user.CredentialIDKey]) != 1 || len(resp.Audiences) == 0 {
		t.Fatalf("uid=%s extra=%v audiences=%v", resp.User.GetUID(), resp.User.GetExtra(), resp.Audiences)
	}
}

func TestAuthenticatedGroupNotDuplicated(t *testing.T) {
	tokens := WithAuthenticatedGroup(union.New(AdminToken("secret")))
	resp, ok, err := tokens.AuthenticateToken(context.Background(), "secret")
	if err != nil || !ok {
		t.Fatalf("auth %v %v", ok, err)
	}
	got := resp.User.GetGroups()
	if len(got) != 2 || got[0] != user.SystemPrivilegedGroup || got[1] != user.AllAuthenticated {
		t.Fatalf("groups %v", got)
	}
}

func TestAuthenticatedGroupSkipsUnknownToken(t *testing.T) {
	tokens := WithAuthenticatedGroup(union.New(AdminToken("secret")))
	resp, ok, err := tokens.AuthenticateToken(context.Background(), "other")
	if resp != nil || ok || err != nil {
		t.Fatalf("resp=%v ok=%v err=%v", resp, ok, err)
	}
}
