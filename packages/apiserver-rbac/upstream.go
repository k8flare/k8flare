package rbac

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/rbac"
	"k8s.io/kubernetes/pkg/registry/rbac/clusterrole"
	"k8s.io/kubernetes/pkg/registry/rbac/clusterrolebinding"
	"k8s.io/kubernetes/pkg/registry/rbac/role"
	"k8s.io/kubernetes/pkg/registry/rbac/rolebinding"
)

func init() {
	utilruntime.Must(rbac.AddToScheme(registry.InternalScheme))
	utilruntime.Must(rbacv1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "rbac.authorization.k8s.io", Resource: resource}
	}
	registry.Upstreams[group("clusterrolebindings")] = registry.Upstream{Strategy: clusterrolebinding.Strategy}
	registry.Upstreams[group("clusterroles")] = registry.Upstream{Strategy: clusterrole.Strategy}
	registry.Upstreams[group("rolebindings")] = registry.Upstream{Strategy: rolebinding.Strategy}
	registry.Upstreams[group("roles")] = registry.Upstream{Strategy: role.Strategy}
}
