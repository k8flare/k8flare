# Cloudflare Mesh as a node-networking backend — not adopted

Original research note, July 2026 (kept below, with corrections layered on
top per the honest-correction rule rather than rewritten in place). Updated
twice since:

- `spikes/s4-mesh/RESEARCH.md` (2026-07-02): desk research pass that found
  flannel `host-gw` cannot work over Mesh at all, and reframed the question
  around k3s's own `wireguard-native` backend.
- `spikes/p9-mesh/RESEARCH.md` (2026-07-03, Phase 9): live verification
  pass. **Decision: Cloudflare Mesh is not adopted.** See "Final decision"
  below before reading the rest of this document, which is largely
  superseded context.

## Final decision (Phase 9, 2026-07-03)

**Cloudflare Mesh is not adopted as a node-networking option.** Two
independent findings drove this, both reached by actually running things,
not by reading docs alone:

1. **Mesh's only live value hypothesis over k3s's own `wireguard-native`
   backend — NAT traversal, letting a BYO VM join without opening an inbound
   port — could not be verified.** Enabling/inspecting Mesh requires the
   Cloudflare dashboard (Zero Trust → Networks → Mesh) or a Zero-Trust-scoped
   API token; neither was available to this task (see
   `spikes/p9-mesh/RESEARCH.md` §1 for exactly what was and wasn't checked
   via `wrangler whoami`). No Mesh network could be created to prototype
   against.
2. **k3s's official Mesh-free alternative was live-tested and its
   control-plane integration works correctly.** Two real `cmd/agent`
   instances, on genuinely separate Docker networks joined only through a
   shared "public" network (standing in for two clouds), were driven to
   `--flannel-backend=wireguard-native` + `--node-external-ip` entirely
   through this project's own supervisor-config path (no Mesh, no VPC
   peering). The backend selection and the internal-vs-external-IP split
   both worked exactly as k3s's own multicloud docs describe. Full pod-to-pod
   route convergence didn't complete in the test window, but an A/B control
   with the currently-shipped `host-gw` default reproduced the **identical**
   symptom — proving that gap is a pre-existing, backend-agnostic issue in
   this project's control plane (already on record: see
   `docs/general-purpose-k8s-plan.md` Phase 1's open follow-ups and
   `README.md`'s own "Service networking" gap row), not something Mesh would
   have avoided.

### Addendum (2026-07-07): adopted for the kubelet-proxy path specifically; the pod-network question above is unchanged

**Scope check first:** the Final Decision above is about Mesh as a
**pod-network/CNI underlay** (a flannel replacement) -- that question
is untouched by this addendum. What changed is a narrower, different
use this document's own "Where Mesh fits" table already scoped
separately: **the kubelet-proxy path for `kubectl logs`/`kubectl
exec`**, previously served by Cloudflare Tunnel + Workers VPC Service
(`scripts/setup-tunnel.sh`). User decision 2026-07-07: replace that
path with Mesh (`spikes/s17-mesh-nodevm/FINDINGS.md` gates 1-2, closed
that day) -- README's setup section and
`workers/k8flare/src/gateway/proxy/target.ts` reflect this (Mesh
preferred, Tunnel+VPC Service kept as a fallback for existing
deployments).

**Point 1 above is now factually superseded, recorded not deleted:**
"neither [dashboard nor API token] was available... no Mesh network
could be created" was true of the credentials on hand in Phase 9. On
2026-07-07, with a properly-scoped `CLOUDFLARE_API_TOKEN`, a real Mesh
node was created and connected purely via the Cloudflare REST API --
`POST /accounts/{id}/warp_connector`, `GET .../token`, then `warp-cli
connector new`/`connect` on the target host -- no Zero Trust dashboard
step at any point. Mesh IP assignment (`100.96.0.1/32`) and healthy
status were verified on a real node. Whether this changes the
CNI-underlay calculus in the Final Decision above (its point 1's NAT-
traversal hypothesis was never actually tested, only blocked on
access) is an open question for whoever revisits pod-networking --
not answered by this addendum.

**Recommendation**: nodes that don't share a VPC/subnet should use k3s's
built-in `--flannel-backend=wireguard-native` plus `--node-external-ip` (now
wired end-to-end: `cmd/agent`'s `--node-external-ip` flag, plus a
`FlannelExternalIP` field on the control plane's cluster config, added in
Phase 9 — see `pkg/apiserver/supervisor.go` and `cmd/agent/main.go`). This
needs no new Cloudflare product, no new cost, and no new dependency, unlike
Mesh. Cluster DNS remains CoreDNS, unchanged (decided in S4, not revisited
here).

Full detail, methodology, and citations: `spikes/p9-mesh/RESEARCH.md` and
`spikes/s4-mesh/RESEARCH.md`. The rest of this document is the original
April-note-era evaluation, kept for history.

---

## What it is

