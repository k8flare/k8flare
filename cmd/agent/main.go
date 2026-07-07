//go:build !js

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
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
	"github.com/k8flare/k8flare/pkg/dnsshim"
	"github.com/k8flare/k8flare/pkg/meshconnector"
	"github.com/k8flare/k8flare/pkg/vkubeproxy"
	cli "github.com/urfave/cli/v2"
)

// runKubeletPlainProxy serves the kubelet's authenticated HTTPS API
// (10250: /containerLogs, /exec, ...) over plain HTTP on the given port.
// Cloudflare Workers cannot speak TLS to a self-signed kubelet cert
// through containerFetch, so on per-Pod microVM nodes the gateway's
// logs/metrics bridge dials this port instead. Only meaningful together
// with the node image's kubelet drop-in (anonymous auth + AlwaysAllow):
// the VM runs exactly one Pod and has no inbound network path except the
// token-gated Worker, so TLS+authz on the last localhost hop adds
// nothing (documented in README's node backend section).
func runKubeletPlainProxy(port int) {
	target := &url.URL{Scheme: "https", Host: "127.0.0.1:10250"}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	proxy.FlushInterval = -1 // stream `kubectl logs -f` line by line
	addr := fmt.Sprintf(":%d", port)
	log.Printf("kubelet plain-HTTP proxy listening on %s -> %s", addr, target)
	if err := http.ListenAndServe(addr, proxy); err != nil {
		log.Printf("kubelet plain-HTTP proxy failed: %v", err)
	}
}

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
	meshConnectorToken := flag.String("mesh-connector-token", os.Getenv("MESH_CONNECTOR_TOKEN"), "Cloudflare Mesh connector token (mint via POST /accounts/{id}/warp_connector then GET .../token -- no dashboard needed, see spikes/s17-mesh-nodevm/FINDINGS.md gate 2). If set, this node joins Mesh and advertises its Mesh IP as --node-external-ip automatically; requires the cloudflare-warp package pre-installed on the node image")
	meshIPAsNodeIP := flag.Bool("mesh-ip-as-node-ip", false, "When --mesh-connector-token is set, ALSO set the discovered Mesh IP as kubelet's --node-ip (k3s --node-ip), not just --node-external-ip. Per-Pod microVM nodes only (images/node/entrypoint.sh passes this): hostNetwork:true Pods inherit their PodIP from the Node's InternalIP, which real kubelet sources from --node-ip, not --node-external-ip (verified against k3s-io/kubernetes's kubelet_pods.go getHostIPsAnyWay + pkg/daemons/agent/agent_linux.go's node-ip arg wiring -- see spikes/s17-mesh-nodevm/FINDINGS.md's per-Pod-Mesh entry). BYO VM nodes must leave this false: flannel wireguard-native needs the real internal IP undisturbed and only reads --node-external-ip for cross-cloud reachability")
	nodeExternalIP := flag.String("node-external-ip", "", "Node external IP to advertise (needed for flannel wireguard-native across networks that don't share L2; see docs/cloudflare-mesh-networking.md). Overridden by --mesh-connector-token's discovered Mesh IP if both are set")
	nodeLabels := flag.String("node-labels", "", "Comma-separated key=value labels the kubelet registers its Node with (k3s --node-label). Per-Pod microVM nodes use this for the k8flare.com/backend selector label")
	nodeTaints := flag.String("node-taints", "", "Comma-separated key=value:Effect taints the kubelet registers its Node with (k3s --node-taint). Per-Pod microVM nodes use this for the pod-on-containers NoSchedule taint")
	withNodeID := flag.Bool("with-node-id", true, "Append a unique ID suffix to the node name (k3s --with-node-id). Disable for per-Pod microVM nodes, whose names must match exactly what workers/nodes' cf-containers-scheduler registered and will later bind to / tear down")
	kubeletPlainProxyPort := flag.Int("kubelet-plain-proxy-port", 0, "Serve the kubelet's HTTPS API (10250) over plain HTTP on this port for the Workers logs/metrics bridge (0 = disabled). Per-Pod microVM nodes only; pair with the node image's kubelet auth drop-in")
	virtualKubeProxyCIDR := flag.String("virtual-kube-proxy-cidr", "", "Service CIDR (e.g. 10.43.0.0/16) to intercept and forward via pkg/vkubeproxy's TUN+userspace-TCP bridge (empty = disabled). Per-Pod microVM nodes only: hostNetwork:true means the real embedded kube-proxy has no netfilter to route ClusterIP traffic with there, unlike a BYO VM node, which never needs this")
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

	// Prepare k3s data directory and set PATH for CNI plugins.
	if err := prepareK3sDataDir(*dataDir); err != nil {
		log.Fatalf("failed to prepare k3s data dir: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Join Mesh before building agentConfig -- the discovered Mesh IP
	// must be present in the very first Node registration, not patched
	// in after the fact.
	var meshIP string
	if *meshConnectorToken != "" {
		var err error
		meshIP, err = meshconnector.Run(ctx, *meshConnectorToken)
		if err != nil {
			log.Fatalf("failed to join Cloudflare Mesh: %v", err)
		}
		log.Printf("joined Cloudflare Mesh, IP: %s", meshIP)
		*nodeExternalIP = meshIP
	}

	var wg sync.WaitGroup

	agentConfig := cmds.Agent{}
	agentConfig.Token = *token
	agentConfig.ServerURL = *serverURL
	agentConfig.NodeName = *nodeName
	agentConfig.DataDir = *dataDir
	agentConfig.DisableLoadBalancer = true
	agentConfig.WithNodeID = *withNodeID
	if *nodeLabels != "" {
		agentConfig.Labels = *cli.NewStringSlice(strings.Split(*nodeLabels, ",")...)
	}
	if *nodeTaints != "" {
		agentConfig.Taints = *cli.NewStringSlice(strings.Split(*nodeTaints, ",")...)
	}
	if *nodeExternalIP != "" {
		agentConfig.NodeExternalIP.Set(*nodeExternalIP)
	}
	if *meshIPAsNodeIP && meshIP != "" {
		agentConfig.NodeIP.Set(meshIP)
	}

	log.Printf("Starting k3s-cf-agent: server=%s node=%s", *serverURL, *nodeName)

	if *kubeletPlainProxyPort > 0 {
		go runKubeletPlainProxy(*kubeletPlainProxyPort)
	}

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

	// Cluster DNS: node-local shim (NodeLocal DNSCache address), no
	// Containers/CoreDNS Deployment dependency. kubelet's --cluster-dns
	// is set to the same address via supervisor.go's clusterConfig.
	go dnsshim.Run(ctx, *serverURL, *token, "cluster.local")

	// Virtual kube-proxy (task #13): only set on the Containers node
	// image's entrypoint.sh, never for BYO VM nodes (see the flag's own
	// help text above).
	if *virtualKubeProxyCIDR != "" {
		go vkubeproxy.Run(ctx, *serverURL, *token, *virtualKubeProxyCIDR)
	}

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
