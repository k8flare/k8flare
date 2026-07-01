//go:build !js

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	kubeschedulerconfigv1 "k8s.io/kube-scheduler/config/v1"
	sapp "k8s.io/kubernetes/cmd/kube-scheduler/app"
	"sigs.k8s.io/yaml"
)

func main() {
	serverURL := flag.String("server", "", "Control plane URL (e.g., https://your-k8flare.workers.dev)")
	token := flag.String("token", os.Getenv("K3S_TOKEN"), "Cluster token")
	dataDir := flag.String("data-dir", "/var/lib/rancher/k8flare-scheduler", "Directory for the generated kubeconfig and scheduler config")
	verbosity := flag.String("v", "0", "klog verbosity level, forwarded to the underlying kube-scheduler")
	insecureSkipTLSVerify := flag.Bool("insecure-skip-tls-verify", false, "Skip TLS certificate verification (for local/self-signed dev servers only)")
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

	schedulerConfigPath := filepath.Join(*dataDir, "scheduler-config.yaml")
	if err := writeSchedulerConfig(schedulerConfigPath, kubeconfigPath); err != nil {
		log.Fatalf("failed to write scheduler config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	log.Printf("Starting k8flare-scheduler: server=%s", *serverURL)

	// Same embedding pattern k3s itself uses for kube-scheduler
	// (pkg/executor/embed/embed.go's Scheduler method): NewSchedulerCommand
	// -> SetArgs -> ExecuteContext, no direct Setup/Run calls.
	command := sapp.NewSchedulerCommand(ctx.Done())
	command.SetArgs([]string{
		"--config=" + schedulerConfigPath,
		"--leader-elect=false",
		"--v=" + *verbosity,
	})

	if err := command.ExecuteContext(ctx); err != nil {
		log.Fatalf("scheduler failed: %v", err)
	}
}

// writeKubeconfig writes a kubeconfig authenticating with the shared cluster
// bearer token. k8flare's AuthMiddleware treats any token equal to the
// cluster token as authenticated with system:masters, so no client-cert
// bootstrap (unlike cmd/agent's cacert dance) is needed here, and Cloudflare
// Workers serves a publicly-trusted TLS cert, so no custom CA is needed either.
//
// serverURL must use https:// for the token to actually be sent: client-go's
// clientcmd only applies a kubeconfig's credentials — including the bearer
// token — when rest.IsConfigTransportTLS reports true (client-go's
// client_config.go: "only try to read the auth information if we are
// secure"), so a plain-http server here would make every request silently
// unauthenticated rather than fail loudly. insecureSkipTLSVerify is for
// local/self-signed dev servers only; production Cloudflare Workers serving
// already has a publicly-trusted cert and doesn't need it.
func writeKubeconfig(path, serverURL, token string, insecureSkipTLSVerify bool) error {
	cfg := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"k8flare": {Server: serverURL, InsecureSkipTLSVerify: insecureSkipTLSVerify},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"scheduler": {Token: token},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"k8flare": {Cluster: "k8flare", AuthInfo: "scheduler"},
		},
		CurrentContext: "k8flare",
	}
	return clientcmd.WriteToFile(cfg, path)
}

// writeSchedulerConfig writes a KubeSchedulerConfiguration disabling the
// PV/PVC/StorageClass-touching plugins this apiserver can't back (unlike
// Dynamic Resource Allocation's ResourceClaim/ResourceSlice informers, which
// are started unconditionally once the GA-locked DRA feature gate is on,
// these are ordinary plugins that stop watching their GVKs once disabled).
// Every field left unset here is filled in by the scheduler's own defaulting
// on load, same as any minimal hand-written scheduler config.
func writeSchedulerConfig(path, kubeconfigPath string) error {
	leaderElect := false

	cfg := kubeschedulerconfigv1.KubeSchedulerConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubescheduler.config.k8s.io/v1",
			Kind:       "KubeSchedulerConfiguration",
		},
		ClientConnection: componentbaseconfigv1alpha1.ClientConnectionConfiguration{
			Kubeconfig: kubeconfigPath,
			QPS:        50,
			Burst:      100,
		},
		LeaderElection: componentbaseconfigv1alpha1.LeaderElectionConfiguration{
			LeaderElect: &leaderElect,
		},
		Profiles: []kubeschedulerconfigv1.KubeSchedulerProfile{
			{
				Plugins: &kubeschedulerconfigv1.Plugins{
					MultiPoint: disabledPlugins("VolumeBinding", "VolumeRestrictions", "NodeVolumeLimits", "VolumeZone"),
				},
			},
		},
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal scheduler config: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

func disabledPlugins(names ...string) kubeschedulerconfigv1.PluginSet {
	plugins := make([]kubeschedulerconfigv1.Plugin, len(names))
	for i, name := range names {
		plugins[i] = kubeschedulerconfigv1.Plugin{Name: name}
	}
	return kubeschedulerconfigv1.PluginSet{Disabled: plugins}
}
