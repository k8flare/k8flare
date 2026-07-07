//go:build !js

// Package dnsshim is the node-local half of cluster DNS (user decision
// 2026-07-07: no Containers/CoreDNS Deployment dependency). It binds the
// NodeLocal DNSCache convention address on this node, forwards
// cluster.local queries to the control plane over DNS-over-HTTPS (the
// synthesis half, pkg/apiserver's dns.go), and forwards everything else
// to the node's own upstream resolvers -- exactly what a real cluster's
// CoreDNS Corefile "forward . /etc/resolv.conf" stanza does, just
// without CoreDNS itself.
//
// kubelet's --cluster-dns is set to this same address via
// supervisor.go's clusterConfig.ClusterDNS, so every ClusterFirst Pod's
// /etc/resolv.conf nameserver line points here with zero k3s/kubelet
// code changes -- the embedded agent already applies whatever
// /v1-k3s/config advertises.
package dnsshim

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// NodeLocalDNSIP must match pkg/apiserver/dns.go's constant of the same
// name (not a shared Go const -- see that file's comment for why).
const NodeLocalDNSIP = "169.254.20.10"

const dohTimeout = 5 * time.Second

// Run binds NodeLocalDNSIP:53 (UDP+TCP) and serves until ctx is
// cancelled. Errors setting up the link-local address or the listeners
// are logged, not fatal -- a cluster still works without cluster DNS
// (README's pre-existing documented gap), just as it did before this
// package existed.
func Run(ctx context.Context, serverURL, token, clusterDomain string) {
	if err := ensureLinkLocalAddress(); err != nil {
		log.Printf("dnsshim: failed to configure %s (cluster DNS will not be available): %v", NodeLocalDNSIP, err)
		return
	}

	upstream, err := upstreamResolvers()
	if err != nil || len(upstream) == 0 {
		log.Printf("dnsshim: failed to read upstream resolvers from /etc/resolv.conf: %v", err)
		return
	}

	h := &handler{
		dohURL:        strings.TrimRight(serverURL, "/") + "/dns-query",
		token:         token,
		clusterSuffix: "." + clusterDomain + ".",
		upstream:      upstream[0] + ":53",
		client:        &http.Client{Timeout: dohTimeout},
	}

	addr := NodeLocalDNSIP + ":53"
	udpServer := &dns.Server{Addr: addr, Net: "udp", Handler: h}
	tcpServer := &dns.Server{Addr: addr, Net: "tcp", Handler: h}

	go func() {
		if err := udpServer.ListenAndServe(); err != nil {
			log.Printf("dnsshim: udp server: %v", err)
		}
	}()
	go func() {
		if err := tcpServer.ListenAndServe(); err != nil {
			log.Printf("dnsshim: tcp server: %v", err)
		}
	}()
	log.Printf("dnsshim: serving cluster DNS on %s (cluster.local -> %s, else -> %s)", addr, h.dohURL, h.upstream)

	<-ctx.Done()
	udpServer.ShutdownContext(context.Background())
	tcpServer.ShutdownContext(context.Background())
}

// ensureLinkLocalAddress assigns NodeLocalDNSIP to loopback so the
// process can bind it (Linux refuses to bind an address not already
// configured on some interface). Idempotent: "RTNETLINK answers: File
// exists" from a prior run is not an error.
func ensureLinkLocalAddress() error {
	cmd := exec.Command("ip", "addr", "add", NodeLocalDNSIP+"/32", "dev", "lo")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "File exists") {
			return nil
		}
		return fmt.Errorf("ip addr add: %w (%s)", err, stderr.String())
	}
	return nil
}

// upstreamResolvers returns this node's own nameservers, read from the
// real /etc/resolv.conf -- kubelet never rewrites the HOST's
// resolv.conf (only each Pod's, to point at NodeLocalDNSIP), so this is
// stable to read at any point in the agent's lifecycle.
func upstreamResolvers() ([]string, error) {
	cfg, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return nil, err
	}
	return cfg.Servers, nil
}

type handler struct {
	dohURL        string
	token         string
	clusterSuffix string
	upstream      string
	client        *http.Client
}

func (h *handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 1 && strings.HasSuffix(r.Question[0].Name, h.clusterSuffix) {
		h.forwardDoH(w, r)
		return
	}
	h.forwardUpstream(w, r)
}

func (h *handler) forwardDoH(w dns.ResponseWriter, r *dns.Msg) {
	packed, err := r.Pack()
	if err != nil {
		dns.HandleFailed(w, r)
		return
	}
	req, err := http.NewRequest(http.MethodPost, h.dohURL, bytes.NewReader(packed))
	if err != nil {
		dns.HandleFailed(w, r)
		return
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Authorization", "Bearer "+h.token)

	resp, err := h.client.Do(req)
	if err != nil {
		log.Printf("dnsshim: DoH request failed: %v", err)
		dns.HandleFailed(w, r)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("dnsshim: DoH request: HTTP %d", resp.StatusCode)
		dns.HandleFailed(w, r)
		return
	}
	var body bytes.Buffer
	if _, err := body.ReadFrom(resp.Body); err != nil {
		dns.HandleFailed(w, r)
		return
	}
	var reply dns.Msg
	if err := reply.Unpack(body.Bytes()); err != nil {
		dns.HandleFailed(w, r)
		return
	}
	w.WriteMsg(&reply)
}

func (h *handler) forwardUpstream(w dns.ResponseWriter, r *dns.Msg) {
	client := &dns.Client{Net: "udp", Timeout: dohTimeout}
	reply, _, err := client.Exchange(r, h.upstream)
	if err != nil {
		dns.HandleFailed(w, r)
		return
	}
	if reply.Truncated {
		tcpClient := &dns.Client{Net: "tcp", Timeout: dohTimeout}
		if tcpReply, _, err := tcpClient.Exchange(r, h.upstream); err == nil {
			reply = tcpReply
		}
	}
	w.WriteMsg(reply)
}
