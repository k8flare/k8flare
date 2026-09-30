package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEdgeCertificateWritesTheFilesToUpload(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["POST /internal/edge-certificate"] = `{"cert":"CERT","key":"KEY","serverCA":"SERVERCA","clientCA":"CLIENTCA"}`
	dir := t.TempDir()
	out, err := f.run(t, "edge-certificate", "--hosts", "api.example.com,127.0.0.1", "--ttl", "48h", "--out-dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"edge.crt": "CERT", "edge.key": "KEY", "client-ca.crt": "CLIENTCA"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q %v", name, got, err)
		}
		if !strings.Contains(out, filepath.Join(dir, name)) {
			t.Fatalf("output does not name %s: %q", name, out)
		}
	}
	if info, _ := os.Stat(filepath.Join(dir, "edge.key")); info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode %v", info.Mode().Perm())
	}
	var body struct {
		Hosts      []string `json:"hosts"`
		TTLSeconds int64    `json:"ttlSeconds"`
	}
	if err := json.Unmarshal([]byte(f.last().body), &body); err != nil || len(body.Hosts) != 2 || body.Hosts[0] != "api.example.com" || body.TTLSeconds != 172800 {
		t.Fatalf("body %s", f.last().body)
	}
}

func TestEdgeCertificateNeedsHosts(t *testing.T) {
	f := newFakeCluster(t)
	if _, err := f.run(t, "edge-certificate", "--out-dir", t.TempDir()); err == nil {
		t.Fatal("no hosts accepted")
	}
}
