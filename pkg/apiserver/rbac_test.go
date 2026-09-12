package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// rbacDo sends one request as a derived identity: a real ServiceAccount
// JWT bearer token (see mintDerivedIdentity), or the empty string for the
// admin identity (the bare cluster token).
func rbacDo(t *testing.T, method, path, bearerToken, body string) (*http.Response, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", testPort, path), rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	token := bearerToken
	if token == "" {
		token = "k8flare-dev-token"
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

func mintDerivedIdentity(t *testing.T, client *kubernetes.Clientset, ns, name string) (string, rbacv1.Subject) {
	t.Helper()
	ctx := context.Background()
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	if _, err := client.CoreV1().ServiceAccounts(ns).Create(ctx, sa, metav1.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("create serviceaccount %s/%s: %v", ns, name, err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().ServiceAccounts(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
	})
	tr := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{Audiences: []string{"https://k8flare.internal"}},
	}
	result, err := client.CoreV1().ServiceAccounts(ns).CreateToken(ctx, name, tr, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("CreateToken for %s/%s: %v", ns, name, err)
	}
	return result.Status.Token, rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Namespace: ns, Name: name}
}

// TestRBACEnforcement drives the real RBACAuthorizer end-to-end: a
// derived identity is denied by default, gains exactly what a live
// Role/RoleBinding grants, and `kubectl auth can-i` (SSAR) agrees with
// enforcement. The cluster token (system:masters) stays all-powerful.
func TestRBACEnforcement(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	bearer, subject := mintDerivedIdentity(t, client, "default", "rbac-test-alice")

	// The admin identity (bare cluster token) bypasses RBAC.
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/default/pods", "", ""); resp.StatusCode != 200 {
		t.Fatalf("admin list pods: got %d, want 200: %s", resp.StatusCode, body)
	}

	// A derived identity with no bindings is denied...
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/default/pods", bearer, ""); resp.StatusCode != 403 {
		t.Fatalf("unbound user list pods: got %d, want 403: %s", resp.StatusCode, body)
	}
	// ...but can hit discovery (system:discovery -> system:authenticated,
	// straight from the real bootstrap policy).
	if resp, body := rbacDo(t, "GET", "/api/v1", bearer, ""); resp.StatusCode != 200 {
		t.Fatalf("unbound user discovery: got %d, want 200: %s", resp.StatusCode, body)
	}

	// Grant get/list/watch pods in default via live objects.
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "rbac-test-pod-reader", Namespace: "default"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch"},
		}},
	}
	if _, err := client.RbacV1().Roles("default").Create(ctx, role, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create role: %v", err)
	}
	t.Cleanup(func() {
		_ = client.RbacV1().Roles("default").Delete(ctx, role.Name, metav1.DeleteOptions{})
	})
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rbac-test-pod-reader", Namespace: "default"},
		Subjects:   []rbacv1.Subject{subject},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: role.Name},
	}
	if _, err := client.RbacV1().RoleBindings("default").Create(ctx, binding, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create rolebinding: %v", err)
	}
	t.Cleanup(func() {
		_ = client.RbacV1().RoleBindings("default").Delete(ctx, binding.Name, metav1.DeleteOptions{})
	})

	// Granted verbs work; everything else stays denied.
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/default/pods", bearer, ""); resp.StatusCode != 200 {
		t.Fatalf("bound user list pods: got %d, want 200: %s", resp.StatusCode, body)
	}
	pod := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"rbac-test-denied"},"spec":{"containers":[{"name":"c","image":"busybox"}]}}`
	if resp, body := rbacDo(t, "POST", "/api/v1/namespaces/default/pods", bearer, pod); resp.StatusCode != 403 {
		t.Fatalf("bound user create pod: got %d, want 403: %s", resp.StatusCode, body)
	}
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/kube-system/pods", bearer, ""); resp.StatusCode != 403 {
		t.Fatalf("bound user list pods in kube-system: got %d, want 403: %s", resp.StatusCode, body)
	}
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/default/secrets", bearer, ""); resp.StatusCode != 403 {
		t.Fatalf("bound user list secrets: got %d, want 403: %s", resp.StatusCode, body)
	}

	// `kubectl auth can-i` (SSAR) answers from the same authorizer.
	ssar := func(verb, resource string) bool {
		body := fmt.Sprintf(`{"apiVersion":"authorization.k8s.io/v1","kind":"SelfSubjectAccessReview","spec":{"resourceAttributes":{"verb":%q,"resource":%q,"namespace":"default"}}}`, verb, resource)
		resp, out := rbacDo(t, "POST", "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews", bearer, body)
		if resp.StatusCode != 201 {
			t.Fatalf("ssar %s %s: got %d: %s", verb, resource, resp.StatusCode, out)
		}
		var parsed struct {
			Status struct {
				Allowed bool `json:"allowed"`
			} `json:"status"`
		}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("ssar decode: %v: %s", err, out)
		}
		return parsed.Status.Allowed
	}
	if !ssar("get", "pods") {
		t.Fatalf("can-i get pods: got false, want true")
	}
	if ssar("delete", "pods") {
		t.Fatalf("can-i delete pods: got true, want false")
	}

	// The kubelet identity (Basic node:<token>) reads nodes through the
	// restored system:node group binding.
	req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/v1/nodes", testPort), nil)
	if err != nil {
		t.Fatalf("new node request: %v", err)
	}
	req.SetBasicAuth("node", "k8flare-dev-token")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("node list nodes: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("node identity list nodes: got %d, want 200", resp.StatusCode)
	}
}

// TestClusterAdminBootstrapRoles covers the k8flare:* roles rbac.go ships
// (bundled the same read-time way as upstream's system:* ones, so they are
// bindable without being visible to `kubectl get clusterroles`). Binding
// them to a derived identity must grant exactly cluster administration:
// Cluster objects, plus the credential Secrets in k8flare-system and
// nothing outside it.
//
// This proves the roles GRANT what they claim. It does not make them a
// confinement boundary: the cluster token is still system:masters and
// bypasses RBAC entirely (docs/cluster-api-design.md's caveat).
func TestClusterAdminBootstrapRoles(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	const ns = "k8flare-system"
	bearer, subject := mintDerivedIdentity(t, client, "default", "rbac-test-cluster-admin")

	// Unbound: no access to either half.
	if resp, body := rbacDo(t, "GET", "/apis/k8flare.com/v1alpha1/clusters", bearer, ""); resp.StatusCode != 403 {
		t.Fatalf("unbound user list clusters: got %d, want 403: %s", resp.StatusCode, body)
	}

	if _, err := client.CoreV1().Namespaces().Create(ctx,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{},
	); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("create %s namespace: %v", ns, err)
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rbac-test-cluster-admin"},
		Subjects:   []rbacv1.Subject{subject},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "k8flare:cluster-admin"},
	}
	if _, err := client.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create clusterrolebinding: %v", err)
	}
	t.Cleanup(func() {
		_ = client.RbacV1().ClusterRoleBindings().Delete(context.Background(), crb.Name, metav1.DeleteOptions{})
	})
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rbac-test-cluster-secrets", Namespace: ns},
		Subjects:   []rbacv1.Subject{subject},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "k8flare:cluster-secret-reader"},
	}
	if _, err := client.RbacV1().RoleBindings(ns).Create(ctx, rb, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create rolebinding: %v", err)
	}
	t.Cleanup(func() {
		_ = client.RbacV1().RoleBindings(ns).Delete(context.Background(), rb.Name, metav1.DeleteOptions{})
	})

	// The bundled ClusterRole is resolvable even though no such object
	// exists in storage, and grants the whole Cluster lifecycle.
	if resp, body := rbacDo(t, "GET", "/apis/k8flare.com/v1alpha1/clusters", bearer, ""); resp.StatusCode != 200 {
		t.Fatalf("bound user list clusters: got %d, want 200: %s", resp.StatusCode, body)
	}
	cluster := `{"apiVersion":"k8flare.com/v1alpha1","kind":"Cluster","metadata":{"name":"rbac-test-issued"},"spec":{"displayName":"issued by a bound admin"}}`
	if resp, body := rbacDo(t, "POST", "/apis/k8flare.com/v1alpha1/clusters", bearer, cluster); resp.StatusCode != 201 {
		t.Fatalf("bound user create cluster: got %d, want 201: %s", resp.StatusCode, body)
	}
	t.Cleanup(func() {
		_, _ = rbacDo(t, "DELETE", "/apis/k8flare.com/v1alpha1/clusters/rbac-test-issued", bearer, "")
	})

	// Credential Secrets in k8flare-system only -- the namespaced Role is
	// the whole point of not granting secrets cluster-wide.
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/"+ns+"/secrets", bearer, ""); resp.StatusCode != 200 {
		t.Fatalf("bound user list %s secrets: got %d, want 200: %s", ns, resp.StatusCode, body)
	}
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/default/secrets", bearer, ""); resp.StatusCode != 403 {
		t.Fatalf("bound user list default secrets: got %d, want 403: %s", resp.StatusCode, body)
	}
	if resp, body := rbacDo(t, "DELETE", "/api/v1/namespaces/"+ns+"/secrets/nonexistent", bearer, ""); resp.StatusCode != 403 {
		t.Fatalf("bound user delete a %s secret: got %d, want 403: %s", ns, resp.StatusCode, body)
	}
	// Cluster administration is not general administration.
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/default/pods", bearer, ""); resp.StatusCode != 403 {
		t.Fatalf("bound user list pods: got %d, want 403: %s", resp.StatusCode, body)
	}
}

func TestAuthIgnoresForgedRemoteIdentityHeaders(t *testing.T) {
	setupWranglerDev(t)

	forgedIdentityStatus := func(headerName string) int {
		t.Helper()
		req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/v1/namespaces/default/pods", testPort), nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer k8flare-dev-token")
		req.Header.Set(headerName, "rbac-test-forged-unbound")
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	for _, headerName := range []string{"X-Remote-User", "x-remote-user", "X-REMOTE-USER"} {
		if got := forgedIdentityStatus(headerName); got != 200 {
			t.Fatalf("cluster token + forged %s: got %d, want 200 (an unbound derived identity would get 403 if this header reached the Go authenticator)", headerName, got)
		}
	}
}
