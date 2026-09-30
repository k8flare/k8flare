package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"time"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testCAPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "server-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

type seen struct {
	method, path, query, auth, body string
}

type fakeCluster struct {
	server  *httptest.Server
	calls   []seen
	status  int
	respond map[string]string
}

func newFakeCluster(t *testing.T) *fakeCluster {
	t.Helper()
	f := &fakeCluster{status: http.StatusOK, respond: map[string]string{"GET /cacerts": testCAPEM(t)}}
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.calls = append(f.calls, seen{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), string(body)})
		w.WriteHeader(f.status)
		_, _ = io.WriteString(w, f.respond[r.Method+" "+r.URL.Path])
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeCluster) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := run(append(args, "--server", f.server.URL, "--token", "admin-secret", "--insecure-skip-tls-verify"), &out)
	return out.String(), err
}

func (f *fakeCluster) last() seen { return f.calls[len(f.calls)-1] }

func k10Of(t *testing.T, caPEM, creds string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(caPEM))
	return "K10" + hex.EncodeToString(sum[:]) + "::" + creds
}

func TestTokenCreateSendsTTLAndDescriptionAndPrintsOnlyTheK10Token(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["POST /internal/tokens"] = `{"id":"abc123","token":"abc123.0123456789abcdef"}`
	out, err := f.run(t, "token", "create", "--ttl", "1h", "--description", "ci")
	if err != nil {
		t.Fatal(err)
	}
	want := k10Of(t, f.respond["GET /cacerts"], "abc123.0123456789abcdef") + "\n"
	if out != want {
		t.Fatalf("output %q, want %q", out, want)
	}
	c := f.last()
	if c.method != "POST" || c.path != "/internal/tokens" || c.auth != "Bearer admin-secret" {
		t.Fatalf("%+v", c)
	}
	var body struct {
		Description string `json:"description"`
		TTLSeconds  int64  `json:"ttlSeconds"`
	}
	if err := json.Unmarshal([]byte(c.body), &body); err != nil || body.Description != "ci" || body.TTLSeconds != 3600 {
		t.Fatalf("body %s", c.body)
	}
}

func TestTokenCreateTTLDefaultsToADayAndZeroMeansNever(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["POST /internal/tokens"] = `{"id":"abc123","token":"abc123.x"}`
	if _, err := f.run(t, "token", "create"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.last().body, `"ttlSeconds":86400`) {
		t.Fatalf("default: %s", f.last().body)
	}
	if _, err := f.run(t, "token", "create", "--ttl", "0"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.last().body, `"ttlSeconds":0`) {
		t.Fatalf("zero: %s", f.last().body)
	}
	if _, err := f.run(t, "token", "create", "--ttl", "-5m"); err == nil {
		t.Fatal("negative ttl accepted")
	}
	if _, err := f.run(t, "token", "create", "--ttl", "soon"); err == nil {
		t.Fatal("unparsable ttl accepted")
	}
}

func TestTokenListPrintsATable(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["GET /internal/tokens"] = `{"items":[{"id":"abc123","description":"ci","createdAt":"2026-01-01T00:00:00Z","expiresAt":"2999-01-01T00:00:00Z"},{"id":"def456","createdAt":"2026-01-01T00:00:00Z"}]}`
	out, err := f.run(t, "token", "list")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "ID") {
		t.Fatalf("output:\n%s", out)
	}
	if !strings.Contains(lines[1], "abc123") || !strings.Contains(lines[1], "ci") || !strings.Contains(lines[1], "2999-01-01T00:00:00Z") {
		t.Fatalf("row 1: %s", lines[1])
	}
	if !strings.Contains(lines[2], "<never>") || !strings.Contains(lines[2], "<none>") {
		t.Fatalf("row 2: %s", lines[2])
	}
}

func TestTokenDeleteAcceptsIDsAndFullTokens(t *testing.T) {
	f := newFakeCluster(t)
	if _, err := f.run(t, "token", "delete", "abc123", "def456.0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || f.calls[0].method != "DELETE" || f.calls[0].path != "/internal/tokens/abc123" || f.calls[1].path != "/internal/tokens/def456" {
		t.Fatalf("%+v", f.calls)
	}
	if _, err := f.run(t, "token", "delete"); err == nil {
		t.Fatal("delete without an id")
	}
}

func TestTokenRotatePrintsTheNewToken(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["POST /internal/tokens/abc123/rotate"] = `{"id":"abc123","token":"abc123.newnewnewnewnew0"}`
	out, err := f.run(t, "token", "rotate", "abc123")
	if err != nil || out != k10Of(t, f.respond["GET /cacerts"], "abc123.newnewnewnewnew0")+"\n" {
		t.Fatalf("%q %v", out, err)
	}
}

