package rbac

import (
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDefaultsRoleBindingAPIGroup(t *testing.T) {
	binding := &rbacv1.RoleBinding{Subjects: []rbacv1.Subject{{Kind: rbacv1.UserKind, Name: "alice"}}}
	scheme.Scheme.Default(binding)
	if binding.RoleRef.APIGroup != rbacv1.GroupName {
		t.Fatalf("roleRef=%q", binding.RoleRef.APIGroup)
	}
	if binding.Subjects[0].APIGroup != rbacv1.GroupName {
		t.Fatalf("subject=%q", binding.Subjects[0].APIGroup)
	}
}
