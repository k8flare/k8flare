package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
)

const (
	accessTokenEnv = "K8FLARE_ACCESS_TOKEN"
	execInfoEnv    = "KUBERNETES_EXEC_INFO"
)

const accessCredentialUsage = `usage: k8flare access-credential [--server <url>]

prints a client.authentication.k8s.io/v1 ExecCredential holding the Cloudflare
Access application token, for a kubeconfig exec user. --server defaults to the
cluster kubectl passes with provideClusterInfo. the token is read from
$K8FLARE_ACCESS_TOKEN, or else from "cloudflared access token -app=<url>",
which needs cloudflared on PATH and a session from "cloudflared access login <url>".`

func runAccessCredential(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("access-credential", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	server := fs.String("server", execInfoServer(), "URL of the Access application in front of the cluster")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, accessCredentialUsage)
	}
	if *server == "" || fs.NArg() > 0 {
		return errors.New(accessCredentialUsage)
	}
	token, err := accessToken(*server)
	if err != nil {
		return err
	}
	var claims jwt.RegisteredClaims
	if _, _, err := jwt.NewParser().ParseUnverified(token, &claims); err != nil {
		return fmt.Errorf("the Access token for %s is not a JWT: %w", *server, err)
	}
	status := &clientauthenticationv1.ExecCredentialStatus{Token: token}
	if claims.ExpiresAt != nil {
		status.ExpirationTimestamp = &metav1.Time{Time: claims.ExpiresAt.Time}
	}
	return json.NewEncoder(out).Encode(clientauthenticationv1.ExecCredential{
		TypeMeta: metav1.TypeMeta{APIVersion: clientauthenticationv1.SchemeGroupVersion.String(), Kind: "ExecCredential"},
		Status:   status,
	})
}

func execInfoServer() string {
	var info clientauthenticationv1.ExecCredential
	if json.Unmarshal([]byte(os.Getenv(execInfoEnv)), &info) != nil || info.Spec.Cluster == nil {
		return ""
	}
	return info.Spec.Cluster.Server
}

func accessToken(app string) (string, error) {
	if token := strings.TrimSpace(os.Getenv(accessTokenEnv)); token != "" {
		return token, nil
	}
	cmd := exec.Command("cloudflared", "access", "token", "-app="+app)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("no Access token for %s: cloudflared access token: %w; run \"cloudflared access login %s\" or set $%s", app, err, app, accessTokenEnv)
	}
	return strings.TrimSpace(string(stdout)), nil
}
