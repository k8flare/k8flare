package apiserver

import (
	"context"
	"net/http"
	"sync"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	rbacauthorizer "k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac"
	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy"
)

// RBAC enforcement, built from upstream's real pieces rather than a
// reimplementation (inviolable rule #3): the actual RBACAuthorizer
// (plugin/pkg/auth/authorizer/rbac) evaluating the actual bootstrap
// policy (bootstrappolicy) unioned with the live Role/RoleBinding/
// ClusterRole/ClusterRoleBinding objects users create through this
// apiserver's own stores.
//
// Two deliberate deviations from a stock kube-apiserver, both recorded
// here rather than hidden:
//
//  1. Bootstrap policy is unioned at READ time inside the authorizer's
//     getters instead of being reconciled into storage at startup. A
//     stock apiserver writes ~80 default ClusterRoles/Bindings into
//     etcd on boot; per-request instantiation (S19) would turn that
//     into a reconcile check on every cold start, and multi-cluster
//     would pay it per cluster. Consequence: the defaults authorize
//     exactly like upstream but are not visible to
//     `kubectl get clusterroles`.
//  2. There is no Node authorizer. Upstream stopped binding the
//     system:node ClusterRole to the system:nodes group in 1.8 in
//     favor of the Node authorizer + NodeRestriction admission; this
//     stack authenticates kubelets as the system:nodes group via the
//     shared cluster token, so it restores the pre-1.8 group bindings
//     (system:node, system:node-proxier) as static policy. Same
//     trust level as before RBAC landed -- per-node identity is a
//     TLS-client-cert follow-up, not part of this change.
//
// The all-powerful cluster token maps to the system:masters group
// (auth.go), which short-circuits below exactly like kube-apiserver's
// SystemPrivilegedGroup bypass -- zero storage reads on the paths
// kubectl-as-admin, the KCM, the scheduler, and the kubelet bridge
// actually take today. RBAC evaluation with its storage reads only
// runs for derived identities (X-Remote-User today, ServiceAccount
// tokens when TokenRequest lands).

// rbacPolicy implements the four validation interfaces the real
// RBACAuthorizer resolves rules through, backed by this apiserver's
// stores unioned with the static bootstrap policy.
type rbacPolicy struct {
	stores map[string]*ResourceStore // storesByGV[rbacv1] map: roles, rolebindings, clusterroles, clusterrolebindings
}

var (
	rbacBootstrapOnce            sync.Once
	bootstrapClusterRoles        map[string]*rbacv1.ClusterRole
	bootstrapClusterRoleBindings []*rbacv1.ClusterRoleBinding
	bootstrapNamespaceRoles      map[string]map[string]*rbacv1.Role
	bootstrapNamespaceBindings   map[string][]*rbacv1.RoleBinding
)

// nodeGroupBindings restores the pre-1.8 group bindings for kubelets and
// kube-proxy (deviation #2 above).
func nodeGroupBindings() []*rbacv1.ClusterRoleBinding {
	mk := func(role string) *rbacv1.ClusterRoleBinding {
		return &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "k8flare:" + role},
			Subjects:   []rbacv1.Subject{{Kind: rbacv1.GroupKind, APIGroup: rbacv1.GroupName, Name: user.NodesGroup}},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role},
		}
	}
	return []*rbacv1.ClusterRoleBinding{mk("system:node"), mk("system:node-proxier")}
}

func loadBootstrapPolicy() {
	rbacBootstrapOnce.Do(func() {
		bootstrapClusterRoles = map[string]*rbacv1.ClusterRole{}
		roles := bootstrappolicy.ClusterRoles()
		for i := range roles {
			bootstrapClusterRoles[roles[i].Name] = &roles[i]
		}
		// ControllerRoles cover the system:controller:* identities the
		// real KCM uses when running with per-controller credentials --
		// harmless today (our KCM presents system:masters) and required
		// the day it switches to ServiceAccount tokens.
		ctrl := bootstrappolicy.ControllerRoles()
		for i := range ctrl {
			bootstrapClusterRoles[ctrl[i].Name] = &ctrl[i]
		}
		crbs := bootstrappolicy.ClusterRoleBindings()
		for i := range crbs {
			bootstrapClusterRoleBindings = append(bootstrapClusterRoleBindings, &crbs[i])
		}
		ctrlB := bootstrappolicy.ControllerRoleBindings()
		for i := range ctrlB {
			bootstrapClusterRoleBindings = append(bootstrapClusterRoleBindings, &ctrlB[i])
		}
		bootstrapClusterRoleBindings = append(bootstrapClusterRoleBindings, nodeGroupBindings()...)

		bootstrapNamespaceRoles = map[string]map[string]*rbacv1.Role{}
		for ns, roles := range bootstrappolicy.NamespaceRoles() {
			m := map[string]*rbacv1.Role{}
			for i := range roles {
				m[roles[i].Name] = &roles[i]
			}
			bootstrapNamespaceRoles[ns] = m
		}
		bootstrapNamespaceBindings = map[string][]*rbacv1.RoleBinding{}
		for ns, rbs := range bootstrappolicy.NamespaceRoleBindings() {
			for i := range rbs {
				bootstrapNamespaceBindings[ns] = append(bootstrapNamespaceBindings[ns], &rbs[i])
			}
		}
	})
}

