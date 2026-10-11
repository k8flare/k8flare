package apiserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
)

func componentHandler(t *testing.T, componentKey string) http.Handler {
	t.Helper()
	store := fakeVaultStore()
	if _, err := store.Put(context.Background(), auth.ComponentKeyPath, []byte(componentKey), 0); err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(Config{Kine: store.HTTP, AdminToken: "admin-token", ClusterUID: "component-test"})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func postAs(handler http.Handler, token, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestInternalCallersAuthenticateWithTheStoredComponentKey(t *testing.T) {
	handler := componentHandler(t, "stored-component-key")
	key := []byte("stored-component-key")
	for _, c := range []struct{ component, path, body string }{
		{"queues", "/internal/queue/followup", `{"target":"gc"}`},
		{"nodes", "/internal/nodes/vm-plan", `{}`},
	} {
		if rec := postAs(handler, auth.MintComponentToken(key, c.component), c.path, c.body); rec.Code != http.StatusOK {
			t.Errorf("%s %s: %d %s", c.component, c.path, rec.Code, rec.Body.String())
		}
	}
	if rec := postAs(handler, auth.MintComponentToken(key, "queues"), "/internal/nodes/vm-plan", `{}`); rec.Code != http.StatusForbidden {
		t.Errorf("queues reached the nodes endpoint: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postAs(handler, "admin-token", "/internal/queue/followup", `{"target":"gc"}`); rec.Code != http.StatusOK {
		t.Errorf("admin token: %d %s", rec.Code, rec.Body.String())
	}
}

func TestComponentTokensKeyedByTheAdminTokenAreRefused(t *testing.T) {
	handler := componentHandler(t, "stored-component-key")
	for _, component := range []string{"queues", "workloads"} {
		if rec := postAs(handler, auth.MintComponentToken([]byte("admin-token"), component), "/internal/queue/followup", `{"target":"gc"}`); rec.Code == http.StatusOK {
			t.Errorf("%s token keyed by ADMIN_TOKEN was accepted", component)
		}
	}
}
