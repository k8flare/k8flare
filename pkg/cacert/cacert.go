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
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// systemCABundlePaths lists well-known paths for the system CA bundle on Linux.
var systemCABundlePaths = []string{
	"/etc/pki/tls/certs/ca-bundle.crt",                  // Amazon Linux, RHEL, Fedora
	"/etc/ssl/certs/ca-certificates.crt",                // Debian, Ubuntu
	"/etc/ssl/ca-bundle.pem",                            // openSUSE
	"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", // RHEL 7+
}

// ReplaceServerCA continuously monitors the k3s server-ca.crt file and
// replaces it with the system CA bundle whenever k3s overwrites it.
//
// k3s's cert-monitor periodically re-downloads the CA from the server and
// overwrites server-ca.crt, so a one-shot replacement is insufficient.
//
// This races against k3s's own agent bootstrap goroutine, which downloads
// the self-signed CA, writes server-ca.crt, and (in the same call chain)
// immediately builds a long-lived REST client that reads that file exactly
// once (k3s's util.WaitForAPIServerReady) -- if that read happens before we
// replace the file, the wrong CA is captured for the lifetime of the
// process and every subsequent TLS-verified request against the real
// (Cloudflare-terminated) server fails. A fixed-interval poll (previously
// 10ms) is fundamentally racy here: on a real bare-metal CI runner it has
// consistently won (`e2e-conformance.yml` reaches Node Ready reliably), but
// it reproducibly LOSES this race under nested virtualization (a privileged
// Docker container on an OrbStack Linux VM on macOS, used for local
// hands-on verification) -- confirmed by a goroutine dump showing
// `k3s-io/k3s/pkg/executor/embed.(*Embedded).Kubelet.func1()` permanently
// parked on `<-e.APIServerReadyChan()`, which never closes because the
// captured client can never validate the real server's TLS certificate.
// fsnotify reacts to the actual write syscall (typically sub-millisecond
// latency) instead of waiting for the next poll tick, which narrows the
// race window by roughly two orders of magnitude versus the old 10ms
// ticker. It cannot make the race fully deterministic (no local fork of
// vendored k3s to add a real synchronization point), but combined with an
// immediate replace-if-present check right after the watcher is armed, this
// is a substantial, well-understood improvement over blind polling.
func ReplaceServerCA(ctx context.Context, dataDir string) {
	caDir := filepath.Join(dataDir, "agent")
	caCertPath := filepath.Join(caDir, "server-ca.crt")

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

	// replace overwrites caCertPath with systemCAData unless it already
	// holds exactly that content (avoids a redundant write -- and log line
	// -- on every fsnotify event once we've already won).
	replace := func() {
		data, err := os.ReadFile(caCertPath)
		if err != nil {
			return
		}
		if bytes.Equal(data, systemCAData) {
			return
		}
		if err := os.WriteFile(caCertPath, systemCAData, 0600); err != nil {
			log.Printf("cacert: failed to replace server-ca.crt: %v", err)
			return
		}
		log.Printf("cacert: replaced %s with system CA bundle (%d bytes)", caCertPath, systemCASize)
	}

	// Ensure the directory exists so the watcher can be armed before k3s
	// ever creates server-ca.crt inside it -- otherwise we'd need to watch
	// a parent directory for the child directory's own creation first,
	// adding another polling race on top of this one. k3s creates this same
	// directory itself during bootstrap (MkdirAll is idempotent), so this
	// doesn't change k3s's own behavior.
	if err := os.MkdirAll(caDir, 0700); err != nil {
		log.Printf("cacert: failed to create %s: %v", caDir, err)
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("cacert: failed to create fsnotify watcher (%v), falling back to polling", err)
		pollReplace(ctx, caCertPath, replace)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(caDir); err != nil {
		log.Printf("cacert: failed to watch %s (%v), falling back to polling", caDir, err)
		pollReplace(ctx, caCertPath, replace)
		return
	}

	// Covers both a process restart (file already present) and the
	// unavoidable sliver of time between MkdirAll/watcher.Add above and the
	// watcher actually being armed.
	replace()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Clean(event.Name) != caCertPath {
				continue
			}
			if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				replace()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("cacert: watcher error: %v", err)
		}
	}
}

// pollReplace is the fallback path if an fsnotify watcher can't be created
// (e.g. inotify instance limits) -- the original fixed-interval behavior,
// kept only as a safety net rather than the primary mechanism.
func pollReplace(ctx context.Context, caCertPath string, replace func()) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := os.Stat(caCertPath); err != nil {
				continue
			}
			replace()
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
