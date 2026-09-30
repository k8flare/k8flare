package apiserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

func secretsEncryptMux() *http.ServeMux {
	client := &kine.Client{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"revision":7,"kvs":[]}`)), Header: http.Header{}, Request: r}, nil
	})}}
	mux := http.NewServeMux()
	installSecretsEncrypt(mux, client)
	return mux
}

func callSecretsEncrypt(method, path string, groups ...string) (int, string) {
	req := httptest.NewRequest(method, path, nil)
	req = req.WithContext(genericapirequest.WithUser(req.Context(), &user.DefaultInfo{Name: "someone", Groups: groups}))
	rec := httptest.NewRecorder()
	secretsEncryptMux().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestSecretsEncryptRoutesAreAdminOnly(t *testing.T) {
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/internal/secrets-encrypt/status"},
		{http.MethodPost, "/internal/secrets-encrypt/reencrypt"},
	} {
		if code, _ := callSecretsEncrypt(c.method, c.path, user.AllAuthenticated); code != http.StatusForbidden {
			t.Fatalf("%s %s as non-admin: %d", c.method, c.path, code)
		}
	}
}

func TestSecretsEncryptStatusAndReencryptAsAdmin(t *testing.T) {
	code, body := callSecretsEncrypt(http.MethodGet, "/internal/secrets-encrypt/status", user.SystemPrivilegedGroup)
	if code != http.StatusOK || !strings.Contains(body, `"enabled":false`) || !strings.Contains(body, `"total":0`) {
		t.Fatalf("status: %d %s", code, body)
	}
	code, body = callSecretsEncrypt(http.MethodPost, "/internal/secrets-encrypt/reencrypt", user.SystemPrivilegedGroup)
	if code != http.StatusOK || !strings.Contains(body, `"rewritten":0`) {
		t.Fatalf("reencrypt: %d %s", code, body)
	}
	if code, _ := callSecretsEncrypt(http.MethodGet, "/internal/secrets-encrypt/reencrypt", user.SystemPrivilegedGroup); code != http.StatusMethodNotAllowed {
		t.Fatalf("GET reencrypt: %d", code)
	}
}
