package registry

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestNamespaceLifecycleKeepsImmortalNamespaces(t *testing.T) {
	h := NamespaceLifecycle(&kine.Client{HTTP: http.DefaultClient})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, name := range []string{"default", "kube-system", "kube-public"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/namespaces/"+name, nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d body=%s", name, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "this namespace may not be deleted") {
			t.Fatalf("%s body=%s", name, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/namespaces/app", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("app status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNamespaceLifecycleAllowsAccessReviewInTerminatingNamespace(t *testing.T) {
	ns, err := runtime.Encode(scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "gone"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(lifecycleStore{data: map[string][]byte{"/registry/namespaces/gone": ns}})
	defer srv.Close()
	passed := false
	h := NamespaceLifecycle(&kine.Client{HTTP: rewriteHost(srv)})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		passed = true
		w.WriteHeader(http.StatusCreated)
	}))
	review := httptest.NewRecorder()
	h.ServeHTTP(review, httptest.NewRequest(http.MethodPost, "/apis/authorization.k8s.io/v1/namespaces/gone/localsubjectaccessreviews", strings.NewReader("{}")))
	if review.Code != http.StatusCreated || !passed {
		t.Fatalf("review status=%d passed=%v body=%s", review.Code, passed, review.Body.String())
	}
	passed = false
	pod := httptest.NewRecorder()
	h.ServeHTTP(pod, httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/gone/pods", strings.NewReader("{}")))
	if pod.Code != http.StatusForbidden || passed {
		t.Fatalf("pod status=%d passed=%v body=%s", pod.Code, passed, pod.Body.String())
	}
	if !strings.Contains(pod.Body.String(), "NamespaceTerminating") {
		t.Fatalf("pod body=%s", pod.Body.String())
	}
}

type lifecycleStore struct {
	data map[string][]byte
}

func (m lifecycleStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/kv" {
		http.NotFound(w, r)
		return
	}
	k := r.URL.Query().Get("key")
	if v, ok := m.data[k]; ok {
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kv": map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 1}})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
}

func rewriteHost(srv *httptest.Server) *http.Client {
	return &http.Client{Transport: hostRewrite{base: srv.URL}}
}

type hostRewrite struct{ base string }

func (h hostRewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	next, err := http.NewRequest(req.Method, h.base+req.URL.RequestURI(), req.Body)
	if err != nil {
		return nil, err
	}
	next.Header = req.Header
	return http.DefaultTransport.RoundTrip(next)
}
