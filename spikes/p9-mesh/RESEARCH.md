# Phase 9: Cloudflare Mesh adopt/no-adopt — live verification (2026-07-03)

Continues `spikes/s4-mesh/RESEARCH.md` (2026-07-02 desk research). That pass
concluded flannel `host-gw` cannot work over Mesh (no L2 adjacency), that k3s
already ships an official Mesh-free alternative
(`--flannel-backend=wireguard-native` + `--node-external-ip`), and reframed
the real question as "what does Mesh add on top of wireguard-native" — with
NAT traversal (no inbound port needed) as the only live hypothesis. This
phase's job was to actually run things: check whether Mesh is usable on the
project's Cloudflare account, and hands-on verify wireguard-native. No
code/deploy changes to `workers/*` or `pkg/apiserver`'s shipped behavior were
made permanently — see "Changes kept" at the bottom.

## 1. Cloudflare Mesh live check: blocked on dashboard access, not verifiable in this task

Per the task brief, checked whether Zero Trust/Mesh is usable on the
KOOFFICE account (`ed17c5c18eb6052e70234ec181709fba`) before assuming a
prototype was possible.

- `npx wrangler whoami` confirms this session is authenticated against the
  correct account (KOOFFICE, matching the given account ID) with an OAuth
  token whose scopes include `connectivity (admin)` alongside the usual
  Workers/D1/Containers scopes.
- However, Cloudflare Mesh lives entirely under Zero Trust / Cloudflare One
  (`dash.cloudflare.com` → Networks → Mesh), a separate product surface from
  the Workers API that `wrangler` talks to. There is no `wrangler` subcommand
  for it, and confirming/enabling it programmatically would require either
  (a) driving the Cloudflare dashboard directly, or (b) a Zero-Trust-scoped
  API call whose required permission group is distinct from what a
  Workers-focused OAuth token is known to carry — attempting that would mean
  pulling the raw OAuth bearer token out of wrangler's local credential
  store and reusing it outside the tool it's scoped for, which this task
  deliberately avoided rather than guess at a permission boundary.
- **Conclusion: Mesh enablement/usability on this account could not be
  verified within this task's tool access.** Per the task brief's own
  pre-authorized fallback, this is reported honestly rather than assumed
  either way. This alone does not decide adopt/no-adopt (see §4) but it does
  mean the "vxlan-over-Mesh prototype" branch of the task was not attempted:
  there is nothing to prototype against without a Mesh network to join.

## 2. wireguard-native live verification: methodology

Since Mesh itself couldn't be reached, effort went into the part explicitly
achievable without it: does k3s's official `wireguard-native` +
`--node-external-ip` combination actually work when integrated into this
project's specific control plane (Worker-served supervisor config, `cmd/agent`
embed, Node-annotation-based peer coordination)?

**Topology** (Docker Desktop/OrbStack on this machine, arm64):
- Two private "VPC" networks, `net-a` (10.10.1.0/24) and `net-b`
  (10.10.2.0/24), one per node, standing in for two clouds/accounts that
  don't share a subnet.
- A shared `pub-net` (10.10.9.0/24) both nodes also attach to, standing in
  for "the public internet" — the only path between the two nodes.
- Node A: `net-a` IP `10.10.1.11` (its regular/internal kubelet IP) +
  `pub-net` IP `10.10.9.11` (its external IP). Node B symmetric on `net-b`
  (`10.10.2.12`) + `pub-net` (`10.10.9.12`).
- Caveat found and worked around: this Docker runtime (OrbStack, not literal
  Docker Engine) does **not** enforce the inter-bridge-network isolation a
  real Linux Docker Engine host does by default (`net-a`↔`net-b` were
  reachable out of the box; inserting manual `iptables DROP` rules in the
  shared VM netns didn't change that either — traffic isn't traversing the
  `DOCKER-FORWARD` iptables chain the way it does on Engine). This sandbox
  therefore cannot fully prove "no route would exist without the tunnel";
  the value of the test is in confirming the **mechanism** (backend
  selection, internal/external IP separation, per-node config) works
  end-to-end in this project's stack, not in a clean network-isolation
  negative control. Documented honestly rather than presented as more rigorous
  than it is (integrity rule 2/4).

