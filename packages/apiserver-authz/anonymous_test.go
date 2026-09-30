package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

func TestAnonymousGetsOnlyPublicInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kvs": []any{}})
	}))
	defer srv.Close()
	a := New(&kine.Client{HTTP: &http.Client{Transport: rewriteHost{base: srv.URL, next: srv.Client().Transport}}})
	anonymous := &user.DefaultInfo{Name: user.Anonymous, Groups: []string{user.AllUnauthenticated}}
	cases := []struct {
		path  string
		allow bool
	}{
		{"/healthz", true},
		{"/livez", true},
		{"/readyz", true},
		{"/version", true},
		{"/api", false},
		{"/apis", false},
		{"/openapi/v2", false},
		{"/openid/v1/jwks", false},
		{"/.well-known/openid-configuration", false},
	}
	for _, c := range cases {
		d, _, err := a.Authorize(context.Background(), &authorizer.AttributesRecord{User: anonymous, Verb: "get", Path: c.path})
		if err != nil {
			t.Fatalf("%s: %v", c.path, err)
		}
		if got := d == authorizer.DecisionAllow; got != c.allow {
			t.Errorf("%s: allowed=%v want %v", c.path, got, c.allow)
		}
	}
	d, _, _ := a.Authorize(context.Background(), &authorizer.AttributesRecord{User: anonymous, ResourceRequest: true, Verb: "list", Resource: "pods", Namespace: "default"})
	if d == authorizer.DecisionAllow {
		t.Error("anonymous listed pods")
	}
}
