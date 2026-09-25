package admission

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	authorizationv1 "k8s.io/api/authorization/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCertificateApprovalDeniesUnauthorized(t *testing.T) {
	h := newCertAdmitHandler(t, false)
	out := postAdmit(t, h, csrApprovalReq("view", false))
	if out.Allowed {
		t.Fatal("expected deny")
	}
	if !strings.Contains(out.Message, `user not permitted to approve requests with signerName "kubernetes.io/kube-apiserver-client"`) {
		t.Fatalf("message = %q", out.Message)
	}
}

func TestCertificateApprovalAllowsAuthorized(t *testing.T) {
	h := newCertAdmitHandler(t, true)
	out := postAdmit(t, h, csrApprovalReq("admin", false))
	if !out.Allowed {
		t.Fatalf("expected allow: %+v", out)
	}
}

func TestCertificateSigningDeniesUnauthorized(t *testing.T) {
	h := newCertAdmitHandler(t, false)
	out := postAdmit(t, h, csrApprovalReq("view", true))
	if out.Allowed {
		t.Fatal("expected deny")
	}
	if !strings.Contains(out.Message, `user not permitted to sign requests with signerName "kubernetes.io/kube-apiserver-client"`) {
		t.Fatalf("message = %q", out.Message)
	}
}

func newCertAdmitHandler(t *testing.T, allow bool) http.Handler {
	t.Helper()
	sar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var review authorizationv1.SubjectAccessReview
		_ = json.NewDecoder(r.Body).Decode(&review)
		review.Status.Allowed = allow
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(review)
	}))
	t.Cleanup(sar.Close)
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	t.Cleanup(kineSrv.Close)
	return NewHandler(Config{Kine: rewriteClient(kineSrv), API: rewriteClient(sar)})
}

func csrApprovalReq(user string, signing bool) admit.Request {
	sub := "approval"
	if signing {
		sub = "status"
	}
	obj := map[string]any{
		"apiVersion": "certificates.k8s.io/v1",
		"kind":       "CertificateSigningRequest",
		"metadata":   map[string]any{"name": "csr"},
		"spec":       map[string]any{"signerName": certificatesv1.KubeAPIServerClientSignerName},
		"status":     map[string]any{},
	}
	old := map[string]any{
		"apiVersion": "certificates.k8s.io/v1",
		"kind":       "CertificateSigningRequest",
		"metadata":   map[string]any{"name": "csr"},
		"spec":       map[string]any{"signerName": certificatesv1.KubeAPIServerClientSignerName},
		"status":     map[string]any{},
	}
	if signing {
		obj["status"] = map[string]any{"certificate": "Y2VydA=="}
	} else {
		obj["status"] = map[string]any{"conditions": []any{map[string]any{"type": "Approved", "status": "True"}}}
	}
	return admit.Request{
		Phase:       "validate",
		Name:        "csr",
		Resource:    schema.GroupVersionResource{Group: "certificates.k8s.io", Version: "v1", Resource: "certificatesigningrequests"},
		Subresource: sub,
		Kind:        schema.GroupVersionKind{Group: "certificates.k8s.io", Version: "v1", Kind: "CertificateSigningRequest"},
		Operation:   "UPDATE",
		Object:      obj,
		OldObject:   old,
		User:        admit.User{Username: user, Groups: []string{"system:authenticated"}},
	}
}
