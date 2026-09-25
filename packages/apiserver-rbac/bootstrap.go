package rbac

import (
	"context"
	"net/http"
	"sync"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy"
)

func init() {
	registry.Middleware = append(registry.Middleware, func(stores map[string]*registry.Store) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return bootstrapRBAC(stores["clusterroles"], stores["clusterrolebindings"], stores["roles"], stores["rolebindings"], next)
		}
	})
}

func bootstrapRBAC(clusterRoles, clusterRoleBindings, roles, roleBindings *genericregistry.Store, next http.Handler) http.Handler {
	if clusterRoles == nil || clusterRoleBindings == nil {
		return next
	}
	var once sync.Once
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			ensureRBAC(r.Context(), clusterRoles, clusterRoleBindings, roles, roleBindings)
		})
		next.ServeHTTP(w, r)
	})
}

func ensureRBAC(ctx context.Context, clusterRoles, clusterRoleBindings, roles, roleBindings *genericregistry.Store) {
	for _, cr := range append(bootstrappolicy.ClusterRoles(), bootstrappolicy.ControllerRoles()...) {
		ensureClusterRole(ctx, clusterRoles, &cr)
	}
	for _, crb := range append(bootstrappolicy.ClusterRoleBindings(), bootstrappolicy.ControllerRoleBindings()...) {
		ensureClusterRoleBinding(ctx, clusterRoleBindings, &crb)
	}
	if roles != nil {
		for ns, list := range bootstrappolicy.NamespaceRoles() {
			for i := range list {
				ensureRole(ctx, roles, ns, &list[i])
			}
		}
	}
	if roleBindings != nil {
		for ns, list := range bootstrappolicy.NamespaceRoleBindings() {
			for i := range list {
				ensureRoleBinding(ctx, roleBindings, ns, &list[i])
			}
		}
	}
}

func ensureClusterRole(ctx context.Context, store *genericregistry.Store, cr *rbacv1.ClusterRole) {
	obj := cr.DeepCopy()
	ctx = genericapirequest.WithNamespace(ctx, metav1.NamespaceNone)
	ctx = genericapirequest.WithRequestInfo(ctx, &genericapirequest.RequestInfo{
		IsResourceRequest: true,
		Verb:              "create",
		APIGroup:          "rbac.authorization.k8s.io",
		APIVersion:        "v1",
		Resource:          "clusterroles",
		Name:              obj.Name,
	})
	if _, err := store.Create(ctx, obj, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		println("apiserver: bootstrap clusterrole:", obj.Name, err.Error())
	}
}

func ensureClusterRoleBinding(ctx context.Context, store *genericregistry.Store, crb *rbacv1.ClusterRoleBinding) {
	obj := crb.DeepCopy()
	ctx = genericapirequest.WithNamespace(ctx, metav1.NamespaceNone)
	ctx = genericapirequest.WithRequestInfo(ctx, &genericapirequest.RequestInfo{
		IsResourceRequest: true,
		Verb:              "create",
		APIGroup:          "rbac.authorization.k8s.io",
		APIVersion:        "v1",
		Resource:          "clusterrolebindings",
		Name:              obj.Name,
	})
	if _, err := store.Create(ctx, obj, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		println("apiserver: bootstrap clusterrolebinding:", obj.Name, err.Error())
	}
}

func ensureRole(ctx context.Context, store *genericregistry.Store, ns string, role *rbacv1.Role) {
	obj := role.DeepCopy()
	ctx = genericapirequest.WithNamespace(ctx, ns)
	ctx = genericapirequest.WithRequestInfo(ctx, &genericapirequest.RequestInfo{
		IsResourceRequest: true,
		Verb:              "create",
		APIGroup:          "rbac.authorization.k8s.io",
		APIVersion:        "v1",
		Resource:          "roles",
		Namespace:         ns,
		Name:              obj.Name,
	})
	if _, err := store.Create(ctx, obj, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		println("apiserver: bootstrap role:", ns+"/"+obj.Name, err.Error())
	}
}

func ensureRoleBinding(ctx context.Context, store *genericregistry.Store, ns string, rb *rbacv1.RoleBinding) {
	obj := rb.DeepCopy()
	ctx = genericapirequest.WithNamespace(ctx, ns)
	ctx = genericapirequest.WithRequestInfo(ctx, &genericapirequest.RequestInfo{
		IsResourceRequest: true,
		Verb:              "create",
		APIGroup:          "rbac.authorization.k8s.io",
		APIVersion:        "v1",
		Resource:          "rolebindings",
		Namespace:         ns,
		Name:              obj.Name,
	})
	if _, err := store.Create(ctx, obj, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		println("apiserver: bootstrap rolebinding:", ns+"/"+obj.Name, err.Error())
	}
}
