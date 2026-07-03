//go:build !js

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/k3s-io/k3s/pkg/agent"
	"github.com/k3s-io/k3s/pkg/cli/cmds"
	"github.com/k3s-io/k3s/pkg/daemons/executor"
	"github.com/k3s-io/k3s/pkg/executor/embed"
	"github.com/k8flare/k8flare/pkg/cacert"
)

// prepareK3sDataDir ensures k3s data directory is extracted and adds its
// bin directories to PATH. The embedded executor's Bootstrap method uses
// exec.LookPath("host-local") to find CNI plugins, so these directories
// must be in PATH before agent.Run is called.
func prepareK3sDataDir(dataDir string) error {
	// Run "k3s check-config" to trigger data extraction.
	// k3s extracts embedded binaries (CNI plugins, containerd, runc, etc.)
	// into <dataDir>/data/<hash>/bin/ on first run.
	k3sBin, err := exec.LookPath("k3s")
	if err != nil {
		return err
	}
	cmd := exec.Command(k3sBin, "check-config")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Ignore exit code; check-config may warn about missing features
	// but the important side effect is data extraction.
	cmd.Run()

	// The "current" symlink points to the active data directory.
	currentDir := filepath.Join(dataDir, "data", "current")
	binDir := filepath.Join(currentDir, "bin")
	cniDir := filepath.Join(dataDir, "data", "cni")

	// Prepend k3s bin directories to PATH so that exec.LookPath finds
	// CNI plugins (host-local, bridge, flannel, loopback, portmap).
	pathParts := []string{cniDir, binDir}
	if existing := os.Getenv("PATH"); existing != "" {
		pathParts = append(pathParts, existing)
	}
	os.Setenv("PATH", strings.Join(pathParts, string(os.PathListSeparator)))

	log.Printf("PATH updated: prepended %s and %s", cniDir, binDir)
	return nil
}

func main() {
	serverURL := flag.String("server", "", "Control plane URL (e.g., https://your-k8flare.workers.dev)")
	token := flag.String("token", os.Getenv("K3S_TOKEN"), "Cluster token")
	nodeName := flag.String("node-name", "", "Node name (default: hostname)")
	dataDir := flag.String("data-dir", "/var/lib/rancher/k3s", "Data directory")
	tunnelToken := flag.String("tunnel-token", os.Getenv("TUNNEL_TOKEN"), "Cloudflare tunnel token")
	nodeExternalIP := flag.String("node-external-ip", "", "Node external IP to advertise (needed for flannel wireguard-native across networks that don't share L2; see docs/cloudflare-mesh-networking.md)")
	flag.Parse()

	if *serverURL == "" {
		log.Fatal("--server is required")
	}
	if *token == "" {
		log.Fatal("--token is required")
	}
	if *nodeName == "" {
		hostname, _ := os.Hostname()
		*nodeName = hostname
	}

	_ = *tunnelToken // TODO: use for cloudflared tunnel

	// Prepare k3s data directory and set PATH for CNI plugins.
	if err := prepareK3sDataDir(*dataDir); err != nil {
		log.Fatalf("failed to prepare k3s data dir: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	var wg sync.WaitGroup

	agentConfig := cmds.Agent{}
	agentConfig.Token = *token
	agentConfig.ServerURL = *serverURL
	agentConfig.NodeName = *nodeName
	agentConfig.DataDir = *dataDir
	agentConfig.DisableLoadBalancer = true
	agentConfig.WithNodeID = true
	if *nodeExternalIP != "" {
		agentConfig.NodeExternalIP.Set(*nodeExternalIP)
	}

	log.Printf("Starting k3s-cf-agent: server=%s node=%s", *serverURL, *nodeName)

	// Replace server-ca.crt with system CA bundle after k3s writes it.
	// Cloudflare Workers uses a publicly trusted TLS cert, not our self-signed CA.
	go cacert.ReplaceServerCA(ctx, *dataDir)

	// Patch kubeconfigs to use token auth instead of client certificate auth.
	// Cloudflare terminates TLS, so client certs never reach the Workers control plane.
	go cacert.PatchKubeconfigs(ctx, *dataDir, *token)

	// Write /run/flannel/subnet.env directly by querying the API for PodCIDR.
	// The k3s flannel informer often fails to sync during startup because the
	// Go WASM API handler is temporarily overloaded. This bypasses the informer.
	go cacert.WriteSubnetEnv(ctx, *serverURL, *token, *nodeName)

	embedded, err := embed.New(ctx, &agentConfig)
	if err != nil {
		log.Fatalf("failed to create embedded executor: %v", err)
	}
	executor.Set(embedded)

	if err := agent.Run(ctx, &wg, agentConfig); err != nil {
		log.Fatalf("agent failed: %v", err)
	}

	// agent.Run starts goroutines and returns nil; block until signal.
	<-ctx.Done()
	log.Println("Shutting down k3s-cf-agent")
	wg.Wait()
}