func TestSecretsEncryptStatusAndReencrypt(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["GET /internal/secrets-encrypt/status"] = `{"enabled":true,"activeKey":"aesgcm k2","inactiveKeys":["aesgcm k1"],"total":10,"current":7,"stale":3}`
	f.respond["POST /internal/secrets-encrypt/reencrypt"] = `{"rewritten":3}`
	out, err := f.run(t, "secrets-encrypt", "status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Encryption Status: Enabled", "Active Key: aesgcm k2", "Inactive Keys: aesgcm k1", "Secrets: 10 total, 7 current, 3 stale"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	out, err = f.run(t, "secrets-encrypt", "reencrypt")
	if err != nil || !strings.Contains(out, "3") {
		t.Fatalf("%q %v", out, err)
	}
	if f.last().method != "POST" {
		t.Fatalf("%+v", f.last())
	}
}

func TestSecretsEncryptStatusDisabled(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["GET /internal/secrets-encrypt/status"] = `{"enabled":false,"total":0,"current":0,"stale":0}`
	out, err := f.run(t, "secrets-encrypt", "status")
	if err != nil || !strings.Contains(out, "Encryption Status: Disabled") {
		t.Fatalf("%q %v", out, err)
	}
}

func TestSnapshotSaveAndList(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["POST /internal/snapshots"] = `{"ok":true,"key":"clusters/default/snapshots/2026-01-01T00-00-00-000Z.json","revision":12,"count":4,"bytes":900}`
	f.respond["GET /internal/snapshots"] = `{"items":[{"key":"clusters/default/snapshots/a.json","size":900,"uploaded":"2026-01-01T00:00:00.000Z"}]}`
	out, err := f.run(t, "snapshot", "save")
	if err != nil || strings.TrimSpace(out) != "clusters/default/snapshots/2026-01-01T00-00-00-000Z.json" {
		t.Fatalf("%q %v", out, err)
	}
	out, err = f.run(t, "snapshot", "list")
	if err != nil || !strings.Contains(out, "KEY") || !strings.Contains(out, "clusters/default/snapshots/a.json") || !strings.Contains(out, "900") {
		t.Fatalf("%q %v", out, err)
	}
}

func TestSnapshotRestoreSendsKeyAndForce(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["POST /internal/snapshots/restore"] = `{"ok":true,"key":"clusters/default/snapshots/a.json","written":5,"removed":2,"revision":30}`
	out, err := f.run(t, "snapshot", "restore", "--force", "clusters/default/snapshots/a.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "5") || !strings.Contains(out, "2") {
		t.Fatalf("output %q", out)
	}
	var body struct {
		Key   string `json:"key"`
		Force bool   `json:"force"`
	}
	if err := json.Unmarshal([]byte(f.last().body), &body); err != nil || body.Key != "clusters/default/snapshots/a.json" || !body.Force {
		t.Fatalf("body %s", f.last().body)
	}
	if _, err := f.run(t, "snapshot", "restore"); err == nil {
		t.Fatal("restore without a key")
	}
}

func TestSnapshotRestoreWithoutForceExplainsARefusal(t *testing.T) {
	f := newFakeCluster(t)
	f.status = http.StatusConflict
	f.respond["POST /internal/snapshots/restore"] = `{"error":"cluster is not empty; restoring replaces its contents, pass force to proceed","keys":41}`
	_, err := f.run(t, "snapshot", "restore", "clusters/default/snapshots/a.json")
	if err == nil || !strings.Contains(err.Error(), "not empty") || !strings.Contains(err.Error(), "409") {
		t.Fatalf("error %v", err)
	}
	if strings.Contains(f.last().body, `"force":true`) {
		t.Fatalf("force sent by default: %s", f.last().body)
	}
}

func TestRestoreToATime(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["POST /internal/restore"] = `{"ok":true,"bookmark":"b1","to":"2026-01-01T00:00:00.000Z"}`
	out, err := f.run(t, "restore", "--to", "2026-01-01T00:00:00Z")
	if err != nil || !strings.Contains(out, "2026-01-01T00:00:00.000Z") {
		t.Fatalf("%q %v", out, err)
	}
	if f.last().method != "POST" || f.last().path != "/internal/restore" || f.last().query != "to=2026-01-01T00%3A00%3A00Z" {
		t.Fatalf("%+v", f.last())
	}
	if _, err := f.run(t, "restore"); err == nil {
		t.Fatal("restore without --to")
	}
}

func TestServerErrorsSurfaceStatusAndBody(t *testing.T) {
	f := newFakeCluster(t)
	f.status = http.StatusForbidden
	f.respond["GET /internal/tokens"] = "forbidden"
	_, err := f.run(t, "token", "list")
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("error %v", err)
	}
}

func TestConnectionFromKubeconfig(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["GET /internal/tokens"] = `{"items":[]}`
	path := filepath.Join(t.TempDir(), "kubeconfig")
	config := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: %s
    insecure-skip-tls-verify: true
users:
- name: u
  user:
    token: from-kubeconfig
contexts:
- name: x
  context:
    cluster: c
    user: u
current-context: x
`, f.server.URL)
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"token", "list", "--kubeconfig", path}, &out); err != nil {
		t.Fatal(err)
	}
	if f.last().auth != "Bearer from-kubeconfig" {
		t.Fatalf("auth %q", f.last().auth)
	}
}

func TestMissingConnectionIsAnError(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	if err := run([]string{"token", "list"}, &out); err == nil {
		t.Fatal("expected an error without a cluster")
	}
}

func TestUnknownCommands(t *testing.T) {
	var out bytes.Buffer
	for _, args := range [][]string{{}, {"nope"}, {"token"}, {"token", "nope"}, {"snapshot", "nope"}, {"secrets-encrypt", "nope"}} {
		if err := run(args, &out); err == nil {
			t.Fatalf("%v accepted", args)
		}
	}
}
