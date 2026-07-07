# S17: WARP/Mesh-native NodeVM — findings

Goal (user proposal 2026-07-06): embed the Cloudflare One (WARP) client
in the node image as the STANDARD network — per-Pod microVM gets a Mesh
IP (100.96.0.0/12) = routable Pod IP; gateway reaches kubelet via
Workers VPC (`vpc_networks`, Mesh on-ramp); ClusterIP CIDR advertised
into Mesh. NOT a reversal of the Phase 9 "Mesh as flannel underlay"
rejection: hostNetwork per-Pod nodes removed the CNI/L2 requirement that
decision hinged on (docs/cloudflare-mesh-networking.md gets an addendum
when this spike concludes).

## Gate 1 — client runs headless in a glibc container: PASS (2026-07-06)

- pkg.cloudflareclient.com ships **Ubuntu/Debian/RHEL only. No
  Alpine/musl build, no static binary** (checked 2026-07-06). → the
  node image must move from `alpine:3.21` to `debian:bookworm-slim` (or
  ship a glibc chroot) if Mesh becomes standard.
- Probe (Dockerfile + probe.sh here): `cloudflare-warp` installs on
  bookworm-slim/amd64, `warp-svc` starts headless in a container with
  `--cap-add NET_ADMIN --device /dev/net/tun`, `warp-cli` talks to it.
  D-Bus absence is a warning only.
- **The client is Rust, not Go** (crate paths `warp::warp_service`,
  `actor_ipc::*` in its own logs) and closed-source → it cannot be
  linked into the Go agent as one binary. The "one artifact" shape is:
  bake the .deb into the node image and have k8flare-agent (PID 1)
  spawn/supervise `warp-svc` exactly like it already supervises the k3s
  embed. A Go-native alternative (wireguard-go joining Mesh directly)
  is not viable: Mesh's enrollment/control plane is proprietary; only
  the official client is a documented path.
- Note: `warp-cli registration new` with NO Zero Trust org registered a
  free CONSUMER WARP device from inside the container (proves the
  enrollment machinery works headless), but **Mesh membership (mesh IP,
  CIDR advertising) requires Zero Trust org enrollment** — gate 2.

## Gate 2 — automated Zero Trust enrollment per ephemeral VM: BLOCKED on credentials

- The wrangler OAuth token has no Zero Trust scopes (devices/orgs APIs
  return auth errors). Needs from the operator: a Zero Trust team name
  and an Access service token (client id/secret) authorized by a device
  enrollment policy, plus an API token with Zero Trust Devices scopes
  for teardown-time device deletion.

## Gate 2 — CLOSED (2026-07-07): unattended API enrollment verified end-to-end on real infrastructure

Full loop executed for real, against the KOOFFICE Cloudflare account
and a real OrbStack Ubuntu 24.04 arm64 VM (`k8flare-agent`) -- the same
node this project's local dev already uses, no new infrastructure
created:

1. `POST /accounts/{id}/warp_connector {"name": "k8flare-mesh-node1"}`
   -- created mesh node `9a4f1c4a-2091-478a-8bc5-67d5549a4210`, zero
   dashboard interaction.
2. `GET /accounts/{id}/warp_connector/{id}/token` -- fetched the
   connector token via API.
3. `apt install cloudflare-warp` (arm64 package exists on
   `pkg.cloudflareclient.com`'s `noble` suite -- gate 1's finding was
   from an amd64 Docker probe and didn't confirm arm64; now confirmed
   too).
4. `warp-cli --accept-tos connector new <TOKEN>` (the `--accept-tos`
   flag is required non-interactively; bare `connector new` demands a
   TTY otherwise) then `warp-cli --accept-tos connect`.
5. Verified: `warp-cli status` reports "Connected" / "Network: healthy";
   `ip addr show` confirms a real Mesh IP,
   `100.96.0.1/32` on a `CloudflareWARP` interface (the documented
   100.96.0.0/12 Mesh CIDR, exactly as this doc's goal section
   predicted).

**Gate 2's original blocker (no unattended per-VM enrollment) is
resolved**, both the method (API-only, no dashboard) and the
mechanism (arm64-compatible, works on this project's existing dev VM
shape). The node is left connected for follow-on gates 3-5.

## Gates 3-5 — pending

- vpc_networks Mesh on-ramp from the gateway (latency vs containerFetch)
- PoP-relay throughput for pod traffic
- TLS posture unchanged either way (Workers fetch has no custom-CA
  support): the :10256 in-VM shim + TokenReview webhook auth stays.