func (p *rbacPolicy) GetRole(ctx context.Context, namespace, name string) (*rbacv1.Role, error) {
	if rs := p.stores["roles"]; rs != nil {
		if obj, err := rs.Get(ctx, namespace, name); err == nil {
			if role, ok := obj.(*rbacv1.Role); ok {
				return role, nil
			}
		}
	}
	loadBootstrapPolicy()
	if role, ok := bootstrapNamespaceRoles[namespace][name]; ok {
		return role, nil
	}
	return nil, apierrors.NewNotFound(rbacv1.Resource("role"), name)
}

func (p *rbacPolicy) GetClusterRole(ctx context.Context, name string) (*rbacv1.ClusterRole, error) {
	if rs := p.stores["clusterroles"]; rs != nil {
		if obj, err := rs.Get(ctx, "", name); err == nil {
			if role, ok := obj.(*rbacv1.ClusterRole); ok {
				return role, nil
			}
		}
	}
	loadBootstrapPolicy()
	if role, ok := bootstrapClusterRoles[name]; ok {
		return role, nil
	}
	return nil, apierrors.NewNotFound(rbacv1.Resource("clusterrole"), name)
}

func (p *rbacPolicy) ListRoleBindings(ctx context.Context, namespace string) ([]*rbacv1.RoleBinding, error) {
	loadBootstrapPolicy()
	var out []*rbacv1.RoleBinding
	out = append(out, bootstrapNamespaceBindings[namespace]...)
	if rs := p.stores["rolebindings"]; rs != nil {
		if obj, err := rs.List(ctx, namespace, "", ""); err == nil {
			if list, ok := obj.(*rbacv1.RoleBindingList); ok {
				for i := range list.Items {
					out = append(out, &list.Items[i])
				}
			}
		}
	}
	return out, nil
}

func (p *rbacPolicy) ListClusterRoleBindings(ctx context.Context) ([]*rbacv1.ClusterRoleBinding, error) {
	loadBootstrapPolicy()
	var out []*rbacv1.ClusterRoleBinding
	out = append(out, bootstrapClusterRoleBindings...)
	if rs := p.stores["clusterrolebindings"]; rs != nil {
		if obj, err := rs.List(ctx, "", "", ""); err == nil {
			if list, ok := obj.(*rbacv1.ClusterRoleBindingList); ok {
				for i := range list.Items {
					out = append(out, &list.Items[i])
				}
			}
		}
	}
	return out, nil
}

// NewRBACAuthorizer builds the real upstream RBACAuthorizer over this
// apiserver's rbac.authorization.k8s.io/v1 stores (the map produced by
// NewResourceStoresForGroupVersion for that GroupVersion).
func NewRBACAuthorizer(rbacStores map[string]*ResourceStore) authorizer.Authorizer {
	p := &rbacPolicy{stores: rbacStores}
	return rbacauthorizer.New(p, p, p, p)
}

// requestInfoFactory parses API paths exactly the way kube-apiserver
// does (verb mapping, subresources, namespaces) -- reused, not rewritten.
var requestInfoFactory = &apirequest.RequestInfoFactory{
	APIPrefixes:          sets.NewString("api", "apis"),
	GrouplessAPIPrefixes: sets.NewString("api"),
}

// authorizeRequest evaluates one request for one user. Exposed for the
// SAR/SSAR handlers so `kubectl auth can-i` answers from the very same
// authorizer that gates real traffic.
func authorizeRequest(ctx context.Context, authz authorizer.Authorizer, u *UserInfo, r *http.Request) (authorizer.Decision, string, error) {
	info := &user.DefaultInfo{Name: u.Name, Groups: u.Groups}
	for _, g := range u.Groups {
		// kube-apiserver's SystemPrivilegedGroup bypass, verbatim: the
		// cluster token's identity never touches RBAC storage.
		if g == user.SystemPrivilegedGroup {
			return authorizer.DecisionAllow, "system:masters bypass", nil
		}
	}
	ri, err := requestInfoFactory.NewRequestInfo(r)
	if err != nil {
		return authorizer.DecisionDeny, "could not parse request info", err
	}
	attrs := authorizer.AttributesRecord{
		User:            info,
		Verb:            ri.Verb,
		Namespace:       ri.Namespace,
		APIGroup:        ri.APIGroup,
		APIVersion:      ri.APIVersion,
		Resource:        ri.Resource,
		Subresource:     ri.Subresource,
		Name:            ri.Name,
		ResourceRequest: ri.IsResourceRequest,
		Path:            ri.Path,
	}
	return authz.Authorize(ctx, attrs)
}

// AuthzMiddleware enforces RBAC between AuthMiddleware (which set the
// user) and the resource handlers. Requests with no user in context
// never reach here (AuthMiddleware 401s them first).
func AuthzMiddleware(authz authorizer.Authorizer, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFromContext(r.Context())
		if u == nil {
			writeJSON(w, http.StatusUnauthorized, statusResponse{
				Kind: "Status", APIVersion: "v1", Metadata: map[string]string{},
				Status: "Failure", Message: "Unauthorized", Reason: "Unauthorized", Code: 401,
			})
			return
		}
		decision, reason, err := authorizeRequest(r.Context(), authz, u, r)
		if err != nil || decision != authorizer.DecisionAllow {
			msg := "forbidden: User \"" + u.Name + "\" cannot perform this action"
			if reason != "" {
				msg += ": " + reason
			}
			writeJSON(w, http.StatusForbidden, statusResponse{
				Kind: "Status", APIVersion: "v1", Metadata: map[string]string{},
				Status: "Failure", Message: msg, Reason: "Forbidden", Code: 403,
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
