//go:build !js

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	cmapp "k8s.io/kubernetes/cmd/kube-controller-manager/app"
)

func main() {
	serverURL := flag.String("server", "", "Control plane URL (e.g., https://your-k8flare.workers.dev)")
	token := flag.String("token", os.Getenv("K3S_TOKEN"), "Cluster token")
	dataDir := flag.String("data-dir", "/var/lib/rancher/k8flare-controller-manager", "Directory for the generated kubeconfig")
	verbosity := flag.String("v", "0", "klog verbosity level, forwarded to the underlying kube-controller-manager")
	insecureSkipTLSVerify := flag.Bool("insecure-skip-tls-verify", false, "Skip TLS certificate verification (for local/self-signed dev servers only)")
	// endpoint,endpointslice replace workers/storage/src/endpoints.ts; nodeipam
	// replaces workers/storage/src/scheduler.ts's PodCIDR allocation;
	// nodelifecycle,taint-eviction-controller replace
	// workers/storage/src/nodelifecycle.ts (Phase 5, "TS reconcilers -> real
	// kube-controller-manager"). Names verified against the vendored
	// k8s.io/kubernetes source, not assumed: IsControllerEnabled
	// (k8s.io/controller-manager's app/helper.go) supports only an exact
	// name, "-name", or "*" -- there is no "+name" syntax. node-lifecycle's
	// alias is "nodelifecycle" (no hyphen); taint-eviction has no alias, only
	// the literal canonical name "taint-eviction-controller"
	// (newTaintEvictionControllerDescriptor, cmd/kube-controller-manager/app/core.go).
	controllers := flag.String("controllers", "replicaset,deployment,daemonset,job,cronjob,endpoint,endpointslice,nodeipam,nodelifecycle,taint-eviction-controller", "Comma-separated controllers to enable, forwarded to --controllers")
	flag.Parse()

	if *serverURL == "" {
		log.Fatal("--server is required")
	}
	if *token == "" {
		log.Fatal("--token is required")
	}

	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		log.Fatalf("failed to create data dir: %v", err)
	}

	kubeconfigPath := filepath.Join(*dataDir, "kubeconfig.yaml")
	if err := writeKubeconfig(kubeconfigPath, *serverURL, *token, *insecureSkipTLSVerify); err != nil {
		log.Fatalf("failed to write kubeconfig: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	log.Printf("Starting k8flare-controller-manager: server=%s controllers=%s", *serverURL, *controllers)

	// Same embedding pattern k3s itself uses for kube-controller-manager
	// (pkg/executor/embed/embed.go's ControllerManager method), and the same
	// pattern this project already uses for the real kube-scheduler
	// (cmd/scheduler/main.go): NewControllerManagerCommand -> SetArgs ->
	// ExecuteContext, no direct Setup/Run calls. Unlike NewSchedulerCommand,
	// this constructor takes no stop-channel argument -- cancellation flows
	// in only through ExecuteContext's context.
	//
	// --secure-port=0 disables the controller-manager's own HTTPS
	// healthz/metrics server, which isn't needed here and would otherwise
	// try to bind a port and set up delegated authentication/authorization
	// against a real apiserver's TokenReview/SubjectAccessReview APIs that
	// this project doesn't implement.
	command := cmapp.NewControllerManagerCommand()
	command.SetArgs([]string{
		"--kubeconfig=" + kubeconfigPath,
		"--leader-elect=false",
		"--controllers=" + *controllers,
		"--secure-port=0",
		// Required for the nodeipam controller to allocate anything (it's a
		// silent no-op otherwise); /24-per-node from this /16 matches the
		// k3s default cluster CIDR cmd/agent's embedded flannel expects, and
		// the /24s the deleted scheduler.ts used to hand out by hand.
		"--allocate-node-cidrs=true",
		"--cluster-cidr=10.42.0.0/16",
		"--v=" + *verbosity,
	})

	if err := command.ExecuteContext(ctx); err != nil {
		log.Fatalf("controller-manager failed: %v", err)
	}
}

// writeKubeconfig writes a kubeconfig authenticating with the shared cluster
// bearer token. Identical rationale to cmd/scheduler/main.go's
// writeKubeconfig: k8flare's AuthMiddleware treats any token equal to the
// cluster token as authenticated system:masters (no client-cert bootstrap),
// and serverURL must use https:// for client-go's clientcmd to actually send
// the token (rest.IsConfigTransportTLS gates it).
func writeKubeconfig(path, serverURL, token string, insecureSkipTLSVerify bool) error {
	cfg := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"k8flare": {Server: serverURL, InsecureSkipTLSVerify: insecureSkipTLSVerify},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"controller-manager": {Token: token},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"k8flare": {Cluster: "k8flare", AuthInfo: "controller-manager"},
		},
		CurrentContext: "k8flare",
	}
	return clientcmd.WriteToFile(cfg, path)
}
