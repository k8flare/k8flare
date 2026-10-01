package rbac

import (
	"context"
	"errors"
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
	hash := rbacBootstrapHash()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			ctx := context.WithoutCancel(r.Context())
			_ = registry.RunBootstrap(ctx, clusterRoles, "rbac", hash, func(ctx context.Context) error {
				return ensureRBAC(ctx, clusterRoles, clusterRoleBindings, roles, roleBindings)
			})
		})
		next.ServeHTTP(w, r)
	})
}

func rbacBootstrapHash() string {
	return registry.HashObjects(
		bootstrappolicy.ClusterRoles(),
		bootstrappolicy.ControllerRoles(),
		bootstrappolicy.ClusterRoleBindings(),
		bootstrappolicy.ControllerRoleBindings(),
		bootstrappolicy.NamespaceRoles(),
		bootstrappolicy.NamespaceRoleBindings(),
	)
}

func ensureRBAC(ctx context.Context, clusterRoles, clusterRoleBindings, roles, roleBindings *genericregistry.Store) error {
	var failed error
	for _, cr := range append(bootstrappolicy.ClusterRoles(), bootstrappolicy.ControllerRoles()...) {
		failed = errors.Join(failed, ensureClusterRole(ctx, clusterRoles, &cr))
	}
	for _, crb := range append(bootstrappolicy.ClusterRoleBindings(), bootstrappolicy.ControllerRoleBindings()...) {
		failed = errors.Join(failed, ensureClusterRoleBinding(ctx, clusterRoleBindings, &crb))
	}
	if roles != nil {
		for ns, list := range bootstrappolicy.NamespaceRoles() {
			for i := range list {
				failed = errors.Join(failed, ensureRole(ctx, roles, ns, &list[i]))
			}
		}
	}
	if roleBindings != nil {
		for ns, list := range bootstrappolicy.NamespaceRoleBindings() {
			for i := range list {
				failed = errors.Join(failed, ensureRoleBinding(ctx, roleBindings, ns, &list[i]))
			}
		}
	}
	return failed
}

func ensureClusterRole(ctx context.Context, store *genericregistry.Store, cr *rbacv1.ClusterRole) error {
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
		return err
	}
	return nil
}

func ensureClusterRoleBinding(ctx context.Context, store *genericregistry.Store, crb *rbacv1.ClusterRoleBinding) error {
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
		return err
	}
	return nil
}

func ensureRole(ctx context.Context, store *genericregistry.Store, ns string, role *rbacv1.Role) error {
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
		return err
	}
	return nil
}

func ensureRoleBinding(ctx context.Context, store *genericregistry.Store, ns string, rb *rbacv1.RoleBinding) error {
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
		return err
	}
	return nil
}
