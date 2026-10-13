package admissionregistration

import (
	"context"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
)

var (
	paramReader   = &user.DefaultInfo{Name: "reader", Groups: []string{user.AllAuthenticated}}
	paramOutsider = &user.DefaultInfo{Name: "outsider", Groups: []string{user.AllAuthenticated}}
)

func seed(t *testing.T, client *kine.Client, key string, obj runtime.Object, gv schema.GroupVersion) {
	t.Helper()
	s := kine.NewStorage(client, scheme.Codecs.LegacyCodec(gv), func() runtime.Object { return obj.DeepCopyObject() })
	if err := s.Create(context.Background(), key, obj, obj.DeepCopyObject(), 0); err != nil {
		t.Fatal(err)
	}
}

func paramStore(t *testing.T, name, kind string) *registry.Store {
	t.Helper()
	cs := registrytest.NewCountingStore(t)
	rbacGV := rbacv1.SchemeGroupVersion
	seed(t, cs.Client(), "/clusterroles/param-reader", &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "param-reader"},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get", "list"}, APIGroups: []string{""}, Resources: []string{"configmaps"}}},
	}, rbacGV)
	seed(t, cs.Client(), "/clusterrolebindings/param-reader", &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "param-reader"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "param-reader"},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: "reader"}},
	}, rbacGV)
	policyGV := admissionregv1.SchemeGroupVersion
	validating := policy()
	validating.Name = "policy"
	validating.Spec.ParamKind = &admissionregv1.ParamKind{APIVersion: "v1", Kind: "ConfigMap"}
	seed(t, cs.Client(), "/validatingadmissionpolicies/policy", validating, policyGV)
	mutating := mutatingPolicy()
	mutating.Name = "policy"
	seed(t, cs.Client(), "/mutatingadmissionpolicies/policy", mutating, policyGV)
	store := cs.NewStore(t, schema.GroupVersion{Group: "admissionregistration.k8s.io", Version: "v1"}, metav1.APIResource{Name: name, Kind: kind})
	registry.Customizers[name](store, registry.Deps{Admission: admissionClient(t), Kine: cs.Client()})
	return store
}

func mutatingPolicy() *admissionregv1.MutatingAdmissionPolicy {
	return &admissionregv1.MutatingAdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "mutate.example.com"},
		Spec: admissionregv1.MutatingAdmissionPolicySpec{
			ParamKind:     &admissionregv1.ParamKind{APIVersion: "v1", Kind: "ConfigMap"},
			FailurePolicy: ptr.To(admissionregv1.Fail),
			MatchConstraints: &admissionregv1.MatchResources{
				MatchPolicy:       ptr.To(admissionregv1.Equivalent),
				NamespaceSelector: &metav1.LabelSelector{},
				ObjectSelector:    &metav1.LabelSelector{},
				ResourceRules: []admissionregv1.NamedRuleWithOperations{{
					RuleWithOperations: admissionregv1.RuleWithOperations{
						Operations: []admissionregv1.OperationType{admissionregv1.Create},
						Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
					},
				}},
			},
			ReinvocationPolicy: admissionregv1.NeverReinvocationPolicy,
			Mutations: []admissionregv1.Mutation{{
				PatchType:          admissionregv1.PatchTypeApplyConfiguration,
				ApplyConfiguration: &admissionregv1.ApplyConfiguration{Expression: "Object{}"},
			}},
		},
	}
}

func asUser(store *registry.Store, who user.Info) context.Context {
	return genericapirequest.WithUser(registrytest.Context(store), who)
}

func createAs(store *registry.Store, who user.Info, obj runtime.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	rest.FillObjectMetaSystemFields(accessor)
	return rest.BeforeCreate(store.CreateStrategy, asUser(store, who), obj)
}

func updateAs(store *registry.Store, who user.Info, obj, old runtime.Object) error {
	ctx := asUser(store, who)
	store.UpdateStrategy.PrepareForUpdate(ctx, obj, old)
	return rest.BeforeUpdate(store.UpdateStrategy, ctx, obj, old)
}

func TestPolicyParamKindNeedsReadAccess(t *testing.T) {
	cases := []struct {
		name, resource, kind, field string
		object                      func() runtime.Object
	}{
		{"ValidatingAdmissionPolicy", "validatingadmissionpolicies", "ValidatingAdmissionPolicy", "spec.paramKind", func() runtime.Object {
			p := policy()
			p.Spec.ParamKind = &admissionregv1.ParamKind{APIVersion: "v1", Kind: "ConfigMap"}
			return p
		}},
		{"MutatingAdmissionPolicy", "mutatingadmissionpolicies", "MutatingAdmissionPolicy", "spec.paramKind", func() runtime.Object { return mutatingPolicy() }},
		{"ValidatingAdmissionPolicyBinding", "validatingadmissionpolicybindings", "ValidatingAdmissionPolicyBinding", "spec.paramRef", func() runtime.Object {
			return &admissionregv1.ValidatingAdmissionPolicyBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "binding.example.com"},
				Spec: admissionregv1.ValidatingAdmissionPolicyBindingSpec{
					PolicyName:        "policy",
					ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny},
					ParamRef:          &admissionregv1.ParamRef{Name: "cfg", Namespace: "default", ParameterNotFoundAction: ptr.To(admissionregv1.DenyAction)},
				},
			}
		}},
		{"MutatingAdmissionPolicyBinding", "mutatingadmissionpolicybindings", "MutatingAdmissionPolicyBinding", "spec.paramRef", func() runtime.Object {
			return &admissionregv1.MutatingAdmissionPolicyBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "binding.example.com"},
				Spec: admissionregv1.MutatingAdmissionPolicyBindingSpec{
					PolicyName: "policy",
					ParamRef:   &admissionregv1.ParamRef{Name: "cfg", Namespace: "default", ParameterNotFoundAction: ptr.To(admissionregv1.DenyAction)},
				},
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := paramStore(t, c.resource, c.kind)
			err := createAs(store, paramOutsider, c.object())
			registrytest.RequireFieldError(t, err, c.field, "")
			if err := createAs(store, paramReader, c.object()); err != nil {
				t.Fatalf("a user who can read the param kind must be accepted: %v", err)
			}
		})
	}
}

func TestPolicyUpdateAuthorizesOnlyAChangedParamKind(t *testing.T) {
	store := paramStore(t, "validatingadmissionpolicies", "ValidatingAdmissionPolicy")
	old := policy()
	old.ResourceVersion = "1"
	old.Spec.ParamKind = &admissionregv1.ParamKind{APIVersion: "v1", Kind: "ConfigMap"}
	same := old.DeepCopy()
	if err := updateAs(store, paramOutsider, same, old); err != nil {
		t.Fatalf("an unchanged paramKind needs no read access: %v", err)
	}
	changed := old.DeepCopy()
	changed.Spec.ParamKind = &admissionregv1.ParamKind{APIVersion: "v1", Kind: "Secret"}
	err := updateAs(store, paramOutsider, changed, old)
	registrytest.RequireFieldError(t, err, "spec.paramKind", "")
}
