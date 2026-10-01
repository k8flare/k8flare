//go:build !js

package apiserver_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	devToken         = "k8flare-dev-token"
	joinToken        = "k8flare-dev-join"
	readonlyDevToken = "k8flare-dev-readonly"

	secretsEncryptionKeys = "test:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
)

// startDev runs `wrangler dev` for the worker with a throwaway state
// directory and returns a clientset for it. The wasm assets must already be
// built (make wasm).
func startDev(t *testing.T) *kubernetes.Clientset {
	t.Helper()
	_, cs := startDevURL(t)
	return cs
}

var devState string

func startDevURL(t *testing.T) (string, *kubernetes.Clientset) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "../..")
	workers := []string{"apiserver", "openapi", "customresources", "scheduler", "workloads", "gc", "admission"}
	for _, g := range []string{"core", "coordination", "discovery", "events", "node", "storage", "authentication", "authorization", "apps", "policy", "resource", "rbac", "batch", "admissionregistration", "autoscaling", "scheduling", "networking", "certificates", "flowcontrol", "apiregistration"} {
		workers = append(workers, "apiserver-"+g)
	}
	for _, g := range []string{"core", "coordination", "discovery", "node", "storage", "apps", "policy", "resource", "rbac", "batch", "autoscaling", "scheduling", "networking", "certificates", "flowcontrol"} {
		workers = append(workers, "printers-"+g)
	}
	for _, w := range workers {
		m := "packages/control-plane-worker/assets/wasm/" + w + ".manifest.json"
		if _, err := os.Stat(filepath.Join(root, m)); err != nil {
			t.Fatalf("wasm assets missing; run make wasm: %v", err)
		}
	}
	port := freePort(t)
	state := t.TempDir()
	devState = state
	cmd := exec.Command("pnpm", "exec", "wrangler", "dev", "--local", "--enable-containers=false",
		"-c", "wrangler.jsonc",
		"--persist-to", state, "--port", fmt.Sprint(port), "--inspector-port", "0",
		"--local-upstream", fmt.Sprintf("127.0.0.1:%d", port),
		"--var", "ADMIN_TOKEN:"+devToken, "--var", "READONLY_TOKEN:"+readonlyDevToken, "--var", "JOIN_TOKEN:"+joinToken,
		"--var", "SECRETS_ENCRYPTION_KEYS:"+secretsEncryptionKeys)
	cmd.Dir = root
	cmd.Env = devEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	logFile, err := os.Create(filepath.Join(state, "wrangler.log"))
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		_ = cmd.Wait()
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if t.Failed() {
			log, _ := os.ReadFile(logFile.Name())
			t.Logf("wrangler dev log:\n%s", log)
		}
	})
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(3 * time.Minute)
	for _, path := range []string{"/version", "/api/v1/namespaces/default"} {
		for {
			req, _ := http.NewRequest(http.MethodGet, base+path, nil)
			req.Header.Set("Authorization", "Bearer "+devToken)
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					break
				}
				err = fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
			}
			if time.Now().After(deadline) {
				t.Fatalf("wrangler dev did not become ready: %v", err)
			}
			time.Sleep(time.Second)
		}
	}
	cs, err := kubernetes.NewForConfig(devConfig(base, devToken))
	if err != nil {
		t.Fatal(err)
	}
	waitForFirstControllerPass(t, cs, deadline)
	return base, cs
}

func waitForFirstControllerPass(t *testing.T, cs *kubernetes.Clientset, deadline time.Time) {
	t.Helper()
	for {
		cm, err := cs.CoreV1().ConfigMaps("default").Get(context.Background(), "kube-root-ca.crt", metav1.GetOptions{})
		if err == nil && cm.Annotations["kubernetes.io/description"] != "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the root CA publisher did not reach the default namespace: %v", err)
		}
		time.Sleep(time.Second)
	}
}

func devConfig(base, token string) *rest.Config {
	return &rest.Config{Host: base, BearerToken: token, Transport: &http.Transport{IdleConnTimeout: 2 * time.Second}}
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return c
}

// devEnv drops the variables that put wrangler dev into its AI-agent
// mode, whose observability capture buffers application/json streaming
// responses until they close and so stalls every JSON watch.
func devEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "CLAUDECODE=") || strings.HasPrefix(kv, "AI_AGENT=") {
			continue
		}
		env = append(env, kv)
	}
	return env
}
