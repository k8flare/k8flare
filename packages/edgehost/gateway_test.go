package edgehost

import "testing"

func TestIsGatewayHost(t *testing.T) {
	if !IsGatewayHost("app.example.com") {
		t.Fatal("app")
	}
	for _, host := range []string{"", "k8flare.kooffice.workers.dev", "cluster.internal", "api.k8flare.com", "k8flare.com", "lb-web--default.k8flare.com"} {
		if IsGatewayHost(host) {
			t.Fatal(host)
		}
	}
}

func TestMatchPath(t *testing.T) {
	if !MatchPath("/health", &pathMatch{Type: "Exact", Value: "/health"}) {
		t.Fatal("exact")
	}
	if MatchPath("/healthz", &pathMatch{Type: "Exact", Value: "/health"}) {
		t.Fatal("exact miss")
	}
	if !MatchPath("/api/v1", &pathMatch{Type: "PathPrefix", Value: "/api"}) {
		t.Fatal("prefix")
	}
	if !MatchPath("/x", &pathMatch{Type: "RegularExpression", Value: "^/x$"}) {
		t.Fatal("re")
	}
	if MatchPath("/x", &pathMatch{Type: "RegularExpression", Value: "("}) {
		t.Fatal("bad re")
	}
	if !MatchPath("/", nil) {
		t.Fatal("default")
	}
}

func TestMatchHostname(t *testing.T) {
	if !MatchHostname("app.example.com", nil) {
		t.Fatal("empty")
	}
	if !MatchHostname("app.example.com", []string{"*.example.com"}) {
		t.Fatal("wild")
	}
	if MatchHostname("a.b.example.com", []string{"*.example.com"}) {
		t.Fatal("wild depth")
	}
	if !MatchHostname("app.example.com:443", []string{"app.example.com"}) {
		t.Fatal("port")
	}
}
