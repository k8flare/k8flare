//go:build !js

// Package cacert replaces the k3s server-ca.crt with the system CA bundle.
//
// When k3s bootstraps, it downloads /v1-k3s/server-ca.crt (the k3s self-signed
// CA) and stores it locally. All subsequent TLS connections (kubelet kubeconfig,
// remotedialer, readyz) use this CA to verify the server's TLS certificate.
//
// In our architecture the control plane runs on Cloudflare Workers, which
// terminates TLS with a publicly trusted certificate (not our self-signed CA).
// This package watches for the server-ca.crt file to be created and replaces
// it with the system CA bundle so that kubelet and remotedialer can verify
// Cloudflare's TLS certificate.
package cacert

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"
)

// systemCABundlePaths lists well-known paths for the system CA bundle on Linux.
var systemCABundlePaths = []string{
	"/etc/pki/tls/certs/ca-bundle.crt",                  // Amazon Linux, RHEL, Fedora
	"/etc/ssl/certs/ca-certificates.crt",                 // Debian, Ubuntu
	"/etc/ssl/ca-bundle.pem",                             // openSUSE
	"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem",  // RHEL 7+
}

// ReplaceServerCA continuously monitors the k3s server-ca.crt file and
// replaces it with the system CA bundle whenever k3s overwrites it.
//
// k3s's cert-monitor periodically re-downloads the CA from the server and
// overwrites server-ca.crt. A one-shot replacement is insufficient because
// the file gets reverted to the self-signed CA. This function uses fast
// polling (10ms) for initial replacement, then slower monitoring (1s).
func ReplaceServerCA(ctx context.Context, dataDir string) {
	caCertPath := filepath.Join(dataDir, "agent", "server-ca.crt")

	systemCA := findSystemCABundle()
	if systemCA == "" {
		log.Println("cacert: no system CA bundle found, skipping server-ca.crt replacement")
		return
	}

	systemCAData, err := os.ReadFile(systemCA)
	if err != nil {
		log.Printf("cacert: failed to read system CA bundle %s: %v", systemCA, err)
		return
	}
	systemCASize := int64(len(systemCAData))

	// Fast poll until initial replacement
	fastTicker := time.NewTicker(10 * time.Millisecond)
	defer fastTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-fastTicker.C:
			if _, err := os.Stat(caCertPath); err != nil {
				continue
			}
			if err := os.WriteFile(caCertPath, systemCAData, 0600); err != nil {
				log.Printf("cacert: failed to replace server-ca.crt: %v", err)
				continue
			}
			log.Printf("cacert: replaced %s with system CA bundle (%d bytes)", caCertPath, systemCASize)
			goto monitor
		}
	}

monitor:
	// Continue monitoring: k3s cert-monitor re-downloads CA periodically.
	// Re-replace whenever the file size changes from the system CA bundle.
	fastTicker.Stop()
	slowTicker := time.NewTicker(time.Second)
	defer slowTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-slowTicker.C:
			info, err := os.Stat(caCertPath)
			if err != nil {
				continue
			}
			if info.Size() == systemCASize {
				continue // still our replacement
			}
			// k3s overwrote it; replace again
			if err := os.WriteFile(caCertPath, systemCAData, 0600); err != nil {
				log.Printf("cacert: failed to re-replace server-ca.crt: %v", err)
				continue
			}
			log.Printf("cacert: re-replaced %s with system CA bundle", caCertPath)
		}
	}
}

func findSystemCABundle() string {
	for _, p := range systemCABundlePaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
