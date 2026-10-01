package edgehost

import (
	"net/http"
	"testing"
)

func TestIngressHostname(t *testing.T) {
	if got := IngressHostname("default", "lb-web"); got != "lb-web--default.k8flare.com" {
		t.Fatal(got)
	}
}

func TestParseServiceHost(t *testing.T) {
	cases := []struct {
		host string
		ref  Ref
		ok   bool
	}{
		{"lb-web--default.k8flare.com", Ref{Name: "lb-web", Namespace: "default"}, true},
		{"lb-web.default.svc.k8flare.com", Ref{Name: "lb-web", Namespace: "default"}, true},
		{"lb-web--default.svc.k8flare.com", Ref{Name: "lb-web", Namespace: "default"}, true},
		{"demo.k8flare.com", Ref{}, false},
		{"api.k8flare.com", Ref{}, false},
		{"a.b.default.svc.k8flare.com", Ref{}, false},
		{"lb-web.default.k8flare.com", Ref{}, false},
	}
	for _, tc := range cases {
		got, ok := ParseServiceHost(tc.host)
		if ok != tc.ok || got != tc.ref {
			t.Fatalf("%s: got %+v %v", tc.host, got, ok)
		}
	}
}

func TestParseServiceRoute(t *testing.T) {
	hostReq, _ := http.NewRequest(http.MethodGet, "https://edge/x", nil)
	hostReq.Host = "lb-web--default.k8flare.com"
	if got, ok := ParseServiceRoute(hostReq); !ok || got != (Ref{Name: "lb-web", Namespace: "default"}) {
		t.Fatalf("host: %+v %v", got, ok)
	}
	pathReq, _ := http.NewRequest(http.MethodGet, "https://k8flare.kooffice.workers.dev/svc/default/lb-web/", nil)
	if got, ok := ParseServiceRoute(pathReq); !ok || got != (Ref{Name: "lb-web", Namespace: "default"}) {
		t.Fatalf("path: %+v %v", got, ok)
	}
	fwd, _ := http.NewRequest(http.MethodGet, "https://origin/x", nil)
	fwd.Header.Set("X-Forwarded-Host", "lb-web--kube-system.k8flare.com")
	if got, ok := ParseServiceRoute(fwd); !ok || got != (Ref{Name: "lb-web", Namespace: "kube-system"}) {
		t.Fatalf("forwarded: %+v %v", got, ok)
	}
	plain, _ := http.NewRequest(http.MethodGet, "https://origin/x", nil)
	plain.Host = "A.svc.k8flare.com"
	if got := RequestHost(plain); got != "a.svc.k8flare.com" {
		t.Fatal(got)
	}
}

func TestServicePath(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://k8flare.kooffice.workers.dev/svc/default/lb-web/health?x=1", nil)
	ref := Ref{Namespace: "default", Name: "lb-web"}
	if got := ServicePath(req, ref); got != "/health?x=1" {
		t.Fatal(got)
	}
	root, _ := http.NewRequest(http.MethodGet, "https://k8flare.kooffice.workers.dev/svc/default/lb-web", nil)
	if got := ServicePath(root, ref); got != "/" {
		t.Fatal(got)
	}
}

func useClusterDomain(t *testing.T, domain string) {
	t.Helper()
	SetClusterDomain(domain)
	t.Cleanup(func() { SetClusterDomain("") })
}

func TestConfiguredClusterDomain(t *testing.T) {
	useClusterDomain(t, "example.test")
	if got := IngressHostname("default", "web"); got != "web--default.example.test" {
		t.Fatalf("IngressHostname: got %q, want %q", got, "web--default.example.test")
	}
	ref, ok := ParseServiceHost("web--default.example.test")
	if !ok || ref != (Ref{Name: "web", Namespace: "default"}) {
		t.Fatalf("ParseServiceHost example.test: got %+v, %v", ref, ok)
	}
	if _, ok := ParseServiceHost("web--default.k8flare.com"); ok {
		t.Fatal("web--default.k8flare.com resolved when cluster domain is example.test")
	}
	if ref, ok := ParseServiceHost("web.default.svc.example.test"); !ok || ref != (Ref{Name: "web", Namespace: "default"}) {
		t.Fatalf("ParseServiceHost legacy: got %+v, %v", ref, ok)
	}
	if ref, ok := ParseServiceHost("web--default.svc.example.test"); !ok || ref != (Ref{Name: "web", Namespace: "default"}) {
		t.Fatalf("ParseServiceHost legacy dash: got %+v, %v", ref, ok)
	}
}

func TestDefaultClusterDomainUnchanged(t *testing.T) {
	if got := IngressHostname("default", "lb-web"); got != "lb-web--default.k8flare.com" {
		t.Fatalf("IngressHostname: got %q", got)
	}
	ref, ok := ParseServiceHost("lb-web--default.k8flare.com")
	if !ok || ref != (Ref{Name: "lb-web", Namespace: "default"}) {
		t.Fatalf("ParseServiceHost default: got %+v, %v", ref, ok)
	}
}
