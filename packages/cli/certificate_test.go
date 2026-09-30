package main

import (
	"strings"
	"testing"
	"time"
)

func TestCertificateRotateCACallsTheServerAndPrintsTheBundles(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["POST /internal/certificate/rotate-ca"] = `{"items":[{"name":"server-ca","subject":"k8flare-server-ca@2","notBefore":"2026-01-01T00:00:00Z","notAfter":"2999-01-01T00:00:00Z","current":true},{"name":"server-ca","subject":"k8flare-server-ca@1","notBefore":"2025-01-01T00:00:00Z","notAfter":"2998-01-01T00:00:00Z"}]}`
	out, err := f.run(t, "certificate", "rotate-ca")
	if err != nil {
		t.Fatal(err)
	}
	c := f.last()
	if c.method != "POST" || c.path != "/internal/certificate/rotate-ca" || c.auth != "Bearer admin-secret" {
		t.Fatalf("%+v", c)
	}
	if !strings.Contains(out, "k8flare-server-ca@2") || !strings.Contains(out, "k8flare-server-ca@1") {
		t.Fatalf("output does not list both generations: %q", out)
	}
}

func TestCertificateCheckReportsExpiry(t *testing.T) {
	soon := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	f := newFakeCluster(t)
	f.respond["GET /internal/certificate/check"] = `{"items":[` +
		`{"name":"server-ca","subject":"fresh","notAfter":"2999-01-01T00:00:00Z","current":true},` +
		`{"name":"client-ca","subject":"soon","notAfter":"` + soon + `","current":true},` +
		`{"name":"client-ca","subject":"gone","notAfter":"2001-01-01T00:00:00Z","expired":true}]}`
	out, err := f.run(t, "certificate", "check")
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n")[1:] {
		fields := strings.Fields(line)
		status[fields[1]] = fields[len(fields)-1]
	}
	if status["fresh"] != "ok" || status["soon"] != "expiring" || status["gone"] != "expired" {
		t.Fatalf("%v\n%s", status, out)
	}
}

func TestCertificateNeedsASubcommand(t *testing.T) {
	f := newFakeCluster(t)
	for _, args := range [][]string{{"certificate"}, {"certificate", "rotate"}} {
		if _, err := f.run(t, args...); err == nil {
			t.Fatalf("%v accepted", args)
		}
	}
}
