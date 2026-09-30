package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apiserver/pkg/authentication/request/bearertoken"
	requnion "k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

func serveWithAuth(t *testing.T, authorization string) (int, user.Info) {
	t.Helper()
	var got user.Info
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = genericapirequest.UserFrom(r.Context())
	})
	handler := WithAuth(next, requnion.New(bearertoken.New(AdminToken("secret")), Access{}))
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, got
}

func TestNoCredentialsAreAnonymous(t *testing.T) {
	code, u := serveWithAuth(t, "")
	if code != http.StatusOK || u == nil {
		t.Fatalf("code=%d user=%v", code, u)
	}
	if u.GetName() != user.Anonymous || len(u.GetGroups()) != 1 || u.GetGroups()[0] != user.AllUnauthenticated {
		t.Fatalf("user=%v groups=%v", u.GetName(), u.GetGroups())
	}
}

func TestBadBearerTokenIsUnauthorized(t *testing.T) {
	code, u := serveWithAuth(t, "Bearer wrong")
	if code != http.StatusUnauthorized || u != nil {
		t.Fatalf("code=%d user=%v", code, u)
	}
}

func TestValidTokenIsNotAnonymous(t *testing.T) {
	code, u := serveWithAuth(t, "Bearer secret")
	if code != http.StatusOK || u == nil || u.GetName() != "admin" {
		t.Fatalf("code=%d user=%v", code, u)
	}
}