**Control-plane change (temporary, for this test only — see "Changes kept"):**
`pkg/apiserver/supervisor.go`'s `defaultClusterConfig()` was temporarily
edited to serve `FlannelBackend: "wireguard-native"` plus a newly-added
`FlannelExternalIP: true` field (mirrors k3s's own `--flannel-external-ip`
server flag — required per
[k3s's multicloud doc](https://docs.k3s.io/networking/distributed-multicloud)
so flannel uses the node's *external* IP for its tunnel endpoint instead of
its internal one). This is the only way to drive the backend choice in this
project: `FlannelBackend` is a server/control-plane-dictated value in
upstream k3s (`cmds.ServerConfig.FlannelBackend`), pushed to agents via
`/v1-k3s/config` — there is no agent-side override. The `FlannelExternalIP`
struct field is kept (harmless, defaults to `false`/omitted); the backend
value itself was reverted to `"host-gw"` before finishing this phase (see
bottom).

**`cmd/agent` change (kept, minimal):** added a `--node-external-ip` flag
wiring straight into the existing (already-vendored, unused-until-now)
`cmds.Agent.NodeExternalIP` field. Opt-in, empty by default, so it changes
nothing for existing same-VPC deployments. Needed because k3s has no way to
advertise a node's external IP without it, and the upstream multicloud docs
require it on every node regardless of backend.

Built `cmd/agent` for `linux/arm64`, packaged into a Debian-slim image
alongside the real `k3s` binary (used only for `cmd/agent`'s one-time
CNI/containerd/runc asset extraction, exactly like
`e2e-conformance.yml`'s "Install k3s binary" step — never actually run as a
server/agent itself). Ran both containers `--privileged --cgroupns=host` with
`/sys/fs/cgroup` bind-mounted rw (required for nested containerd's cgroup v2
delegation; without it kubelet fails immediately with `cannot enter cgroupv2
"/sys/fs/cgroup/kubepods" with domain controllers -- it is in an invalid
state`). Control plane: a real local 4-config `wrangler dev` (gateway +
storage + apiserver + runtime), HTTPS with a self-signed cert (matching
`e2e-conformance.yml`'s pattern — required because client-go's `clientcmd`
only sends bearer-token auth over a TLS transport), reachable from containers
via `host.docker.internal`.

## 3. Results

**What is confirmed working, end-to-end, for real:**

- Both nodes registered `Ready`, each with a distinct PodCIDR
  (`10.42.0.0/24` / `10.42.1.0/24`) from the existing allocator.
- `Starting flannel with backend wireguard-native` logged on both — the
  server-dictated backend selection correctly reaches the agent through this
  project's supervisor config endpoint, exactly like `host-gw` does today.
- **The internal/external IP split works correctly**: each Node's
  `status.addresses` shows `InternalIP` = the private per-VPC IP
  (`10.10.1.11` / `10.10.2.12` — what kubelet/kube-proxy use), while
  `metadata.annotations["flannel.alpha.coreos.com/public-ip-overwrite"]`
  shows the *external* `pub-net` IP (`10.10.9.11` / `10.10.9.12`) — i.e.
  `--node-external-ip` + the new `FlannelExternalIP` control-plane field
  together produce exactly the separation k3s's own multicloud docs
  describe, with zero manual annotation editing.
- kube-proxy, kubelet, and the CNI-facing `subnet.env` bypass
  (`pkg/cacert/flannel.go`) all work identically to the `host-gw` path —
  nothing about switching backends broke any of the already-working parts.

**What did not complete within the test window (422s / ~7 minutes, polled
every 5s):** neither node ever created its `flannel-wg` WireGuard interface
(`ip link show` / `wg show` both empty throughout), and neither Node object
ever gained the `flannel.alpha.coreos.com/backend-data` / `backend-type`
annotations flannel's own subnet manager writes once its Node informer
(`flannel-io/flannel@v0.28.4`'s `pkg/subnet/kube/kube.go`) finishes its
initial sync (it allows up to **10 minutes** (`nodeControllerSyncTimeout`)
before logging its own explicit failure — this run's 422s bound didn't reach
that, so flannel's own error log never fired either — but 7 minutes of
silence is already well past the "temporarily overloaded during startup
burst" framing in `pkg/cacert/flannel.go`'s own doc comment, which describes
a brief sync-time hiccup, not a multi-minute one).

**Control run (A/B, per this project's own "actually verify, don't
conclude from the first plausible cause" rule):** reverted the control
plane to the shipped `host-gw` default, fresh containers, same 2-node
topology (this time both nodes on the same `net-a`, i.e. `host-gw`'s ideal
same-L2 case). **Identical symptom**: both nodes `Ready` with distinct
PodCIDRs, `Starting flannel with backend host-gw` logged, but neither node
ever got a route to the other's PodCIDR (`ip route show` empty for
`10.42.0.0/16`), and no backend-data annotations appeared either.

**This means the blocker is pre-existing and backend-agnostic, not a
wireguard-native-specific failure.** It reproduces identically with the
currently-shipped, "supported" backend. This is consistent with, and very
likely the direct mechanism behind, two things this project already had
recorded as open, unresolved gaps before this phase:
- `docs/general-purpose-k8s-plan.md` Phase 1: "**What is NOT yet proven:**
  an actual `curl` to a `ClusterIP` reaching the backing Pod," plus its
  "kubelet Node-lister staleness under concurrent watch load" and CI-hang
  open follow-ups — all symptoms of the same family (an informer/watch not
  reliably completing or delivering under this project's control plane).
- `README.md`'s own gap table: "Service networking... actual traffic
  routing (`curl` a `ClusterIP` and reach the backing Pod) isn't proven
  end-to-end yet."

Root-causing *why* flannel's Node informer (a plain, unfiltered
`fields.Everything()` List+Watch — not the "unregistered type" or
"unsupported field selector" bug class found repeatedly elsewhere in this
project) doesn't converge is out of scope for this phase (it's a
control-plane watch-delivery question, not a Mesh-vs-wireguard-native
question), but it is the actual current blocker on *any* multi-node pod
networking in this project today, regardless of which CNI backend or
node-networking product is chosen. Recorded here rather than left
undiscovered, per the honest-correction rule.

## 4. Decision: Phase 9 — Cloudflare Mesh **not adopted**

- Mesh's only live value hypothesis over wireguard-native — NAT traversal,
  letting a BYO VM join without opening any inbound port — remains
  **unverified and unverifiable within this task's tooling** (§1). Adopting
  a product on an unverified hypothesis, with real known costs (double UDP
  encapsulation, ~14% UDP loss at load per the S4 research's third-party
  data, Linux client GA'd days before that research, zero K8s/CNI
  integration precedent anywhere), is not warranted.
- k3s's own `wireguard-native` + `--node-external-ip` mechanism — the
  Mesh-free alternative — is now **confirmed working correctly** for the
  parts this project's control plane is responsible for (backend selection
  propagation, internal/external IP separation). It costs nothing extra, is
  already-vendored, and needs no new product enablement.
