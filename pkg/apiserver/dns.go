package apiserver

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"

	"github.com/miekg/dns"
)

// NodeLocalDNSIP is the address kubelet's --cluster-dns points at
// (advertised via supervisor.go's clusterConfig.ClusterDNS) and the
// address pkg/agent (cmd/agent) binds on every node. The NodeLocal
// DNSCache convention address (link-local, so identical on every node
// with no cross-host collision) -- not a shared Go constant with
// cmd/agent since that would pull this whole package's dependency tree
// into the agent binary for one literal; duplicated deliberately, see
// pkg/agent's matching comment.
const NodeLocalDNSIP = "169.254.20.10"

// Cluster DNS, without a CoreDNS Deployment or any Containers dependency
// (user decision 2026-07-07): this endpoint is the synthesis half of a
// DNS-over-HTTPS (RFC 8484) resolver. The other half is
// cmd/agent's pkg/agent, a node-local UDP/TCP listener kubelet's
// --cluster-dns points at (the NodeLocal DNSCache convention address,
// 169.254.20.10 -- link-local, so identical on every node with no
// collision) that forwards cluster.local queries here over DoH and
// everything else to the node's own upstream resolvers. Request-driven,
// so idle cost is zero -- no resident DNS process, unlike a CoreDNS Pod.
//
// Deviation recorded per rule #3 ("embed the real thing first"): CoreDNS
// itself was considered (its "kubernetes" plugin needs nothing but a
// real Kubernetes API, which this apiserver is). Not embedded because
// CoreDNS's build is plugin.cfg-driven code generation, not a plain
// `go build` of a versioned command the way cmd/scheduler and
// cmd/controller-manager embed real k8s.io/kubernetes binaries -- a
// meaningfully different integration shape. This handler covers Service
// A-record resolution (ClusterIP, and headless-Service backing Pod IPs
// via EndpointSlice); Pod hostname/subdomain records and SRV records are
// not yet implemented.
//
// Only cluster.local A/AAAA queries ever reach this handler -- the shim
// only forwards those. Auth is the same Bearer/Basic token every other
// apiserver endpoint uses (AuthMiddleware), which the shim, not an
// end-user's resolver, presents.

// dnsStores is the minimal read surface this handler needs, resolved once
// at registration time from the corev1 and discoveryv1 store maps
// (main.go's storesByGV).
type dnsStores struct {
	services       *ResourceStore
	endpointSlices *ResourceStore
}

// RegisterDNSHandlers registers the DoH endpoint. clusterDomain must match
// what's advertised via /v1-k3s/config's ClusterDomain (supervisor.go).
func RegisterDNSHandlers(
	mux *http.ServeMux,
	coreStores map[string]*ResourceStore,
	discoveryStores map[string]*ResourceStore,
	tokensFn TokensFunc,
	clusterDomain string,
) {
	ds := &dnsStores{
		services:       coreStores["services"],
		endpointSlices: discoveryStores["endpointslices"],
	}
	suffix := "." + clusterDomain + "."
	mux.Handle("POST /dns-query", AuthMiddleware(tokensFn, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleDoH(w, r, ds, suffix)
	})))
}

func handleDoH(w http.ResponseWriter, r *http.Request, ds *dnsStores, clusterDomainSuffix string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var query dns.Msg
	if err := query.Unpack(body); err != nil {
		http.Error(w, "malformed DNS message", http.StatusBadRequest)
		return
	}

	resp := new(dns.Msg)
	resp.SetReply(&query)
	resp.Authoritative = true

	if len(query.Question) == 1 {
		answerQuestion(r.Context(), ds, query.Question[0], clusterDomainSuffix, resp)
	} else {
		resp.Rcode = dns.RcodeFormatError
	}

	packed, err := resp.Pack()
	if err != nil {
		http.Error(w, "pack response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/dns-message")
	w.Write(packed)
}

// answerQuestion resolves exactly the Service-record shape this project
// supports today: <service>.<namespace>.svc.<clusterDomain>. Anything
// else -- Pod hostname/subdomain records, SRV, PTR -- is NXDOMAIN, same
// as if the record genuinely doesn't exist (the client's resolver moves
// on to the next search-list entry or gives up, standard DNS behavior).
func answerQuestion(ctx context.Context, ds *dnsStores, q dns.Question, clusterDomainSuffix string, resp *dns.Msg) {
	if q.Qtype != dns.TypeA && q.Qtype != dns.TypeAAAA {
		resp.Rcode = dns.RcodeNameError
		return
	}
	name := strings.TrimSuffix(q.Name, clusterDomainSuffix)
	if name == q.Name {
		resp.Rcode = dns.RcodeNameError // didn't even end in our cluster domain
		return
	}
	labels := strings.Split(name, ".")
	if len(labels) != 3 || labels[2] != "svc" {
		resp.Rcode = dns.RcodeNameError
		return
	}
	svcName, namespace := labels[0], labels[1]

	if ds.services == nil {
		resp.Rcode = dns.RcodeServerFailure
		return
	}
	obj, err := ds.services.Get(ctx, namespace, svcName)
	if err != nil {
		resp.Rcode = dns.RcodeNameError
		return
	}
	svc, ok := obj.(*corev1.Service)
	if !ok {
		resp.Rcode = dns.RcodeServerFailure
		return
	}

	var ips []net.IP
	if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != corev1.ClusterIPNone {
		if ip := net.ParseIP(svc.Spec.ClusterIP); ip != nil {
			ips = append(ips, ip)
		}
	} else if svc.Spec.ClusterIP == corev1.ClusterIPNone {
		// Headless Service: resolve to the backing Pods' own IPs via
		// EndpointSlice, the standard headless-Service DNS shape.
		ips = headlessEndpointIPs(ctx, ds, namespace, svcName)
	}
	if len(ips) == 0 {
		resp.Rcode = dns.RcodeNameError
		return
	}

	for _, ip := range ips {
		v4 := ip.To4()
		if q.Qtype == dns.TypeA && v4 != nil {
			resp.Answer = append(resp.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 5},
				A:   v4,
			})
		} else if q.Qtype == dns.TypeAAAA && v4 == nil {
			resp.Answer = append(resp.Answer, &dns.AAAA{
				Hdr:  dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 5},
				AAAA: ip,
			})
		}
	}
	// A query for the family we have no addresses in (e.g. AAAA against
	// an IPv4-only cluster) is a legitimate empty-answer NOERROR, not
	// NXDOMAIN -- the name exists, this record type just doesn't.
}

// headlessEndpointIPs lists this Service's EndpointSlices (labeled
// kubernetes.io/service-name, the standard EndpointSlice controller
// convention -- the real endpointslice controller sets this) and returns
// every ready address.
func headlessEndpointIPs(ctx context.Context, ds *dnsStores, namespace, svcName string) []net.IP {
	if ds.endpointSlices == nil {
		return nil
	}
	obj, err := ds.endpointSlices.List(ctx, namespace, "", "kubernetes.io/service-name="+svcName)
	if err != nil {
		return nil
	}
	list, ok := obj.(*discoveryv1.EndpointSliceList)
	if !ok {
		return nil
	}
	var ips []net.IP
	for _, slice := range list.Items {
		for _, ep := range slice.Endpoints {
			if ep.Conditions.Ready != nil && !*ep.Conditions.Ready {
				continue
			}
			for _, addr := range ep.Addresses {
				if ip := net.ParseIP(addr); ip != nil {
					ips = append(ips, ip)
				}
			}
		}
	}
	return ips
}
