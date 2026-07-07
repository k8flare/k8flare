//go:build !js

// Package meshconnector runs this node as a Cloudflare Mesh node
// (spikes/s17-mesh-nodevm/FINDINGS.md gate 2, closed 2026-07-07): joins
// the account-wide Mesh network via the real `warp-cli` client and
// returns the Mesh IP this node was assigned, so cmd/agent can advertise
// it as the Node's ExternalIP -- the same flow Phase 9 already wired for
// wireguard-native (`--node-external-ip`), just fed a Mesh IP instead of
// a manually-configured one.
//
// Prerequisite: the `cloudflare-warp` package must already be installed
// on the node image (this package does not install it -- matching how
// cmd/agent doesn't install CNI plugins/containerd either, that's the
// node image's job). See README's node setup section.
package meshconnector

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Run registers this node as a Mesh connector using the given connector
// token (minted via the Cloudflare API -- POST
// /accounts/{id}/warp_connector then GET .../token, no dashboard
// interaction required per gate 2's finding) and blocks until a Mesh IP
// is assigned, returning it. Idempotent: a node already registered
// (`connector new` on top of an existing registration) just re-connects.
func Run(ctx context.Context, token string) (string, error) {
	if out, err := run(ctx, "warp-cli", "--accept-tos", "connector", "new", token); err != nil {
		return "", fmt.Errorf("warp-cli connector new: %w (%s)", err, out)
	}
	if out, err := run(ctx, "warp-cli", "--accept-tos", "connect"); err != nil {
		return "", fmt.Errorf("warp-cli connect: %w (%s)", err, out)
	}
	return waitForMeshIP(ctx)
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// waitForMeshIP polls the CloudflareWARP interface (the client's fixed
// interface name) until it has a routable address -- warp-svc brings
// the interface up asynchronously after `connect` returns.
func waitForMeshIP(ctx context.Context) (string, error) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if ip, err := meshInterfaceIP(); err == nil && ip != "" {
			return ip, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("no Mesh IP assigned to CloudflareWARP within 30s")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

var inetLineRE = regexp.MustCompile(`inet (\d+\.\d+\.\d+\.\d+)/\d+`)

// meshInterfaceIP shells out to `ip addr show CloudflareWARP` -- there
// is no Go netlink dependency elsewhere in this project, and this
// interface name is a client-fixed constant, not something we control,
// so parsing its one line of output is simpler than adding one.
func meshInterfaceIP() (string, error) {
	out, err := run(context.Background(), "ip", "addr", "show", "CloudflareWARP")
	if err != nil {
		return "", err
	}
	m := inetLineRE.FindStringSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("no inet address found in: %s", strings.TrimSpace(out))
	}
	if net.ParseIP(m[1]) == nil {
		return "", fmt.Errorf("unparseable address %q", m[1])
	}
	return m[1], nil
}
