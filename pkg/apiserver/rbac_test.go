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

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// rbacDo sends one request as a derived identity: valid cluster token +
// X-Remote-User (the TLS-proxy identity path in auth.go). groups may be
// empty (auth.go then assigns just system:authenticated).
func rbacDo(t *testing.T, method, path, remoteUser, remoteGroups string, body string) (*http.Response, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", testPort, path), rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer k8flare-dev-token")
	if remoteUser != "" {
		req.Header.Set("X-Remote-User", remoteUser)
	}
	if remoteGroups != "" {
		req.Header.Set("X-Remote-Group", remoteGroups)
	}
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

// TestRBACEnforcement drives the real RBACAuthorizer end-to-end: a
// derived identity is denied by default, gains exactly what a live
// Role/RoleBinding grants, and `kubectl auth can-i` (SSAR) agrees with
// enforcement. The cluster token (system:masters) stays all-powerful.
func TestRBACEnforcement(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	const user = "rbac-test-alice"

	// The admin identity (bare cluster token) bypasses RBAC.
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/default/pods", "", "", ""); resp.StatusCode != 200 {
		t.Fatalf("admin list pods: got %d, want 200: %s", resp.StatusCode, body)
	}

	// A derived identity with no bindings is denied...
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/default/pods", user, "", ""); resp.StatusCode != 403 {
		t.Fatalf("unbound user list pods: got %d, want 403: %s", resp.StatusCode, body)
	}
	// ...but can hit discovery (system:discovery -> system:authenticated,
	// straight from the real bootstrap policy).
	if resp, body := rbacDo(t, "GET", "/api/v1", user, "", ""); resp.StatusCode != 200 {
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
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: user}},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: role.Name},
	}
	if _, err := client.RbacV1().RoleBindings("default").Create(ctx, binding, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create rolebinding: %v", err)
	}
	t.Cleanup(func() {
		_ = client.RbacV1().RoleBindings("default").Delete(ctx, binding.Name, metav1.DeleteOptions{})
	})

	// Granted verbs work; everything else stays denied.
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/default/pods", user, "", ""); resp.StatusCode != 200 {
		t.Fatalf("bound user list pods: got %d, want 200: %s", resp.StatusCode, body)
	}
	pod := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"rbac-test-denied"},"spec":{"containers":[{"name":"c","image":"busybox"}]}}`
	if resp, body := rbacDo(t, "POST", "/api/v1/namespaces/default/pods", user, "", pod); resp.StatusCode != 403 {
		t.Fatalf("bound user create pod: got %d, want 403: %s", resp.StatusCode, body)
	}
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/kube-system/pods", user, "", ""); resp.StatusCode != 403 {
		t.Fatalf("bound user list pods in kube-system: got %d, want 403: %s", resp.StatusCode, body)
	}
	if resp, body := rbacDo(t, "GET", "/api/v1/namespaces/default/secrets", user, "", ""); resp.StatusCode != 403 {
		t.Fatalf("bound user list secrets: got %d, want 403: %s", resp.StatusCode, body)
	}

	// `kubectl auth can-i` (SSAR) answers from the same authorizer.
	ssar := func(verb, resource string) bool {
		body := fmt.Sprintf(`{"apiVersion":"authorization.k8s.io/v1","kind":"SelfSubjectAccessReview","spec":{"resourceAttributes":{"verb":%q,"resource":%q,"namespace":"default"}}}`, verb, resource)
		resp, out := rbacDo(t, "POST", "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews", user, "", body)
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

	// The TS watch path enforces the same policy via SubjectAccessReview
	// (gateway/index.ts authorizeWatchRBAC): allowed resource streams,
	// denied resource 403s before a stream opens.
	watchStatus := func(path string) int {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d%s", testPort, path), nil)
		if err != nil {
			t.Fatalf("new watch request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer k8flare-dev-token")
		req.Header.Set("X-Remote-User", user)
		wctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := (&http.Client{}).Do(req.WithContext(wctx))
		if err != nil {
			t.Fatalf("watch %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if got := watchStatus("/api/v1/namespaces/default/pods?watch=true&timeoutSeconds=1"); got != 200 {
		t.Fatalf("watch pods as bound user: got %d, want 200", got)
	}
	if got := watchStatus("/api/v1/namespaces/default/secrets?watch=true&timeoutSeconds=1"); got != 403 {
		t.Fatalf("watch secrets as bound user: got %d, want 403", got)
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
