package rbac

import (
	"context"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	rbacregistryvalidation "k8s.io/kubernetes/pkg/registry/rbac/validation"
	rbacauthorizer "k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac"
)

type recordingStorage struct {
	rest.StandardStorage
	created int
	updated int
	old     runtime.Object
}

func (r *recordingStorage) Create(_ context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	r.created++
	return obj, nil
}

func (r *recordingStorage) Update(ctx context.Context, _ string, info rest.UpdatedObjectInfo, _ rest.ValidateObjectFunc, _ rest.ValidateObjectUpdateFunc, _ bool, _ *metav1.UpdateOptions) (runtime.Object, bool, error) {
	obj, err := info.UpdatedObject(ctx, r.old)
	if err != nil {
		return nil, false, err
	}
	r.updated++
	return obj, false, nil
}

var (
	clusterAdminRules = []rbacv1.PolicyRule{
		{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}},
		{NonResourceURLs: []string{"*"}, Verbs: []string{"*"}},
	}
	podReader = rbacv1.PolicyRule{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}
	secrets   = rbacv1.PolicyRule{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}}
	masters   = &user.DefaultInfo{Name: "root", Groups: []string{user.SystemPrivilegedGroup}}
)

func rbacRule(resource, verb string, names ...string) rbacv1.PolicyRule {
	return rbacv1.PolicyRule{APIGroups: []string{rbacv1.GroupName}, Resources: []string{resource}, Verbs: []string{verb}, ResourceNames: names}
}

func staticPolicy(users map[string][]rbacv1.PolicyRule) *policy {
	roles := []*rbacv1.ClusterRole{{ObjectMeta: metav1.ObjectMeta{Name: "cluster-admin"}, Rules: clusterAdminRules}}
	var bindings []*rbacv1.ClusterRoleBinding
	for name, rules := range users {
		roles = append(roles, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: name}, Rules: rules})
		bindings = append(bindings, &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: name},
			Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: name}},
		})
	}
	resolver, static := rbacregistryvalidation.NewTestRuleResolver(nil, nil, roles, bindings)
	return &policy{authorizer: rbacauthorizer.New(static, static, static, static), resolver: resolver}
}

func requestCtx(u user.Info, resource, namespace string) context.Context {
	ctx := genericapirequest.WithUser(context.Background(), u)
	ctx = genericapirequest.WithNamespace(ctx, namespace)
	return genericapirequest.WithRequestInfo(ctx, &genericapirequest.RequestInfo{
		IsResourceRequest: true,
		APIGroup:          rbacv1.GroupName,
		Resource:          resource,
		Namespace:         namespace,
	})
}

func clusterRoleBindingTo(clusterRole string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "crb"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: clusterRole},
	}
}

func roleBindingTo(clusterRole string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rb", Namespace: "ns"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: clusterRole},
	}
}

