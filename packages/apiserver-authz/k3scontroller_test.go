package authz

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/k8flare/k8flare/packages/addons"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

func packagedClusterRole(t *testing.T, name string) []byte {
	t.Helper()
	for _, f := range addons.Render(addons.Packaged(), addons.Vars()) {
		objs, err := addons.Decode(f.Content)
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		for _, obj := range objs {
			if obj.GetKind() != "ClusterRole" || obj.GetName() != name {
				continue
			}
			raw, err := obj.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			return raw
		}
	}
	t.Fatalf("no packaged ClusterRole %s", name)
	return nil
}

func TestNodeReadsWhatTheNetworkPolicyControllerWatches(t *testing.T) {
	role := packagedClusterRole(t, "system:k3s-controller")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kvs := []any{}
		if r.URL.Path == "/list" && r.URL.Query().Get("prefix") == "/registry/clusterroles/" {
			kvs = append(kvs, map[string]any{
				"key": "/registry/clusterroles/system:k3s-controller", "value": base64.StdEncoding.EncodeToString(role), "modRevision": 1,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kvs": kvs})
	}))
	defer srv.Close()
	a := New(&kine.Client{HTTP: &http.Client{Transport: rewriteHost{base: srv.URL, next: srv.Client().Transport}}})
	node := &user.DefaultInfo{Name: "system:node:k8flare-c1", Groups: []string{user.NodesGroup, user.AllAuthenticated}}
	calls := []componentCall{
		{"list", "", "pods", "", true},
		{"watch", "", "pods", "", true},
		{"list", "", "namespaces", "", true},
		{"watch", "", "namespaces", "", true},
		{"list", "networking.k8s.io", "networkpolicies", "", true},
		{"watch", "networking.k8s.io", "networkpolicies", "", true},
		{"list", "", "secrets", "", false},
		{"list", "", "configmaps", "", false},
		{"list", "", "endpoints", "", true},
		{"update", "", "namespaces", "", false},
		{"create", "networking.k8s.io", "networkpolicies", "", false},
		{"delete", "networking.k8s.io", "networkpolicies", "", false},
	}
	for _, c := range calls {
		d, _, err := a.Authorize(context.Background(), &authorizer.AttributesRecord{
			User: node, ResourceRequest: true, Verb: c.verb, APIGroup: c.group, Resource: c.resource, Subresource: c.subresource,
		})
		if err != nil {
			t.Fatalf("%s %s.%s: %v", c.verb, c.resource, c.group, err)
		}
		if got := d == authorizer.DecisionAllow; got != c.allow {
			t.Errorf("%s %s.%s: allowed=%v want %v", c.verb, c.resource, c.group, got, c.allow)
		}
	}
}
