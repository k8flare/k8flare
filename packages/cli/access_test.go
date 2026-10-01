package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
)

func accessJWT(t *testing.T, expires time.Time) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(expires)}).SignedString([]byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func withoutAccessEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv(accessTokenEnv, "")
	t.Setenv(execInfoEnv, "")
	t.Setenv("PATH", t.TempDir())
}

func fakeCloudflared(t *testing.T, stdout string) (argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\necho " + stdout + "\n"
	if err := os.WriteFile(filepath.Join(dir, "cloudflared"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return argsFile
}

func decodeExecCredential(t *testing.T, out string) clientauthenticationv1.ExecCredential {
	t.Helper()
	var cred clientauthenticationv1.ExecCredential
	if err := json.Unmarshal([]byte(out), &cred); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if cred.APIVersion != "client.authentication.k8s.io/v1" || cred.Kind != "ExecCredential" || cred.Status == nil {
		t.Fatalf("not a v1 ExecCredential with a status: %s", out)
	}
	return cred
}

func TestAccessCredentialPrintsAnExecCredentialExpiringWithTheToken(t *testing.T) {
	withoutAccessEnvironment(t)
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	token := accessJWT(t, expires)
	t.Setenv(accessTokenEnv, token)
	var out bytes.Buffer
	if err := run([]string{"access-credential", "--server", "https://cluster.example.com"}, &out); err != nil {
		t.Fatal(err)
	}
	cred := decodeExecCredential(t, out.String())
	if cred.Status.Token != token {
		t.Fatalf("token %q", cred.Status.Token)
	}
	if cred.Status.ExpirationTimestamp == nil || !cred.Status.ExpirationTimestamp.Time.Equal(expires) {
		t.Fatalf("expirationTimestamp %v, want %v", cred.Status.ExpirationTimestamp, expires)
	}
}

func TestAccessCredentialAsksCloudflaredForTheApplicationToken(t *testing.T) {
	withoutAccessEnvironment(t)
	token := accessJWT(t, time.Now().Add(time.Hour))
	argsFile := fakeCloudflared(t, token)
	var out bytes.Buffer
	if err := run([]string{"access-credential", "--server", "https://cluster.example.com"}, &out); err != nil {
		t.Fatal(err)
	}
	if cred := decodeExecCredential(t, out.String()); cred.Status.Token != token {
		t.Fatalf("token %q", cred.Status.Token)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(args)); got != "access token -app=https://cluster.example.com" {
		t.Fatalf("cloudflared %s", got)
	}
}

func TestAccessCredentialTakesTheServerFromExecInfo(t *testing.T) {
	withoutAccessEnvironment(t)
	argsFile := fakeCloudflared(t, accessJWT(t, time.Now().Add(time.Hour)))
	t.Setenv(execInfoEnv, `{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","spec":{"cluster":{"server":"https://cluster.example.com"},"interactive":false}}`)
	var out bytes.Buffer
	if err := run([]string{"access-credential"}, &out); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(args)); got != "access token -app=https://cluster.example.com" {
		t.Fatalf("cloudflared %s", got)
	}
}

func TestAccessCredentialWithoutATokenIsAnError(t *testing.T) {
	withoutAccessEnvironment(t)
	var out bytes.Buffer
	err := run([]string{"access-credential", "--server", "https://cluster.example.com"}, &out)
	if err == nil || !strings.Contains(err.Error(), "cloudflared access login https://cluster.example.com") || !strings.Contains(err.Error(), accessTokenEnv) {
		t.Fatalf("error %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("printed %s", out.String())
	}
}

func TestAccessCredentialRejectsWhatIsNotAJWTOrHasNoServer(t *testing.T) {
	withoutAccessEnvironment(t)
	var out bytes.Buffer
	if err := run([]string{"access-credential"}, &out); err == nil || !strings.Contains(err.Error(), "--server") {
		t.Fatalf("without a server: %v", err)
	}
	t.Setenv(accessTokenEnv, "not-a-jwt")
	if err := run([]string{"access-credential", "--server", "https://cluster.example.com"}, &out); err == nil || !strings.Contains(err.Error(), "not a JWT") {
		t.Fatalf("with a malformed token: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("printed %s", out.String())
	}
}
