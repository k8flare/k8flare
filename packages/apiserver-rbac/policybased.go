package rbac

import (
	"context"
	"errors"

	authz "github.com/k8flare/k8flare/packages/apiserver-authz"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	kapihelper "k8s.io/kubernetes/pkg/apis/core/helper"
	internalrbac "k8s.io/kubernetes/pkg/apis/rbac"
	rbacregistry "k8s.io/kubernetes/pkg/registry/rbac"
	rbacregistryvalidation "k8s.io/kubernetes/pkg/registry/rbac/validation"
)

func init() {
	registry.Wrappers["roles"] = func(s *registry.Store, res metav1.APIResource, deps registry.Deps) rest.Storage {
		return newRoleStorage(s, res, newPolicy(deps))
	}
	registry.Wrappers["clusterroles"] = func(s *registry.Store, res metav1.APIResource, deps registry.Deps) rest.Storage {
		return newClusterRoleStorage(s, res, newPolicy(deps))
	}
	registry.Wrappers["rolebindings"] = func(s *registry.Store, res metav1.APIResource, deps registry.Deps) rest.Storage {
		return newRoleBindingStorage(s, res, newPolicy(deps))
	}
	registry.Wrappers["clusterrolebindings"] = func(s *registry.Store, res metav1.APIResource, deps registry.Deps) rest.Storage {
		return newClusterRoleBindingStorage(s, res, newPolicy(deps))
	}
}

var fullAuthority = []rbacv1.PolicyRule{
	{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}},
	{NonResourceURLs: []string{"*"}, Verbs: []string{"*"}},
}

type policy struct {
	authorizer authorizer.Authorizer
	resolver   rbacregistryvalidation.AuthorizationRuleResolver
}

func newPolicy(deps registry.Deps) *policy {
	return &policy{authorizer: authz.New(deps.Kine), resolver: authz.RuleResolver(deps.Kine)}
}

type guard struct {
	*registry.Store
	next       rest.StandardStorage
	shortNames []string
	categories []string
	trusted    func(ctx context.Context) bool
	check      func(ctx context.Context, obj, old runtime.Object) error
}

func newGuard(store *registry.Store, res metav1.APIResource, trusted func(context.Context) bool, check func(context.Context, runtime.Object, runtime.Object) error) *guard {
	return &guard{Store: store, next: store, shortNames: res.ShortNames, categories: res.Categories, trusted: trusted, check: check}
}

func (g *guard) ShortNames() []string { return g.shortNames }

func (g *guard) Categories() []string { return g.categories }

func (g *guard) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	if !g.trusted(ctx) {
		if err := g.check(ctx, obj, nil); err != nil {
			return nil, err
		}
	}
	return g.next.Create(ctx, obj, createValidation, options)
}

func (g *guard) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	if g.trusted(ctx) {
		return g.next.Update(ctx, name, objInfo, createValidation, updateValidation, forceAllowCreate, options)
	}
	nonEscalating := rest.WrapUpdatedObjectInfo(objInfo, func(ctx context.Context, obj, old runtime.Object) (runtime.Object, error) {
		if rbacregistry.IsOnlyMutatingGCFields(obj, old, kapihelper.Semantic) {
			return obj, nil
		}
		if err := g.check(ctx, obj, old); err != nil {
			return nil, err
		}
		return obj, nil
	})
	return g.next.Update(ctx, name, nonEscalating, createValidation, updateValidation, forceAllowCreate, options)
}

func groupResource(resource string) schema.GroupResource {
	return schema.GroupResource{Group: rbacv1.GroupName, Resource: resource}
}

func trustedToEscalate(ctx context.Context) bool {
	return rbacregistry.EscalationAllowed(ctx)
}

func (p *policy) trustedToEscalateRoles(ctx context.Context) bool {
	return rbacregistry.EscalationAllowed(ctx) || rbacregistry.RoleEscalationAuthorized(ctx, p.authorizer)
}

func (p *policy) confirmNoEscalation(ctx context.Context, resource, name string, rules []rbacv1.PolicyRule) error {
	if err := rbacregistryvalidation.ConfirmNoEscalation(ctx, p.resolver, rules); err != nil {
		return apierrors.NewForbidden(groupResource(resource), name, err)
	}
	return nil
}

func newRoleStorage(s *registry.Store, res metav1.APIResource, p *policy) *guard {
	return newGuard(s, res, p.trustedToEscalateRoles, func(ctx context.Context, obj, _ runtime.Object) error {
		role := obj.(*rbacv1.Role)
		return p.confirmNoEscalation(ctx, "roles", role.Name, role.Rules)
	})
}

func hasAggregationRule(role *rbacv1.ClusterRole) bool {
	return role != nil && role.AggregationRule != nil && len(role.AggregationRule.ClusterRoleSelectors) > 0
}

func newClusterRoleStorage(s *registry.Store, res metav1.APIResource, p *policy) *guard {
	return newGuard(s, res, p.trustedToEscalateRoles, func(ctx context.Context, obj, old runtime.Object) error {
		role := obj.(*rbacv1.ClusterRole)
		if err := p.confirmNoEscalation(ctx, "clusterroles", role.Name, role.Rules); err != nil {
			return err
		}
		oldRole, _ := old.(*rbacv1.ClusterRole)
		if hasAggregationRule(role) || hasAggregationRule(oldRole) {
			if err := rbacregistryvalidation.ConfirmNoEscalation(ctx, p.resolver, fullAuthority); err != nil {
				return apierrors.NewForbidden(groupResource("clusterroles"), role.Name, errors.New("must have cluster-admin privileges to use the aggregationRule"))
			}
		}
		return nil
	})
}

func (p *policy) checkBinding(ctx context.Context, resource, name string, ref rbacv1.RoleRef, namespace string) error {
	internalRef := internalrbac.RoleRef{APIGroup: ref.APIGroup, Kind: ref.Kind, Name: ref.Name}
	if rbacregistry.BindingAuthorized(ctx, internalRef, namespace, p.authorizer) {
		return nil
	}
	rules, err := p.resolver.GetRoleReferenceRules(ctx, ref, namespace)
	if err != nil {
		return err
	}
	return p.confirmNoEscalation(ctx, resource, name, rules)
}

func newRoleBindingStorage(s *registry.Store, res metav1.APIResource, p *policy) *guard {
	return newGuard(s, res, trustedToEscalate, func(ctx context.Context, obj, _ runtime.Object) error {
		namespace, ok := genericapirequest.NamespaceFrom(ctx)
		if !ok {
			return apierrors.NewBadRequest("namespace is required")
		}
		binding := obj.(*rbacv1.RoleBinding)
		return p.checkBinding(ctx, "rolebindings", binding.Name, binding.RoleRef, namespace)
	})
}

func newClusterRoleBindingStorage(s *registry.Store, res metav1.APIResource, p *policy) *guard {
	return newGuard(s, res, trustedToEscalate, func(ctx context.Context, obj, _ runtime.Object) error {
		binding := obj.(*rbacv1.ClusterRoleBinding)
		return p.checkBinding(ctx, "clusterrolebindings", binding.Name, binding.RoleRef, metav1.NamespaceNone)
	})
}
