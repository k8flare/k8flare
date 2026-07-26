//go:build linux

// RunVKubeProxy is the node-side half of task #13's virtual
// kube-proxy. Pod-on-Containers Pods run `hostNetwork: true`
// (pkg/apiserver/computeclass.go) because the microVM sandbox has no
// working netfilter -- so even though the real k3s kube-proxy embedded in
// this same binary is enabled cluster-wide (DisableKubeProxy: false in
// pkg/apiserver/supervisor.go), it has no netfilter to program on this
// backend and cannot route ClusterIP traffic here. This file
// substitutes a userspace forwarder scoped to exactly the cluster's
// Service CIDR, using the same "don't reimplement TCP, use a real
// embeddable stack" call this project already made for kube-scheduler and
// kube-controller-manager (rule #3): gVisor's pkg/tcpip terminates each
// ClusterIP-bound TCP connection a Pod opens, and this file re-issues
// it as a plain outbound HTTP request against the control plane's
// /nodes/vkubeproxy endpoint (packages/k8flare-worker/src/nodes/podproxy.ts),
// which resolves the target Service via EndpointSlice and forwards to the
// backing Pod's own NodeVM.
//
// BYO VM nodes never run this: cmd/agent only passes
// -virtual-kube-proxy-cidr on the Containers node image's entrypoint.sh --
// a real VM's netfilter works, so its embedded kube-proxy needs no help.
//
// v1 scope, matching the approved design: HTTP/1.x request-response only.
// Raw TCP passthrough and a secure/TokenReview'd path are deferred (see
// docs/general-purpose-k8s-plan.md).
//
// Build tag is `linux`, not this package's other files' cross-platform
// `!js` (cacert.go, dnsshim.go, meshconnector.go): those only shell out
// to CLI tools, which still compiles anywhere even though it only runs
// on Linux at cmd/agent's actual deploy target. This file instead links
// gvisor.dev/gvisor/pkg/tcpip/link/{tun,fdbased}, whose own build
// constraints are unconditionally Linux-only (raw AF_PACKET sockets, TUN
// ioctls) -- `go vet ./pkg/...` from a non-Linux dev machine fails to even
// compile this file otherwise, not just to run it.
package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/fdbased"
	"gvisor.dev/gvisor/pkg/tcpip/link/tun"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"
)

// tunDeviceName is arbitrary (this process owns the whole microVM's
// network namespace under hostNetwork, so there is no name collision to
// worry about). tunMTU matches the flannel host-gw default this project
// already uses elsewhere for the Pod network; ClusterIP traffic never
// leaves this node so path MTU discovery isn't a concern.
const (
	tunDeviceName             = "k8flare0"
	tunMTU                    = 1500
	nicID         tcpip.NICID = 1
)

// Run opens a TUN device, routes serviceCIDR (e.g. "10.43.0.0/16")
// through it, and forwards every TCP connection any process in this
// network namespace opens to an address in that range by terminating it
// locally and re-issuing it as an HTTP request against
// serverURL+"/nodes/vkubeproxy". Errors setting up the TUN device are
// logged, not fatal -- same posture as RunDNSShim: a cluster still
// works without ClusterIP routing on this node, just as it did before
// this package existed. Blocks until ctx is cancelled.
func RunVKubeProxy(ctx context.Context, serverURL, token, serviceCIDR string) {
	fd, err := tun.Open(tunDeviceName)
	if err != nil {
		log.Printf("vkubeproxy: failed to open TUN device (ClusterIP routing will not be available): %v", err)
		return
	}

	linkEP, err := fdbased.New(&fdbased.Options{FDs: []int{fd}, MTU: tunMTU})
	if err != nil {
		unix.Close(fd)
		log.Printf("vkubeproxy: failed to create link endpoint: %v", err)
		return
	}

	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol},
	})
	if tcpErr := s.CreateNIC(nicID, linkEP); tcpErr != nil {
		log.Printf("vkubeproxy: failed to create NIC: %v", tcpErr)
		return
	}
	// This NIC is never assigned an address of its own -- every ClusterIP
	// is, as far as it's concerned, a foreign address it must still
	// accept inbound packets for and originate replies as (the standard
	// tun2socks-style transparent router shape).
	s.SetPromiscuousMode(nicID, true)
	s.SetSpoofing(nicID, true)

	if _, err := subnetFromCIDR(serviceCIDR); err != nil {
		log.Printf("vkubeproxy: invalid service CIDR %q: %v", serviceCIDR, err)
		return
	}
	// A default (0.0.0.0/0) route, not one scoped to serviceCIDR: this
	// governs only which NIC gvisor's *own* userspace stack uses to
	// originate a packet, and there is exactly one NIC in this whole
	// stack (the TUN device) -- every reply this stack ever sends
	// (SYN-ACK, data, FIN) is addressed to the real client's own address
	// (e.g. the microVM's real interface IP), which is never itself
	// inside serviceCIDR, so a subnet-scoped route here made every
	// CreateEndpoint fail with "network is unreachable" (caught only by
	// actually running this against a real TUN device in a Linux
	// container -- see docs/platform-verification.md). The Linux kernel
	// route configured below is what actually scopes interception to
	// serviceCIDR; this stack-internal route does not need to repeat
	// that scoping.
	s.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: nicID}})

	if err := configureHostRoute(tunDeviceName, serviceCIDR); err != nil {
		log.Printf("vkubeproxy: failed to configure host route (ClusterIP routing will not be available): %v", err)
		return
	}

	fwd := tcp.NewForwarder(s, 0, 1024, func(r *tcp.ForwarderRequest) {
		handleConn(r, serverURL, token)
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, fwd.HandlePacket)

	log.Printf("vkubeproxy: forwarding %s via %s -> %s/nodes/vkubeproxy", serviceCIDR, tunDeviceName, serverURL)
	<-ctx.Done()
}

