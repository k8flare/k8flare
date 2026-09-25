package admit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestRemoteAdmitMutateAndDeny(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decode: %v", err)
		}
		if req.Phase == "admit" {
			req.Object["metadata"].(map[string]any)["annotations"] = map[string]any{"mutated": "true"}
			_ = json.NewEncoder(w).Encode(Response{Allowed: true, Object: req.Object})
			return
		}
		_ = json.NewEncoder(w).Encode(Response{Allowed: false, Message: "nope"})
	}))
	defer srv.Close()
	client := srv.Client()
	client.Transport = rewrite{next: client.Transport, host: srv.URL}
	plugin := New(client)
	mut, ok := plugin.(admission.MutationInterface)
	if !ok {
		t.Fatal("expected MutationInterface")
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"}}
	attrs := admission.NewAttributesRecord(pod, nil, schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, "default", "p", schema.GroupVersionResource{Version: "v1", Resource: "pods"}, "", admission.Create, nil, false, &user.DefaultInfo{Name: "admin"})
	if err := mut.Admit(context.Background(), attrs, nil); err == nil {
		t.Fatal("expected validate deny during Admit")
	}
	if pod.Annotations["mutated"] != "true" {
		t.Fatalf("mutation not applied: %+v", pod.Annotations)
	}
	v, ok := plugin.(admission.ValidationInterface)
	if !ok {
		t.Fatal("expected ValidationInterface")
	}
	if err := v.Validate(context.Background(), attrs, nil); err == nil {
		t.Fatal("expected deny")
	}
}

type rewrite struct {
	next http.RoundTripper
	host string
}

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	next := req.Clone(req.Context())
	u, _ := http.NewRequest(req.Method, r.host+req.URL.Path, req.Body)
	next.URL = u.URL
	next.Host = u.Host
	if r.next == nil {
		return http.DefaultTransport.RoundTrip(next)
	}
	return r.next.RoundTrip(next)
}
