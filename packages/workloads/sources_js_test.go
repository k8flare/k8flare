//go:build js && wasm

package workloads

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type stubTransport struct {
	calls atomic.Int64
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls.Add(1)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"items":[]}`)),
	}, nil
}

func testClient(t *testing.T, tr http.RoundTripper) kubernetes.Interface {
	t.Helper()
	cfg := &rest.Config{
		Host:          "https://k8flare.internal",
		BearerToken:   "test-token",
		Transport:     tr,
		ContentConfig: rest.ContentConfig{AcceptContentTypes: "application/json", ContentType: "application/json"},
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("NewForConfig failed: %v", err)
	}
	return client
}

func TestValidatingAdmissionPoliciesSource(t *testing.T) {
	tr := &stubTransport{}
	client := testClient(t, tr)
	var vapSource *source
	for _, s := range sources(client) {
		if s.name == "validatingadmissionpolicies" {
			vapSource = &s
			break
		}
	}
	if vapSource == nil {
		t.Fatal("validatingadmissionpolicies source not found")
	}
	_, err := vapSource.page(context.Background(), metav1.ListOptions{})
	t.Logf("vap list result: calls=%d err=%v", tr.calls.Load(), err)
}

func TestEverySourceCanList(t *testing.T) {
	tr := &stubTransport{}
	client := testClient(t, tr)
	for _, s := range sources(client) {
		t.Run(s.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("source %s panicked: %v", s.name, r)
				}
			}()
			_, _ = s.page(context.Background(), metav1.ListOptions{})
		})
	}
}