### Shipped instead (2026-07-07): the narrower BYO-VM kubelet-proxy replacement, not this spike's full scope

A *different, smaller* piece of what this document envisions landed as
a real feature: the `MESH` `vpc_networks` binding (`network_id:
"cf1:network"`) now exists in `workers/k8flare/wrangler.jsonc`, and
`workers/k8flare/src/gateway/proxy/target.ts` reaches a **BYO VM**
node's kubelet through it (replacing Cloudflare Tunnel + VPC Service
for `kubectl logs`/`exec`) -- see `pkg/meshconnector`,
`cmd/agent`'s `--mesh-connector-token` flag, and
`docs/cloudflare-mesh-networking.md`'s 2026-07-07 addendum. This is
**not** this spike's "per-Pod microVM gets a Mesh IP" goal (that's
Containers-backed NodeVMs joining Mesh individually, a much larger
change to `workers/nodes`) -- it's the same connector mechanism
(gate 1/2), applied to the one node this project already runs
(`cmd/agent`'s BYO VM), for a narrower purpose (kubelet reachability,
not pod-level routing). Gates 3-5's actual questions (latency,
throughput, per-Pod Mesh membership) remain open for the full
NodeVM-Mesh vision below.

## Status: DEFERRED (2026-07-06, user decision)

Parked after gate 1 (pass) and gate 2 method confirmation, in favor of
k8s-primitive work toward a first public release. Resume point: run the
one-time Mesh setup wizard, mint a Tunnel/Mesh-scoped API token, then
gates 3-5 on a single NodeVM. The 50-nodes/account cap is the standing
design constraint to re-check (Mesh Docker image availability may have
changed the calculus by then).

### Correction (2026-07-07): gate 2's "no Zero Trust scopes" finding was too broad

Re-checked while scoping a *narrower* ask (replacing the kubectl
logs/exec Tunnel+VPC Service setup with Mesh, not the full Mesh-as-
standard-NodeVM-network vision above): the wrangler OAuth token DOES
carry a `connectivity (admin)` scope, and `wrangler tunnel list` /
`wrangler tunnel create` / `wrangler tunnel delete` work against the
real account today (confirmed by deleting a stale tunnel this session).
Gate 2's actual finding stands for what it tested — Zero Trust
**Devices/orgs** enrollment APIs (needed to automate per-ephemeral-VM
Mesh membership without a human) returned auth errors — but plain
**Tunnel** management is not blocked the way this entry implied.

**Correction 2: Mesh DOES have a documented API creation path after
all — the get-started guide's dashboard steps are simplified onboarding
UX, not the only path.** Mesh is the rebrand of WARP Connector, and
WARP Connector tunnels are a first-class, documented Cloudflare API
resource:

- `POST /accounts/{account_id}/warp_connector` `{"name": "..."}` →
  creates the mesh node, returns its tunnel `id`.
- `GET /accounts/{account_id}/warp_connector/{id}/token` → returns the
  connector token (`result` is the token string) that
  `warp-cli connector new <TOKEN>` needs.
- `GET /accounts/{account_id}/warp_connector` lists existing ones.

(Cloudflare API reference, read 2026-07-07:
`developers.cloudflare.com/api/resources/zero_trust/subresources/
tunnels/subresources/warp_connector/methods/{create,list}/` and
`.../subresources/token/methods/get/`.) This means **unattended
per-VM Mesh enrollment IS possible via the standard Cloudflare API** —
gate 2's blocker may be resolved, contingent on one still-unverified
fact: whether an API token can be scoped with the right permission
group for this endpoint (the "Cloudflare Tunnel" permission group is
the likely candidate, unconfirmed), and whether the currently
available `connectivity (admin)` OAuth scope on the wrangler-issued
token (confirmed sufficient for regular `cfd_tunnel` management via
`wrangler tunnel`) extends to `warp_connector` too. Not tested end-to-
end this session: doing so means either extracting wrangler's stored
OAuth token for use outside wrangler's own CLI surface (not attempted,
feels like the wrong way to use that credential) or the account owner
minting a purpose-scoped Cloudflare API token for this. Whoever
resumes this: mint a token with the Tunnel/Zero Trust Networks
permission group, `curl -X POST .../warp_connector` with it, and this
gate closes definitively either way.
