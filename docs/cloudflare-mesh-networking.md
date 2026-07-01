# Cloudflare Mesh as a node-networking backend

Research note, July 2026. Evaluates whether Cloudflare Mesh can replace
today's requirement that all k3s nodes share one VPC (so flannel's
`host-gw` backend can add routes assuming direct IP routability between
node IPs) — the premise behind the "Cloudflare Mesh" roadmap item.

## What it is

**Cloudflare Mesh** (GA-ish; announced April 2026 at Agents Week, ~2.5
months old at time of writing) is the renamed, expanded WARP Connector —
existing WARP Connectors are now called mesh nodes. 50 nodes + 50 users
free on any Cloudflare account; no published pricing found for traffic
volume beyond that (see "Open questions").

Docs: [Overview](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/) ·
[Routes](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/routes/) ·
[Tips](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/tips/) ·
[HA](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/high-availability/)

## The connectivity model: true L3, not a service proxy

Unlike Cloudflare Tunnel (L7, proxies specific hostnames/ports, source IP
lost), Mesh gives every participant a private IP from `100.96.0.0/12` and
routes arbitrary TCP/UDP/ICMP between participants with source IP
preserved. This is the right product category for CNI-level networking —
Tunnel is not.

**The one architectural fact that drives everything below: there is no
direct peer-to-peer path.** Every packet transits a Cloudflare PoP, even
between two nodes that could reach each other directly. This is deliberate
(every connection can go through Gateway/Access policy) but it is not a
transparent VPN mesh in the Tailscale/WireGuard-direct sense.

## Can it replace "same VPC" for flannel host-gw? Technically yes

Mesh supports **CIDR route advertisement**: a node can advertise a subnet
behind it, and Mesh forwards traffic for that CIDR to the node, which
delivers it locally — the same gateway role a flannel host-gw node already
plays. The AWS/GCP/Azure setup prerequisites documented for this
(disable EC2 source/dest checking, enable GCP IP forwarding, add a route
table entry for `100.96.0.0/12`, Azure UDR) are exactly the class of
prerequisite host-gw itself needs today for direct routability — Mesh
doesn't remove that requirement, it relocates it from "your cloud's VPC
config" to "Mesh's route config."

Mechanism this implies for k3s: point flannel's node `public-ip` at each
node's Mesh IP instead of its cloud-private IP, and advertise each node's
PodCIDR as a Mesh route. **No documentation confirms this specific
Kubernetes/flannel combination** — it's a well-supported inference from
Mesh's documented mechanics plus the very similar Tailscale-subnet-router
pattern already common in cross-cloud k8s deployments, not a confirmed,
tested fact.

Enrollment is fully scriptable (matters for unattended EC2 user-data): a
pre-generated dashboard token, `apt install cloudflare-warp`, sysctl
forwarding flags, `warp-cli connector new <token> && warp-cli connect` — no
interactive Access login required.

## Why this isn't a clean drop-in

- **No direct path, ever** — every packet detours through a Cloudflare PoP.
  Third-party throughput testing (NetBird's own comparison, and an
  independent blog drawing partly on the same data — treat exact numbers as
  directional, not certified) shows Mesh at roughly **3–5x lower throughput
  than true P2P mesh tools on same-region/same-continent routes**
  (~250–290 Mbps vs. ~1,220–1,300 Mbps DC-to-DC same country). Mesh only
  wins on intercontinental routes where Cloudflare's backbone beats the
  public internet.
- **UDP loss**: ~14% at sustained 300 Mbps load in that same third-party
  data. This is architecturally consistent with a mandatory-relay design
  even if the exact percentage isn't Cloudflare-certified. This is the
  single most important number for a Kubernetes use case — CoreDNS and any
  latency-sensitive pod traffic runs over UDP, and double-digit loss at
  load is a real production risk if all pod-to-pod traffic rides Mesh.
- **MTU overhead**: Cloudflare's own docs recommend a 1,280-byte MTU / 1,240
  MSS clamp for Mesh-to-Mesh traffic (double encapsulation). Host-gw's whole
  point is avoiding this kind of overhead; Mesh reintroduces it.
- **No Kubernetes integration exists**: no CNI plugin, no case study, no
  Mesh Docker/container image yet ("planned for later 2026"). Using it this
  way means pioneering the pattern, not following a supported path.

## Where Mesh fits vs. Tunnel vs. Workers VPC — don't conflate these

| Product                              | Layer                      | Right for                                                                                                                                                                                                                                                                                                                 |
| ------------------------------------ | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Cloudflare Tunnel** (already used) | L7, specific hostname/port | apiserver → kubelet log/exec proxy — a "reach one known service" problem, already working, no reason to touch                                                                                                                                                                                                             |
| **Workers VPC** (beta)               | Host:port, from a Worker   | A Worker binding (`vpc_networks`) that `fetch()`/`connect()`s any IP:port reachable via Tunnel/Mesh/CF WAN on-ramps without pre-registering each host. Plausible future simplification for apiserver→kubelet reachability (one binding instead of per-node Tunnel config) — still host:port granularity, not CIDR routing |
| **Cloudflare Mesh**                  | L3, CIDR routing           | The only one of these that's the right category for pod-network/CNI-underlay — with the throughput/UDP-loss caveats above                                                                                                                                                                                                 |

(Note: a "CNI" mentioned alongside Workers VPC's WAN on-ramps is **Cloudflare
Network Interconnect**, an unrelated Enterprise physical-link product — not
Kubernetes CNI. Easy to confuse in this project's vocabulary.)

## Recommendation

Keep Cloudflare Mesh on the roadmap, but reframe it from "transparent VPC
replacement" to **"a viable node-networking option for lower-throughput,
dev/test, or geographically-dispersed multi-cloud clusters, with known
throughput and UDP-loss tradeoffs users should be told about explicitly"** —
not the default recommendation for a latency-sensitive production cluster.
EC2/GCE in one VPC (today's supported path) remains the right default for
performance-sensitive clusters until Mesh has a direct-path mode or a
measured Kubernetes track record. Before building anything: prototype
flannel-over-Mesh in a throwaway two-node cluster and measure actual DNS/UDP
behavior directly, rather than trusting third-party numbers for a
production go/no-go.

## Open questions

- Pricing for high-volume inter-node traffic beyond the free 50-node tier —
  unverified, worth asking Cloudflare directly before committing, since this
  would carry all inter-node cluster traffic.
- Whether flannel host-gw over Mesh IPs actually works end-to-end — no
  Cloudflare documentation or third-party report confirms this specific
  combination; needs a hands-on prototype.
- Whether Mesh's "later 2026" container image changes the calculus for
  running the connector as a sidecar rather than a host-level daemon.
