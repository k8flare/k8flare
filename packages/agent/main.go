//go:build linux

// Command agent runs the unmodified k3s agent (containerd, kubelet, flannel)
// against a k8flare control plane. What it changes is how the agent
// authenticates to the control plane: TLS terminates at an edge that never
// sees client certificates, so kubeconfigs and the remotedialer tunnel
// connect carry a bearer token made of the node name and the node password
// k3s already registered with the supervisor, instead of a client cert.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/k3s-io/k3s/pkg/agent"
	"github.com/k3s-io/k3s/pkg/agent/tunnel"
	"github.com/k3s-io/k3s/pkg/cli/cmds"
	"github.com/k3s-io/k3s/pkg/daemons/control/deps"
	"github.com/k3s-io/k3s/pkg/daemons/executor"
	"github.com/k3s-io/k3s/pkg/executor/embed"
)

const kubeconfigTemplate = `apiVersion: v1
clusters:
- cluster:
    server: %s
  name: local
contexts:
- context:
    cluster: local
    namespace: default
    user: user
  name: Default
current-context: Default
kind: Config
preferences: {}
users:
- name: user
  user:
    token: %s
`

func main() {
	server := flag.String("server", "", "control plane URL (https://...)")
	token := flag.String("token", os.Getenv("K3S_TOKEN"), "cluster join token")
	nodeName := flag.String("node-name", "", "node name (default: hostname)")
	dataDir := flag.String("data-dir", "/var/lib/rancher/k3s", "k3s data directory")
	flag.Parse()
	if *server == "" || *token == "" {
		log.Fatal("--server and --token are required")
	}
	if *nodeName == "" {
		*nodeName, _ = os.Hostname()
	}
	if err := useBundledBinaries(*dataDir); err != nil {
		log.Fatalf("k3s bundled binaries: %v", err)
	}
	deps.KubeConfigOverride = func(dest, _, _, _, _ string) (bool, error) {
		password, err := os.ReadFile("/etc/rancher/node/password")
		if err != nil {
			return false, err
		}
		nodeToken := fmt.Sprintf("node:%s:%s", *nodeName, strings.TrimSpace(string(password)))
		log.Printf("writing token kubeconfig %s", dest)
		return true, os.WriteFile(dest, []byte(fmt.Sprintf(kubeconfigTemplate, *server, nodeToken)), 0o600)
	}
	tunnel.TunnelHeaderOverride = func() http.Header {
		password, err := os.ReadFile("/etc/rancher/node/password")
		if err != nil {
			return nil
		}
		h := http.Header{}
		h.Set("Authorization", "Bearer node:"+*nodeName+":"+strings.TrimSpace(string(password)))
		return h
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	cfg := cmds.Agent{
		Token:               *token,
		ServerURL:           *server,
		NodeName:            *nodeName,
		DataDir:             *dataDir,
		DisableLoadBalancer: true,
	}
	embedded, err := embed.New(ctx, &cfg)
	if err != nil {
		log.Fatalf("embedded executor: %v", err)
	}
	executor.Set(embedded)
	var wg sync.WaitGroup
	log.Printf("starting k8flare agent: server=%s node=%s", *server, *nodeName)
	if err := agent.Run(ctx, &wg, cfg); err != nil {
		log.Fatalf("agent: %v", err)
	}
	<-ctx.Done()
	wg.Wait()
}

// useBundledBinaries puts the containerd, runc and CNI binaries the k3s
// distribution unpacks into its data directory on PATH. The stock k3s
// binary must have run once on this machine.
func useBundledBinaries(dataDir string) error {
	bin := filepath.Join(dataDir, "data/current/bin")
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("%s not found: run the k3s binary once to unpack it", bin)
	}
	return os.Setenv("PATH", strings.Join([]string{filepath.Join(dataDir, "data/cni"), bin, os.Getenv("PATH")}, string(os.PathListSeparator)))
}
