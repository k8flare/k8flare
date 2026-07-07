package apiserver_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// TestServiceAccountTokenRequest drives the real TokenRequest subresource
// end-to-end: mint a token for a live ServiceAccount, use it as a bearer
// token against a client built from scratch (mirroring what a real Pod's
// projected volume + client-go would do), and confirm RBAC (rbac_test.go)
// governs it exactly like any other identity.
func TestServiceAccountTokenRequest(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()

	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "sa-token-test", Namespace: "default"}}
	if _, err := client.CoreV1().ServiceAccounts("default").Create(ctx, sa, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create serviceaccount: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().ServiceAccounts("default").Delete(ctx, sa.Name, metav1.DeleteOptions{})
	})

	tr := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{Audiences: []string{"https://k8flare.internal"}},
	}
	result, err := client.CoreV1().ServiceAccounts("default").CreateToken(ctx, sa.Name, tr, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if result.Status.Token == "" {
		t.Fatalf("empty token in TokenRequest response")
	}

	saClient, err := kubernetes.NewForConfig(&rest.Config{
		Host:        fmt.Sprintf("http://127.0.0.1:%d", testPort),
		BearerToken: result.Status.Token,
	})
	if err != nil {
		t.Fatalf("build SA client: %v", err)
	}

	// No RBAC bindings yet -- discovery works (system:discovery is bound
	// to system:authenticated in the real bootstrap policy), but ordinary
	// resource access is denied.
	if _, err := saClient.Discovery().ServerVersion(); err != nil {
		t.Fatalf("SA token discovery: %v", err)
	}
	if _, err := saClient.CoreV1().Pods("default").List(ctx, metav1.ListOptions{}); err == nil {
		t.Fatalf("SA token list pods with no binding: got no error, want Forbidden")
	}

	// Grant it access via a live RoleBinding, exactly like a human/derived
	// identity in rbac_test.go -- proves this identity flows through the
	// same real RBACAuthorizer, not a separate code path.
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "sa-token-test-reader", Namespace: "default"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"},
		}},
	}
	if _, err := client.RbacV1().Roles("default").Create(ctx, role, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create role: %v", err)
	}
	t.Cleanup(func() { _ = client.RbacV1().Roles("default").Delete(ctx, role.Name, metav1.DeleteOptions{}) })

	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "sa-token-test-reader", Namespace: "default"},
		Subjects: []rbacv1.Subject{{
			Kind: rbacv1.ServiceAccountKind, Namespace: "default", Name: sa.Name,
		}},
		RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: role.Name},
	}
	if _, err := client.RbacV1().RoleBindings("default").Create(ctx, binding, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create rolebinding: %v", err)
	}
	t.Cleanup(func() {
		_ = client.RbacV1().RoleBindings("default").Delete(ctx, binding.Name, metav1.DeleteOptions{})
	})

	if _, err := saClient.CoreV1().Pods("default").List(ctx, metav1.ListOptions{}); err != nil {
		t.Fatalf("SA token list pods after binding: %v", err)
	}

	// A malformed/unknown bearer token is neither the cluster token nor a
	// valid SA JWT -- must still 401, not fall through to some default identity.
	req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/v1/namespaces/default/pods", testPort), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer not-a-real-token-at-all")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("garbage bearer token: got %d, want 401", resp.StatusCode)
	}
}