func subnetFromCIDR(cidr string) (tcpip.Subnet, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return tcpip.Subnet{}, err
	}
	v4 := ipNet.IP.To4()
	if v4 == nil {
		return tcpip.Subnet{}, fmt.Errorf("only IPv4 is supported, got %s", cidr)
	}
	addr := tcpip.AddrFromSlice(v4)
	mask := tcpip.MaskFromBytes(ipNet.Mask)
	return tcpip.NewSubnet(addr, mask)
}

// configureHostRoute brings the TUN interface up and routes serviceCIDR
// through it -- a plain `ip route`, not netfilter/DNAT. Routing to a TUN
// device is a different, simpler kernel facility than the netfilter
// hostNetwork's own removal of CNI works around, so this is expected to
// work even where iptables-based NAT doesn't (unverified against a real
// deployment -- see docs/platform-verification.md). Same
// exec.Command("ip", ...) pattern pkg/agent.ensureLinkLocalAddress
// already uses in this same binary.
func configureHostRoute(name, cidr string) error {
	if out, err := exec.Command("ip", "link", "set", "dev", name, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("ip link set up: %w (%s)", err, out)
	}
	if out, err := exec.Command("ip", "route", "add", cidr, "dev", name).CombinedOutput(); err != nil {
		if !strings.Contains(string(out), "File exists") {
			return fmt.Errorf("ip route add: %w (%s)", err, out)
		}
	}
	return nil
}

// handleConn completes the intercepted TCP handshake, then proxies one or
// more HTTP/1.x request-responses over it (a persistent client connection
// with keep-alive works the same way real HTTP/1.1 does: read a request,
// forward it, write back the response, repeat until the client closes or
// sends Connection: close). Runs once per accepted connection -- gVisor's
// Forwarder already calls this from its own dispatch goroutine per
// connection, so no extra goroutine is spawned here.
func handleConn(r *tcp.ForwarderRequest, serverURL, token string) {
	id := r.ID()
	var wq waiter.Queue
	ep, err := r.CreateEndpoint(&wq)
	if err != nil {
		log.Printf("vkubeproxy: reject %s:%d -> %s:%d: %v", id.RemoteAddress, id.RemotePort, id.LocalAddress, id.LocalPort, err)
		r.Complete(true)
		return
	}
	r.Complete(false)
	conn := gonet.NewTCPConn(&wq, ep)
	defer conn.Close()

	targetIP := id.LocalAddress.String()
	targetPort := strconv.Itoa(int(id.LocalPort))
	base := strings.TrimRight(serverURL, "/") + "/nodes/vkubeproxy"

	client := &http.Client{}
	reader := bufio.NewReader(conn)
	for {
		req, err := http.ReadRequest(reader)
		if err != nil {
			if err != io.EOF {
				log.Printf("vkubeproxy: read request for %s:%s: %v", targetIP, targetPort, err)
			}
			return
		}

		outReq, err := http.NewRequest(req.Method, base+req.URL.String(), req.Body)
		if err != nil {
			writeError(conn, err)
			return
		}
		outReq.Header = req.Header.Clone()
		outReq.Header.Set("Authorization", "Bearer "+token)
		outReq.Header.Set("X-K8flare-Target-IP", targetIP)
		outReq.Header.Set("X-K8flare-Target-Port", targetPort)
		outReq.ContentLength = req.ContentLength

		resp, err := client.Do(outReq)
		if err != nil {
			writeError(conn, err)
			return
		}
		resp.Close = req.Close
		writeErr := resp.Write(conn)
		resp.Body.Close()
		if writeErr != nil || req.Close {
			return
		}
	}
}

// writeError writes a synthetic 502 response for a proxy-side failure
// (the control plane's /nodes/vkubeproxy endpoint being unreachable, a
// malformed backend response, ...) directly to the Pod's own connection --
// the app on the other end sees an ordinary (if unwelcome) HTTP response,
// not a hung or reset connection.
func writeError(conn net.Conn, err error) {
	body := err.Error()
	resp := &http.Response{
		StatusCode:    http.StatusBadGateway,
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	resp.Write(conn)
}
