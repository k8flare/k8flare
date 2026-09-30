package rbac

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "rbac.authorization.k8s.io", Version: "v1"}, metav1.APIResource{Name: "roles", Kind: "Role", Namespaced: true})
	obj := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
		Rules:      []rbacv1.PolicyRule{{Resources: []string{"pods"}}},
	}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "rules[0].verbs", "verbs must contain at least one value")
}
