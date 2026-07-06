//go:build js && wasm

// S18 spike gate 1: does the real upstream RBAC stack compile for
// GOOS=js/wasm?
//
// Exercises (not just imports — the linker must keep them):
//   - k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac (RBACAuthorizer)
//   - .../rbac/bootstrappolicy (default ClusterRoles/Bindings)
//   - k8s.io/apiserver/pkg/endpoints/request (RequestInfoFactory,
//     URL -> verb/resource attribute resolution)
//   - k8s.io/kubernetes/pkg/serviceaccount (JWT TokenGenerator +
//     authenticator, the TokenRequest/TokenReview backing)
//
// This is a compile/size gate only; it is never deployed.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"net/http"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/endpoints/request"
	rbacregistryvalidation "k8s.io/kubernetes/pkg/registry/rbac/validation"
	"k8s.io/kubernetes/pkg/serviceaccount"
	rbacauthorizer "k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac"
	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy"
)

// staticRoles backs the four rbacregistryvalidation interfaces with
// in-memory slices — the same shape a store-backed implementation in
// pkg/apiserver would have.
type staticRoles struct {
	clusterRoles        []*rbacv1.ClusterRole
	clusterRoleBindings []*rbacv1.ClusterRoleBinding
	roles               []*rbacv1.Role
	roleBindings        []*rbacv1.RoleBinding
}

func (s *staticRoles) GetRole(ctx context.Context, namespace, name string) (*rbacv1.Role, error) {
	for _, r := range s.roles {
		if r.Namespace == namespace && r.Name == name {
			return r, nil
		}
	}
	return nil, fmt.Errorf("role %s/%s not found", namespace, name)
}

func (s *staticRoles) GetClusterRole(ctx context.Context, name string) (*rbacv1.ClusterRole, error) {
	for _, r := range s.clusterRoles {
		if r.Name == name {
			return r, nil
		}
	}
	return nil, fmt.Errorf("clusterrole %s not found", name)
}

func (s *staticRoles) ListRoleBindings(ctx context.Context, namespace string) ([]*rbacv1.RoleBinding, error) {
	var out []*rbacv1.RoleBinding
	for _, rb := range s.roleBindings {
		if rb.Namespace == namespace {
			out = append(out, rb)
		}
	}
	return out, nil
}

func (s *staticRoles) ListClusterRoleBindings(ctx context.Context) ([]*rbacv1.ClusterRoleBinding, error) {
	return s.clusterRoleBindings, nil
}

func main() {
	// Bootstrap policy: the real default cluster roles/bindings
	// (cluster-admin, system:masters binding, system:node, ...).
	roles := &staticRoles{
		clusterRoles:        make([]*rbacv1.ClusterRole, 0),
		clusterRoleBindings: make([]*rbacv1.ClusterRoleBinding, 0),
	}
	for i := range bootstrappolicy.ClusterRoles() {
		roles.clusterRoles = append(roles.clusterRoles, &bootstrappolicy.ClusterRoles()[i])
	}
	for i := range bootstrappolicy.ClusterRoleBindings() {
		roles.clusterRoleBindings = append(roles.clusterRoleBindings, &bootstrappolicy.ClusterRoleBindings()[i])
	}

	authz := rbacauthorizer.New(roles, roles, roles, roles)
	var _ rbacregistryvalidation.AuthorizationRuleResolver // keep the import honest

	// RequestInfoFactory: URL path -> authorizer.Attributes, same config
	// the real apiserver uses.
	rif := &request.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
	}
	req, _ := http.NewRequest("GET", "/api/v1/namespaces/default/pods/foo", nil)
	info, err := rif.NewRequestInfo(req)
	if err != nil {
		panic(err)
	}

	u := &user.DefaultInfo{Name: "admin", Groups: []string{"system:masters", "system:authenticated"}}
	attrs := authorizer.AttributesRecord{
		User:            u,
		Verb:            info.Verb,
		Namespace:       info.Namespace,
		APIGroup:        info.APIGroup,
		APIVersion:      info.APIVersion,
		Resource:        info.Resource,
		Subresource:     info.Subresource,
		Name:            info.Name,
		ResourceRequest: info.IsResourceRequest,
		Path:            req.URL.Path,
	}
	decision, reason, err := authz.Authorize(context.Background(), attrs)
	fmt.Println("decision:", decision, "reason:", reason, "err:", err)

	// ServiceAccount JWT issue + validate round-trip machinery.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	gen, err := serviceaccount.JWTTokenGenerator("https://k8flare.example", key)
	if err != nil {
		panic(err)
	}
	fmt.Println("token generator ready:", gen != nil)
}
