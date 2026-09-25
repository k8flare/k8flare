package core

import (
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestFirstContainerPort(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
		}}},
	}
	if p := firstContainerPort(pod); p != "8080" {
		t.Fatalf("port %s", p)
	}
	if firstContainerPort(&corev1.Pod{}) != "" {
		t.Fatal("expected empty")
	}
}

func TestProxyUserURLEmptyPath(t *testing.T) {
	u, err := proxyUserURL("https://nodetunnel.internal", "")
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "" {
		t.Fatalf("path %q", u.Path)
	}
	u, err = proxyUserURL("https://nodetunnel.internal", "healthz")
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/healthz" {
		t.Fatalf("path %s", u.Path)
	}
}

func TestTunnelDialTransport(t *testing.T) {
	var got *http.Request
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		got = req
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})
	tr := tunnelDialTransport{base: rt, root: "https://nodetunnel.internal", node: "n1", host: "10.42.0.9", port: "8080", secure: true}
	req, _ := http.NewRequest(http.MethodGet, "https://nodetunnel.internal/healthz", nil)
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if got.URL.Path != "/dial/n1/10.42.0.9/8080/healthz" {
		t.Fatalf("path %s", got.URL.Path)
	}
	if got.Header.Get("X-Dial-TLS") != "1" {
		t.Fatal("expected tls header")
	}
}

func TestIsBareProxyPath(t *testing.T) {
	if !isBareProxyPath("/api/v1/namespaces/ns/pods/p/proxy") {
		t.Fatal("bare")
	}
	if isBareProxyPath("/api/v1/namespaces/ns/pods/p/proxy/") {
		t.Fatal("slash")
	}
	if isBareProxyPath("/api/v1/namespaces/ns/pods/p/proxy/foo") {
		t.Fatal("subpath")
	}
}

func TestShouldRedirectProxyRoot(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://apigroups.internal/api/v1/namespaces/ns/pods/p/proxy", nil)
	req.URL.Path = "/api/v1/namespaces/ns/pods/p/proxy/"
	req.RequestURI = "/api/v1/namespaces/ns/pods/p/proxy"
	if !shouldRedirectProxyRoot("", req) {
		t.Fatal("expected redirect from original RequestURI")
	}
	req.RequestURI = "/api/v1/namespaces/ns/pods/p/proxy/"
	if shouldRedirectProxyRoot("", req) {
		t.Fatal("no redirect when original has slash")
	}
	if shouldRedirectProxyRoot("healthz", req) {
		t.Fatal("no redirect with user path")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
