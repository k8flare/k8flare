package authz

import (
	"context"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
	rbacregistryvalidation "k8s.io/kubernetes/pkg/registry/rbac/validation"
	rbacauthorizer "k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac"
	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy"
)

func init() {
	utilruntime.Must(rbacv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(corev1.AddToScheme(scheme.Scheme))
}

var nodeClusterRoles = []string{"system:node-proxier"}

var (
	bootstrapClusterRoles        = bootstrappolicy.ClusterRoles()
	bootstrapClusterRoleBindings = bootstrappolicy.ClusterRoleBindings()
	bootstrapNamespaceRoles      = bootstrappolicy.NamespaceRoles()
	bootstrapNamespaceBindings   = bootstrappolicy.NamespaceRoleBindings()
)

func New(client *kine.Client) authorizer.Authorizer {
	return chain{authorizerfactory.NewPrivilegedGroups(user.SystemPrivilegedGroup), newNodeAuthorizer(client), rbacFor(client)}
}

func Resolver(client *kine.Client) authorizer.RuleResolver {
	return rbacFor(client)
}

func RuleResolver(client *kine.Client) rbacregistryvalidation.AuthorizationRuleResolver {
	p := &policy{client: client, codec: scheme.Codecs.LegacyCodec(rbacv1.SchemeGroupVersion)}
	return rbacregistryvalidation.NewDefaultRuleResolver(p, p, p, p)
}

func rbacFor(client *kine.Client) *rbacauthorizer.RBACAuthorizer {
	p := &policy{client: client, codec: scheme.Codecs.LegacyCodec(rbacv1.SchemeGroupVersion)}
	return rbacauthorizer.New(p, p, p, p)
}

type chain []authorizer.Authorizer

func (c chain) Authorize(ctx context.Context, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	for _, a := range c {
		d, reason, err := a.Authorize(ctx, attrs)
		if err != nil {
			return d, reason, err
		}
		if d != authorizer.DecisionNoOpinion {
			return d, reason, nil
		}
	}
	return authorizer.DecisionNoOpinion, "", nil
}

type policy struct {
	client *kine.Client
	codec  runtime.Codec
}

func notFound(resource, name string) error {
	return apierrors.NewNotFound(schema.GroupResource{Group: rbacv1.GroupName, Resource: resource}, name)
}

func (p *policy) clusterRoles(ctx context.Context) ([]*rbacv1.ClusterRole, error) {
	list := &rbacv1.ClusterRoleList{}
	s := kine.NewStorage(p.client, p.codec, func() runtime.Object { return &rbacv1.ClusterRole{} })
	if err := s.GetList(ctx, "/clusterroles", storage.ListOptions{Recursive: true, Predicate: storage.Everything}, list); err != nil {
		return nil, err
	}
	roles := make([]*rbacv1.ClusterRole, 0, len(list.Items)+len(bootstrapClusterRoles))
	for i := range list.Items {
		roles = append(roles, &list.Items[i])
	}
	for i := range bootstrapClusterRoles {
		roles = append(roles, &bootstrapClusterRoles[i])
	}
	return roles, nil
}

func resolvedRules(ctx context.Context, all []*rbacv1.ClusterRole, role *rbacv1.ClusterRole) ([]rbacv1.PolicyRule, error) {
	if role.AggregationRule == nil {
		return role.Rules, nil
	}
	var rules []rbacv1.PolicyRule
	for _, sel := range role.AggregationRule.ClusterRoleSelectors {
		selector, err := metav1.LabelSelectorAsSelector(&sel)
		if err != nil {
			return nil, err
		}
		for _, r := range all {
			if selector.Matches(labels.Set(r.Labels)) {
				rules = append(rules, r.Rules...)
			}
		}
	}
	return rules, nil
}

func (p *policy) GetRole(ctx context.Context, namespace, name string) (*rbacv1.Role, error) {
	s := kine.NewStorage(p.client, p.codec, func() runtime.Object { return &rbacv1.Role{} })
	role := &rbacv1.Role{}
	if err := s.Get(ctx, "/roles/"+namespace+"/"+name, storage.GetOptions{}, role); err == nil {
		return role, nil
	}
	for i, r := range bootstrapNamespaceRoles[namespace] {
		if r.Name == name {
			return &bootstrapNamespaceRoles[namespace][i], nil
		}
	}
	return nil, notFound("roles", name)
}

func (p *policy) ListRoleBindings(ctx context.Context, namespace string) ([]*rbacv1.RoleBinding, error) {
	list := &rbacv1.RoleBindingList{}
	s := kine.NewStorage(p.client, p.codec, func() runtime.Object { return &rbacv1.RoleBinding{} })
	if err := s.GetList(ctx, "/rolebindings/"+namespace, storage.ListOptions{Recursive: true, Predicate: storage.Everything}, list); err != nil {
		return nil, err
	}
	bindings := make([]*rbacv1.RoleBinding, 0, len(list.Items))
	for i := range list.Items {
		bindings = append(bindings, &list.Items[i])
	}
	for i := range bootstrapNamespaceBindings[namespace] {
		bindings = append(bindings, &bootstrapNamespaceBindings[namespace][i])
	}
	return bindings, nil
}

func (p *policy) GetClusterRole(ctx context.Context, name string) (*rbacv1.ClusterRole, error) {
	all, err := p.clusterRoles(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range all {
		if r.Name != name {
			continue
		}
		rules, err := resolvedRules(ctx, all, r)
		if err != nil {
			return nil, err
		}
		resolved := r.DeepCopy()
		resolved.Rules = rules
		return resolved, nil
	}
	return nil, notFound("clusterroles", name)
}

func (p *policy) ListClusterRoleBindings(ctx context.Context) ([]*rbacv1.ClusterRoleBinding, error) {
	list := &rbacv1.ClusterRoleBindingList{}
	s := kine.NewStorage(p.client, p.codec, func() runtime.Object { return &rbacv1.ClusterRoleBinding{} })
	if err := s.GetList(ctx, "/clusterrolebindings", storage.ListOptions{Recursive: true, Predicate: storage.Everything}, list); err != nil {
		return nil, err
	}
	bindings := make([]*rbacv1.ClusterRoleBinding, 0, len(list.Items)+len(bootstrapClusterRoleBindings)+len(nodeClusterRoles))
	for i := range list.Items {
		bindings = append(bindings, &list.Items[i])
	}
	for i := range bootstrapClusterRoleBindings {
		bindings = append(bindings, &bootstrapClusterRoleBindings[i])
	}
	for _, role := range nodeClusterRoles {
		bindings = append(bindings, &rbacv1.ClusterRoleBinding{
			RoleRef:  rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role},
			Subjects: []rbacv1.Subject{{Kind: rbacv1.GroupKind, APIGroup: rbacv1.GroupName, Name: user.NodesGroup}},
		})
	}
	return bindings, nil
}
