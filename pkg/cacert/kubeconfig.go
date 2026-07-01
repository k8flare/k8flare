//go:build !js

package cacert

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"k8s.io/client-go/tools/clientcmd"
)

// PatchKubeconfigs watches for k3s-generated kubeconfig files and replaces
// client certificate authentication with bearer token authentication.
//
// Cloudflare Workers terminates TLS, so client certificates never reach our
// control plane. By switching to bearer token auth, kubelet and kube-proxy
// can authenticate via the cluster token, which our auth middleware accepts.
//
// This function polls at a fast interval (10ms) until all kubeconfig files
// are initially patched, then continues monitoring at a slower interval (1s)
// to re-patch if k3s overwrites them (e.g. during cert rotation).
// The fast initial poll is critical because k3s writes kubeconfigs and then
// immediately reads them to set up API server readiness checks and kubelet;
// if patching is delayed, the agent will use client certificate auth which
// fails through Cloudflare Workers.
func PatchKubeconfigs(ctx context.Context, dataDir, token string) {
	kubeconfigs := []string{
		filepath.Join(dataDir, "agent", "kubelet.kubeconfig"),
		filepath.Join(dataDir, "agent", "kubeproxy.kubeconfig"),
		filepath.Join(dataDir, "agent", "k3scontroller.kubeconfig"),
	}

	remaining := make(map[string]bool)
	for _, kc := range kubeconfigs {
		remaining[kc] = true
	}

	// Fast poll (10ms) until all files are initially patched.
	fastTicker := time.NewTicker(10 * time.Millisecond)
	defer fastTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-fastTicker.C:
			for path := range remaining {
				if _, err := os.Stat(path); err != nil {
					continue
				}
				if err := patchKubeconfig(path, token); err != nil {
					log.Printf("cacert: failed to patch %s: %v", path, err)
					continue
				}
				log.Printf("cacert: patched %s to use token auth", path)
				delete(remaining, path)
			}
			if len(remaining) == 0 {
				goto monitor
			}
		}
	}

monitor:
	// After initial patching, monitor for kubeconfig rewrites (cert rotation).
	// Track file modification times to detect changes.
	fastTicker.Stop()
	modTimes := make(map[string]time.Time)
	for _, kc := range kubeconfigs {
		if info, err := os.Stat(kc); err == nil {
			modTimes[kc] = info.ModTime()
		}
	}

	slowTicker := time.NewTicker(time.Second)
	defer slowTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-slowTicker.C:
			for _, path := range kubeconfigs {
				info, err := os.Stat(path)
				if err != nil {
					continue
				}
				if prev, ok := modTimes[path]; ok && info.ModTime().Equal(prev) {
					continue // no change
				}
				if needsPatch(path, token) {
					if err := patchKubeconfig(path, token); err != nil {
						log.Printf("cacert: failed to re-patch %s: %v", path, err)
						continue
					}
					log.Printf("cacert: re-patched %s to use token auth", path)
				}
				modTimes[path] = info.ModTime()
			}
		}
	}
}

// needsPatch checks if a kubeconfig still uses client certificate auth
// instead of token auth.
func needsPatch(path, token string) bool {
	config, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return false
	}
	for _, authInfo := range config.AuthInfos {
		if authInfo.Token != token {
			return true
		}
		if authInfo.ClientCertificate != "" || len(authInfo.ClientCertificateData) > 0 {
			return true
		}
	}
	return false
}

func patchKubeconfig(path, token string) error {
	config, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return err
	}

	for _, authInfo := range config.AuthInfos {
		authInfo.Token = token
		authInfo.ClientCertificate = ""
		authInfo.ClientCertificateData = nil
		authInfo.ClientKey = ""
		authInfo.ClientKeyData = nil
	}

	return clientcmd.WriteToFile(*config, path)
}
