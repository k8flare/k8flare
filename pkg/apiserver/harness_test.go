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

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	devToken  = "k8flare-dev-token"
	joinToken = "k8flare-dev-join"
)

// startDev runs `wrangler dev` for the worker with a throwaway state
// directory and returns a clientset for it. The wasm assets must already be
// built (make wasm).
func startDev(t *testing.T) *kubernetes.Clientset {
	t.Helper()
	_, cs := startDevURL(t)
	return cs
}

func startDevURL(t *testing.T) (string, *kubernetes.Clientset) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "../..")
	if _, err := os.Stat(filepath.Join(root, "worker/assets/wasm/apiserver.manifest.json")); err != nil {
		t.Fatalf("wasm assets missing; run make wasm: %v", err)
	}
	port := freePort(t)
	state := t.TempDir()
	cmd := exec.Command("pnpm", "exec", "wrangler", "dev", "--local",
		"--persist-to", state, "--port", fmt.Sprint(port), "--inspector-port", "0")
	cmd.Dir = filepath.Join(root, "worker")
	cmd.Env = append(devEnv(), "ADMIN_TOKEN="+devToken, "JOIN_TOKEN="+joinToken)
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
		if t.Failed() {
			log, _ := os.ReadFile(logFile.Name())
			t.Logf("wrangler dev log:\n%s", log)
		}
	})
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(2 * time.Minute)
	for {
		req, _ := http.NewRequest(http.MethodGet, base+"/version", nil)
		req.Header.Set("Authorization", "Bearer "+devToken)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("wrangler dev did not become ready: %v", err)
		}
		time.Sleep(time.Second)
	}
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: base, BearerToken: devToken})
	if err != nil {
		t.Fatal(err)
	}
	return base, cs
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
