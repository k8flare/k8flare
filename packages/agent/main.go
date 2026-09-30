//go:build linux

// Command agent runs the unmodified k3s agent (containerd, kubelet, flannel)
// against a k8flare control plane. What it changes is how the agent
// authenticates to the control plane: TLS terminates at an edge that never
// sees client certificates, so kubeconfigs and the remotedialer tunnel
// connect carry a bearer token made of the node name and the node password
// k3s already registered with the supervisor, instead of a client cert.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/k3s-io/k3s/pkg/agent"
	"github.com/k3s-io/k3s/pkg/agent/tunnel"
	"github.com/k3s-io/k3s/pkg/cli/cmds"
	"github.com/k3s-io/k3s/pkg/daemons/control/deps"
	"github.com/k3s-io/k3s/pkg/daemons/executor"
	"github.com/k3s-io/k3s/pkg/executor/embed"
	"github.com/k3s-io/k3s/pkg/signals"
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
	nodeLabels := flag.String("node-labels", "", "")
	nodeTaints := flag.String("node-taints", "", "")
	withNodeID := flag.Bool("with-node-id", false, "")
	_ = flag.Int("kubelet-plain-proxy-port", 0, "")
	_ = flag.String("virtual-kube-proxy-cidr", "", "")
	meshAsNodeIP := flag.Bool("mesh-ip-as-node-ip", false, "")
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
	tunnel.TunnelIgnoreEndpointSlices = true
	tunnel.TunnelHeaderOverride = func() http.Header {
		password, err := os.ReadFile("/etc/rancher/node/password")
		if err != nil {
			return nil
		}
		h := http.Header{}
		h.Set("Authorization", "Bearer node:"+*nodeName+":"+strings.TrimSpace(string(password)))
		return h
	}
	ctx := signals.SetupSignalContext()
	cfg := cmds.Agent{
		Token:               *token,
		ServerURL:           *server,
		NodeName:            *nodeName,
		DataDir:             *dataDir,
		DisableLoadBalancer: true,
		WithNodeID:          *withNodeID,
	}
	_ = cfg.Labels.Set("k8flare.com/agent=k8flare")
	if *nodeLabels != "" {
		_ = cfg.Labels.Set(*nodeLabels)
	}
	if *nodeTaints != "" {
		_ = cfg.Taints.Set(*nodeTaints)
	}
	if *meshAsNodeIP {
		ip := meshIPv4()
		for i := 0; i < 30 && ip == ""; i++ {
			time.Sleep(time.Second)
			ip = meshIPv4()
		}
		if ip != "" {
			_ = cfg.NodeIP.Set(ip)
			cfg.FlannelIface = "CloudflareWARP"
		}
	}
	embedded, err := embed.New(ctx, &cfg)
	if err != nil {
		log.Fatalf("embedded executor: %v", err)
	}
	executor.Set(embedded)
	var wg sync.WaitGroup
	go serveKubernetesAPI(ctx, *server, *nodeName, *token)
	log.Printf("starting k8flare agent: server=%s node=%s", *server, *nodeName)
	if err := agent.Run(ctx, &wg, cfg); err != nil {
		log.Fatalf("agent: %v", err)
	}
	<-ctx.Done()
	wg.Wait()
}

func meshIPv4() string {
	_, mesh, err := net.ParseCIDR("100.96.0.0/12")
	if err != nil {
		return ""
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	var fallback string
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil || !mesh.Contains(n.IP) {
				continue
			}
			if iface.Name == "CloudflareWARP" {
				return n.IP.String()
			}
			if fallback == "" {
				fallback = n.IP.String()
			}
		}
	}
	return fallback
}

// useBundledBinaries puts the containerd, runc and CNI binaries the k3s
// distribution unpacks into its data directory on PATH. The stock k3s
// binary must have run once on this machine.
func useBundledBinaries(dataDir string) error {
	bin := filepath.Join(dataDir, "data/current/bin")
	if _, err := os.Stat(bin); err != nil {
		k3s := "k3s"
		if _, lookErr := exec.LookPath(k3s); lookErr != nil {
			k3s = "/usr/local/bin/k3s"
		}
		cmd := exec.Command(k3s, "kubectl", "version", "--client")
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			return fmt.Errorf("%s not found: unpack k3s: %v: %s", bin, runErr, strings.TrimSpace(string(out)))
		}
	}
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("%s not found: run the k3s binary once to unpack it", bin)
	}
	return os.Setenv("PATH", strings.Join([]string{filepath.Join(dataDir, "data/cni"), bin, os.Getenv("PATH")}, string(os.PathListSeparator)))
}
