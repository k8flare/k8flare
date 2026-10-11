package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"k8s.io/apiserver/pkg/authentication/user"
)

func TestAdminTokenSHA256AuthenticatesTheAdminTokenFromItsDigest(t *testing.T) {
	sum := sha256.Sum256([]byte("admin-token"))
	a := AdminTokenSHA256(hex.EncodeToString(sum[:]))
	resp, ok, err := a.AuthenticateToken(t.Context(), "admin-token")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if resp.User.GetName() != "admin" || len(resp.User.GetGroups()) != 2 || resp.User.GetGroups()[0] != user.SystemPrivilegedGroup {
		t.Fatalf("user %s %v", resp.User.GetName(), resp.User.GetGroups())
	}
	for _, token := range []string{"", "admin", hex.EncodeToString(sum[:])} {
		if _, ok, _ := a.AuthenticateToken(t.Context(), token); ok {
			t.Errorf("%q authenticated", token)
		}
	}
	empty := sha256.Sum256(nil)
	if _, ok, _ := AdminTokenSHA256(hex.EncodeToString(empty[:])).AuthenticateToken(t.Context(), ""); ok {
		t.Error("empty token authenticated against the digest of the empty string")
	}
	if _, ok, _ := AdminTokenSHA256("").AuthenticateToken(t.Context(), "admin-token"); ok {
		t.Error("unset digest authenticated")
	}
}
