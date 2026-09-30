package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

type componentCall struct {
	verb, group, resource, subresource string
	allow                              bool
}

func TestComponentIdentitiesGetOnlyTheirUpstreamRoles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kvs": []any{}})
	}))
	defer srv.Close()
	a := New(&kine.Client{HTTP: &http.Client{Transport: rewriteHost{base: srv.URL, next: srv.Client().Transport}}})
	key := []byte("change-me")
	cases := map[string][]componentCall{
		"scheduler": {
			{"list", "", "pods", "", true},
			{"watch", "", "nodes", "", true},
			{"create", "", "pods", "binding", true},
			{"patch", "", "pods", "status", true},
			{"list", "", "persistentvolumeclaims", "", true},
			{"update", "", "persistentvolumes", "", true},
			{"list", "storage.k8s.io", "csinodes", "", true},
			{"list", "resource.k8s.io", "resourceslices", "", true},
			{"update", "resource.k8s.io", "resourceclaims", "status", true},
			{"create", "", "events", "", true},
			{"list", "", "secrets", "", false},
			{"create", "", "pods", "", false},
			{"delete", "", "nodes", "", false},
			{"create", "rbac.authorization.k8s.io", "clusterrolebindings", "", false},
		},
		"gc": {
			{"list", "apps", "deployments", "", true},
			{"watch", "", "pods", "", true},
			{"patch", "apps", "replicasets", "", true},
			{"update", "batch", "jobs", "", true},
			{"delete", "", "pods", "", true},
			{"delete", "example.com", "widgets", "", true},
			{"create", "", "pods", "", false},
			{"create", "rbac.authorization.k8s.io", "clusterrolebindings", "", false},
		},
		"hpa": {
			{"list", "autoscaling", "horizontalpodautoscalers", "", true},
			{"update", "autoscaling", "horizontalpodautoscalers", "status", true},
			{"get", "apps", "deployments", "scale", true},
			{"update", "apps", "deployments", "scale", true},
			{"list", "metrics.k8s.io", "pods", "", true},
			{"list", "", "pods", "", true},
			{"create", "", "events", "", true},
			{"delete", "", "pods", "", false},
			{"create", "", "pods", "", false},
			{"list", "", "secrets", "", false},
		},
		"attachdetach": {
			{"list", "", "pods", "", true},
			{"list", "", "nodes", "", true},
			{"list", "", "persistentvolumes", "", true},
			{"create", "storage.k8s.io", "volumeattachments", "", true},
			{"delete", "storage.k8s.io", "volumeattachments", "", true},
			{"patch", "", "nodes", "status", true},
			{"list", "", "secrets", "", false},
			{"create", "", "pods", "", false},
		},
	}
	tokens := auth.ComponentTokens{Key: key}
	for component, calls := range cases {
		resp, ok, err := tokens.AuthenticateToken(context.Background(), auth.MintComponentToken(key, component))
		if err != nil || !ok {
			t.Fatalf("%s: ok=%v err=%v", component, ok, err)
		}
		for _, c := range calls {
			d, _, err := a.Authorize(context.Background(), &authorizer.AttributesRecord{
				User: resp.User, ResourceRequest: true, Verb: c.verb, APIGroup: c.group, Resource: c.resource, Subresource: c.subresource, Namespace: "default",
			})
			if err != nil {
				t.Fatalf("%s %s %s/%s: %v", component, c.verb, c.resource, c.subresource, err)
			}
			if got := d == authorizer.DecisionAllow; got != c.allow {
				t.Errorf("%s %s %s.%s/%s: allowed=%v want %v", component, c.verb, c.resource, c.group, c.subresource, got, c.allow)
			}
		}
	}
}