- The one thing that didn't fully verify (cross-node route/tunnel
  convergence) is **not a reason to prefer Mesh** — the identical symptom
  was reproduced with the currently-shipped `host-gw` default in the same
  test rig. Switching to Mesh would not route around this blocker, since it
  lives in this project's own control-plane/watch layer, not in the choice
  of CNI backend.
- Per this task's own decision framework: "Mesh の実機検証ができない...
  「Phase 9 は不採用」という結論も正当な完了形態" — this is that outcome,
  reached honestly rather than forced toward completion.

**Recommendation recorded in docs** (see the docs updated alongside this
file): node-to-node networking across accounts/clouds that don't share a
VPC should use k3s's official `--flannel-backend=wireguard-native` +
`--node-external-ip`, not Cloudflare Mesh. Cluster DNS remains CoreDNS
(unchanged, already decided in S4 — not revisited here).

## Changes kept vs. reverted

- **Kept**: `cmd/agent --node-external-ip` flag (opt-in, harmless default,
  needed by anyone who wants to actually use wireguard-native per the
  recommendation above).
- **Kept**: `FlannelExternalIP` field added to `pkg/apiserver/supervisor.go`'s
  `clusterConfig` struct (currently unused — `defaultClusterConfig()` still
  omits it, so it serializes as its zero value/omitted — but the type now
  exists for whoever wires up per-cluster backend choice later).
- **Reverted**: `defaultClusterConfig()`'s `FlannelBackend` value, back to
  the shipped `"host-gw"` default. This phase evaluated Mesh, not "should
  k8flare's default multi-node backend change" — that's a separate,
  bigger decision (needs a place to store a per-cluster preference) out of
  this phase's scope, and the discovered cross-node-convergence blocker
  applies equally to both backends today regardless.
