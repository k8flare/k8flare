package hookecho

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdmissionEchoDenyAndMutate(t *testing.T) {
	h := Handler(nil, "")
	req := httptest.NewRequest(http.MethodPost, "/hook/admission-echo", strings.NewReader(`{"request":{"uid":"u","name":"p","kind":{"kind":"PodAttachOptions"}}}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !strings.Contains(rr.Body.String(), "attaching to pod 'p' is not allowed") {
		t.Fatal(rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/hook/admission-echo", strings.NewReader(`{"request":{"uid":"u","object":{"metadata":{"annotations":{"k8flare.io/admit":"deny"}}}}}`))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !strings.Contains(rr.Body.String(), "denied by admission-echo") {
		t.Fatal(rr.Body.String())
	}
}

func TestConvertHostPort(t *testing.T) {
	got := convertHostPort(map[string]any{"spec": map[string]any{"hostPort": "example.com:80"}}, "demo.example.com/v2")
	spec := got["spec"].(map[string]any)
	if spec["host"] != "example.com" || spec["port"] != "80" {
		t.Fatal(spec)
	}
	back := convertHostPort(map[string]any{"spec": spec}, "demo.example.com/v1")
	if back["spec"].(map[string]any)["hostPort"] != "example.com:80" {
		t.Fatal(back)
	}
}
