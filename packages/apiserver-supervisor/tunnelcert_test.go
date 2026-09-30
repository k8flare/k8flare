package supervisor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestTunnelNodeByCertificate(t *testing.T) {
	as := func(name string, groups ...string) authenticator.Request {
		return authenticator.RequestFunc(func(*http.Request) (*authenticator.Response, bool, error) {
			return &authenticator.Response{User: &user.DefaultInfo{Name: name, Groups: groups}}, true, nil
		})
	}
	cases := []struct {
		name string
		auth authenticator.Request
		code int
		body string
	}{
		{"a node certificate", as("system:node:n1", "system:nodes"), http.StatusOK, `"node":"n1"`},
		{"a node name outside the nodes group", as("system:node:n1", "other"), http.StatusUnauthorized, ""},
		{"a non-node identity", as("system:kube-proxy", "system:nodes"), http.StatusUnauthorized, ""},
		{"an empty node name", as("system:node:", "system:nodes"), http.StatusUnauthorized, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := New(NewVault(fakeKine(t)), "join")
			s.ClientCerts = c.auth
			mux := http.NewServeMux()
			s.Register(mux)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1-k3s/node-tunnel", nil))
			if rr.Code != c.code || !strings.Contains(rr.Body.String(), c.body) {
				t.Fatal(rr.Code, rr.Body.String())
			}
		})
	}
}