**Cloudflare Mesh** (GA'd 2026-04-14, confirmed via official changelog in the
S4 research pass) is the renamed, expanded WARP Connector — existing WARP
Connectors are now called mesh nodes. 50 nodes + 50 users free on any
Cloudflare account (likely via the always-available Zero Trust Free seat
pool, per S4's research — not confirmed to be automatically bundled with
Workers Paid); no published pricing found for traffic volume beyond that
(see "Open questions").

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
transparent VPN mesh in the Tailscale/WireGuard-direct sense. Confirmed by
S4: the default transport is itself UDP (MASQUE/QUIC over TLS 1.3), so this
is a mandatory-relay design, not just a routing detour.

## Can it replace "same VPC" for flannel host-gw? **No — corrected by S4**

The original version of this section reasoned "technically yes" from Mesh's
CIDR route advertisement mechanism looking similar to what a `host-gw` node
already does. **S4's research (2026-07-02) checked flannel's own backend
docs directly and found this was wrong**: `host-gw` requires genuine L2
adjacency between hosts (flannel's own
[backends.md](https://github.com/flannel-io/flannel/blob/master/Documentation/backends.md)
states this explicitly), which Mesh's virtual `100.96.0.0/12` address space
does not provide — there is no shared broadcast domain, only relayed L3
reachability. The same failure mode is already on record for the analogous
Tailscale+host-gw combination
([k3s-io/k3s#8372](https://github.com/k3s-io/k3s/issues/8372)).

`vxlan` only needs L3 UDP reachability, which Mesh does provide — viable in
principle, at the cost of double UDP encapsulation (vxlan inside Mesh's own
MASQUE/QUIC tunnel) and a correspondingly tighter MTU budget. But this
matters less than it might seem, because:

**k3s already ships an official alternative that needs no third-party mesh
product at all**: `--flannel-backend=wireguard-native` plus
`--node-external-ip`
([K3s: Distributed hybrid or multicloud cluster](https://docs.k3s.io/networking/distributed-multicloud)).
This is the option Phase 9 live-verified and recommends — see "Final
decision" above.

Enrollment for Mesh itself is fully scriptable (matters for unattended EC2
user-data): a pre-generated dashboard token, `apt install cloudflare-warp`,
sysctl forwarding flags, `warp-cli connector new <token> && warp-cli
connect` — no interactive Access login required. This remains true but is
moot given the decision above.

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
  data. S4 clarified this is a structural property of Mesh's own transport
  (MASQUE/QUIC is itself UDP), so it affects everything crossing Mesh —
  including TCP payloads — not just UDP-based application traffic.
- **MTU overhead is a real failure mode, not just a tuning recommendation**:
  Cloudflare's own Tips page states packets near 1,460 bytes get pushed over
  1,500 bytes by the double encapsulation and are **silently dropped**.
- **No Kubernetes integration exists**: no CNI plugin, no case study, no
  Mesh Docker/container image yet ("planned for later 2026" as of S4).
  Using it this way would mean pioneering the pattern, not following a
  supported path.
- **Maturity, precisely**: GA since 2026-04-14, but the Linux client Mesh
  server nodes actually depend on only reached its own GA on 2026-06-29 — a
  handful of days before the S4 research pass. "GA" understates how new the
  actual server-node foundation is.

## Where Mesh fits vs. Tunnel vs. Workers VPC — don't conflate these

| Product                              | Layer                      | Right for                                                                                                                                                                                                                                                                                                                 |
| ------------------------------------ | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Cloudflare Tunnel** (already used) | L7, specific hostname/port | apiserver → kubelet log/exec proxy — a "reach one known service" problem, already working, no reason to touch                                                                                                                                                                                                             |
| **Workers VPC** (beta)               | Host:port, from a Worker   | A Worker binding (`vpc_networks`) that `fetch()`/`connect()`s any IP:port reachable via Tunnel/Mesh/CF WAN on-ramps without pre-registering each host. Plausible future simplification for apiserver→kubelet reachability (one binding instead of per-node Tunnel config) — still host:port granularity, not CIDR routing |
| **Cloudflare Mesh**                  | L3, CIDR routing           | The right product category for pod-network/CNI-underlay in principle — not adopted here; see "Final decision"                                                                                                                                                                                                             |

(Note: a "CNI" mentioned alongside Workers VPC's WAN on-ramps is **Cloudflare
Network Interconnect**, an unrelated Enterprise physical-link product — not
Kubernetes CNI. Easy to confuse in this project's vocabulary.)

## Open questions (mostly moot given the decision above, kept for record)

- Pricing for high-volume inter-node traffic beyond the free 50-node tier —
  never resolved; would need Cloudflare directly. Irrelevant now that Mesh
  isn't adopted.
- Whether flannel host-gw over Mesh IPs works end-to-end — resolved by S4:
  no, host-gw needs L2 adjacency Mesh doesn't provide.
- Whether Mesh's "later 2026" container image would change the calculus for
  running the connector as a sidecar rather than a host-level daemon — moot.