func create(g *guard, ctx context.Context, obj runtime.Object) error {
	_, err := g.Create(ctx, obj, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	return err
}

func update(g *guard, ctx context.Context, obj runtime.Object) error {
	_, _, err := g.Update(ctx, "x", rest.DefaultUpdatedObjectInfo(obj), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
	return err
}

func assertOutcome(t *testing.T, err error, wantForbidden bool, reachedStorage bool) {
	t.Helper()
	if wantForbidden {
		if !apierrors.IsForbidden(err) {
			t.Fatalf("want Forbidden, got %v", err)
		}
		if reachedStorage {
			t.Fatal("forbidden request reached storage")
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reachedStorage {
		t.Fatal("allowed request did not reach storage")
	}
}

type outcomeCase struct {
	name          string
	user          user.Info
	wantForbidden bool
}

func TestBindingEscalation(t *testing.T) {
	p := staticPolicy(map[string][]rbacv1.PolicyRule{
		"limited": {podReader},
		"admin":   clusterAdminRules,
		"binder":  {podReader, rbacRule("clusterroles", "bind", "cluster-admin")},
	})
	cases := []outcomeCase{
		{"limited user cannot grant cluster-admin", &user.DefaultInfo{Name: "limited"}, true},
		{"holder of cluster-admin can grant it", &user.DefaultInfo{Name: "admin"}, false},
		{"bind verb permits granting", &user.DefaultInfo{Name: "binder"}, false},
		{"system:masters bypasses", masters, false},
	}
	for _, tc := range cases {
		t.Run("rolebinding create/"+tc.name, func(t *testing.T) {
			inner := &recordingStorage{}
			err := create(newRoleBindingStorage(inner, p), requestCtx(tc.user, "rolebindings", "ns"), roleBindingTo("cluster-admin"))
			assertOutcome(t, err, tc.wantForbidden, inner.created == 1)
		})
		t.Run("rolebinding update/"+tc.name, func(t *testing.T) {
			inner := &recordingStorage{old: roleBindingTo("view")}
			err := update(newRoleBindingStorage(inner, p), requestCtx(tc.user, "rolebindings", "ns"), roleBindingTo("cluster-admin"))
			assertOutcome(t, err, tc.wantForbidden, inner.updated == 1)
		})
		t.Run("clusterrolebinding create/"+tc.name, func(t *testing.T) {
			inner := &recordingStorage{}
			err := create(newClusterRoleBindingStorage(inner, p), requestCtx(tc.user, "clusterrolebindings", ""), clusterRoleBindingTo("cluster-admin"))
			assertOutcome(t, err, tc.wantForbidden, inner.created == 1)
		})
		t.Run("clusterrolebinding update/"+tc.name, func(t *testing.T) {
			inner := &recordingStorage{old: clusterRoleBindingTo("view")}
			err := update(newClusterRoleBindingStorage(inner, p), requestCtx(tc.user, "clusterrolebindings", ""), clusterRoleBindingTo("cluster-admin"))
			assertOutcome(t, err, tc.wantForbidden, inner.updated == 1)
		})
	}
}

func TestRoleEscalation(t *testing.T) {
	p := staticPolicy(map[string][]rbacv1.PolicyRule{
		"limited":   {podReader},
		"escalator": {podReader, rbacRule("clusterroles", "escalate"), rbacRule("roles", "escalate")},
		"holder":    {podReader, secrets},
	})
	cases := []outcomeCase{
		{"limited user cannot grant permissions it lacks", &user.DefaultInfo{Name: "limited"}, true},
		{"escalate verb permits it", &user.DefaultInfo{Name: "escalator"}, false},
		{"holder of the permissions can grant them", &user.DefaultInfo{Name: "holder"}, false},
		{"system:masters bypasses", masters, false},
	}
	clusterRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "reader"}, Rules: []rbacv1.PolicyRule{secrets}}
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: "ns"}, Rules: []rbacv1.PolicyRule{secrets}}
	for _, tc := range cases {
		t.Run("clusterrole create/"+tc.name, func(t *testing.T) {
			inner := &recordingStorage{}
			err := create(newClusterRoleStorage(inner, p), requestCtx(tc.user, "clusterroles", ""), clusterRole)
			assertOutcome(t, err, tc.wantForbidden, inner.created == 1)
		})
		t.Run("clusterrole update/"+tc.name, func(t *testing.T) {
			inner := &recordingStorage{old: &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "reader"}}}
			err := update(newClusterRoleStorage(inner, p), requestCtx(tc.user, "clusterroles", ""), clusterRole)
			assertOutcome(t, err, tc.wantForbidden, inner.updated == 1)
		})
		t.Run("role create/"+tc.name, func(t *testing.T) {
			inner := &recordingStorage{}
			err := create(newRoleStorage(inner, p), requestCtx(tc.user, "roles", "ns"), role)
			assertOutcome(t, err, tc.wantForbidden, inner.created == 1)
		})
		t.Run("role update/"+tc.name, func(t *testing.T) {
			inner := &recordingStorage{old: &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: "ns"}}}
			err := update(newRoleStorage(inner, p), requestCtx(tc.user, "roles", "ns"), role)
			assertOutcome(t, err, tc.wantForbidden, inner.updated == 1)
		})
	}
}

func TestAggregationRuleRequiresFullAuthority(t *testing.T) {
	p := staticPolicy(map[string][]rbacv1.PolicyRule{
		"limited": {podReader},
		"admin":   clusterAdminRules,
	})
	aggregated := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "agg"},
		AggregationRule: &rbacv1.AggregationRule{ClusterRoleSelectors: []metav1.LabelSelector{
			{MatchLabels: map[string]string{"a": "b"}},
		}},
	}
	for name, wantForbidden := range map[string]bool{"limited": true, "admin": false} {
		inner := &recordingStorage{}
		err := create(newClusterRoleStorage(inner, p), requestCtx(&user.DefaultInfo{Name: name}, "clusterroles", ""), aggregated)
		assertOutcome(t, err, wantForbidden, inner.created == 1)
	}
}

func TestGarbageCollectionUpdatesAreNotEscalation(t *testing.T) {
	p := staticPolicy(map[string][]rbacv1.PolicyRule{"limited": {podReader}})
	old := clusterRoleBindingTo("cluster-admin")
	updated := old.DeepCopy()
	updated.Finalizers = []string{"example.com/finalizer"}
	inner := &recordingStorage{old: old}
	err := update(newClusterRoleBindingStorage(inner, p), requestCtx(&user.DefaultInfo{Name: "limited"}, "clusterrolebindings", ""), updated)
	assertOutcome(t, err, false, inner.updated == 1)
}
