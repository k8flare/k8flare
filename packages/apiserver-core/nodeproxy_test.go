package core

import "testing"

func TestNodeProxyIDSplitsPort(t *testing.T) {
	name, port, err := nodeProxyID("k8flare-agent:10250")
	if err != nil {
		t.Fatal(err)
	}
	if name != "k8flare-agent" || port != "10250" {
		t.Fatalf("name=%q port=%q", name, port)
	}
	name, port, err = nodeProxyID("k8flare-c1")
	if err != nil {
		t.Fatal(err)
	}
	if name != "k8flare-c1" || port != "" {
		t.Fatalf("name=%q port=%q", name, port)
	}
	if _, _, err := nodeProxyID(""); err == nil {
		t.Fatal("expected invalid id")
	}
}

func TestNodeProxyURL(t *testing.T) {
	u, err := nodeProxyURL("https://nodetunnel.internal", "k8flare-c1", "metrics/resource")
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/node/k8flare-c1/metrics/resource" {
		t.Fatalf("path %s", u.Path)
	}
	u, err = nodeProxyURL("https://nodetunnel.internal", "k8flare-c1", "/stats/summary")
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/node/k8flare-c1/stats/summary" {
		t.Fatalf("slash path %s", u.Path)
	}
}
