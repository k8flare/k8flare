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

## Gates 3-5 — pending

- vpc_networks Mesh on-ramp from the gateway (latency vs containerFetch)
- PoP-relay throughput for pod traffic
- TLS posture unchanged either way (Workers fetch has no custom-CA
  support): the :10256 in-VM shim + TokenReview webhook auth stays.

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

**Mesh specifically (the Zero Trust Networks > Mesh L3 CIDR resource,
distinct from a Tunnel) still has no documented API/CLI creation
path** (official docs read 2026-07-07,
`developers.cloudflare.com/cloudflare-one/networks/connectors/
cloudflare-mesh/get-started/`): both the network itself and each
node's `warp-cli connector new <TOKEN>` token are dashboard-only
("Networking > Mesh > Add a node"). `warp-cli` is the client that
*consumes* that token, not something that creates the resource --
matching this entry's own gate 1 finding, just restated precisely.
This means gate 2's blocker (no unattended per-VM enrollment) is
unchanged; what's now confirmed is that a human doing the one-time
dashboard step and handing over the resulting connector token remains
the only path, for either the narrow (Tunnel replacement) or the full
Mesh-as-standard-network scope.
