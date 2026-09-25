package rbac

import (
	"testing"

	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy"
)

func TestUpstreamRBACBootstrapContainsDefaults(t *testing.T) {
	roles := append(bootstrappolicy.ClusterRoles(), bootstrappolicy.ControllerRoles()...)
	want := []string{"cluster-admin", "admin", "edit", "view", "system:discovery", "system:basic-user", "system:public-info-viewer", "system:node"}
	byName := map[string]bool{}
	for _, cr := range roles {
		if cr.Name == "" || byName[cr.Name] {
			t.Fatalf("clusterrole name %q", cr.Name)
		}
		if cr.Labels["kubernetes.io/bootstrapping"] != "rbac-defaults" {
			t.Fatalf("clusterrole %s labels = %v", cr.Name, cr.Labels)
		}
		byName[cr.Name] = true
	}
	for _, name := range want {
		if !byName[name] {
			t.Fatalf("missing clusterrole %s", name)
		}
	}
	bindings := append(bootstrappolicy.ClusterRoleBindings(), bootstrappolicy.ControllerRoleBindings()...)
	bound := map[string]bool{}
	for _, crb := range bindings {
		if crb.Name == "" || bound[crb.Name] {
			t.Fatalf("clusterrolebinding name %q", crb.Name)
		}
		bound[crb.Name] = true
	}
	if !bound["cluster-admin"] || !bound["system:discovery"] || !bound["system:basic-user"] {
		t.Fatalf("bindings = %v", bound)
	}
}
