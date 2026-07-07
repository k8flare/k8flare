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

## Per-Pod Mesh membership on the Containers backend (2026-07-08, user decision, implemented)

User decision: for the Fargate-style per-Pod microVM node model
(`workers/k8flare/src/nodes/`, one dedicated NodeVM per Pod,
`cf-containers-scheduler`), Mesh membership should be **Pod-scoped, not
Node-scoped** -- each Pod gets its own Mesh IP, decoupled from whatever
IP the underlying NodeVM has, so a future move to packing multiple Pods
onto one NodeVM (if that ever happens) wouldn't require re-architecting
Mesh membership.

**Design correction mid-task (recorded, not hidden):** the initial brief
for this proposed an Istio-style WARP sidecar container sharing the
Pod's network namespace. The user corrected this before implementation
started, with a better mental model: **AWS Fargate's ENI attachment, not
a sidecar** -- "サイドカーというよりは、FargateのENIみたいな感じかな。
透過的に自然と参加できる状態が理想" (more like Fargate's ENI than a
sidecar; the ideal is transparent, natural participation). This is a
better fit for THIS specific backend because, unlike a general
multi-tenant Kubernetes cluster, `workers/nodes` already runs the SAME
unmodified `cmd/agent` binary (full k3s embed: kubelet + containerd)
inside each per-Pod microVM as the BYO-VM path does on a bare host --
the BYO-VM Mesh-join mechanism (`pkg/meshconnector`, gate 2, already
shipped for the kubelet-proxy use case, commit `b6d4340`) already exists
in that exact binary and needed to be wired on for this backend, not
reinvented as a new sidecar image/container. No sidecar container was
built or evaluated further once this correction landed.

**What actually shipped:**

1. `pkg/meshconnector` (Go, `cmd/agent`) is REUSED UNMODIFIED -- exactly
   the same `warp-cli --accept-tos connector new <TOKEN>` /
   `warp-cli --accept-tos connect` / poll-`CloudflareWARP`-interface
   sequence gate 2 already proved. The only `cmd/agent` change is a new
   flag, `--mesh-ip-as-node-ip` (`cmd/agent/main.go`): when set, the
   discovered Mesh IP is ALSO applied to `agentConfig.NodeIP` (k3s's
   `--node-ip`), not just `agentConfig.NodeExternalIP` (the field the
   already-shipped BYO-VM feature sets).
2. **A genuine, source-verified correction to the "reuse `--node-external-ip`"
   assumption the task's own coordinating message proposed**: read the
   exact pinned kubelet source
   (`github.com/k3s-io/kubernetes@v1.36.2-k3s1`, fetched and inspected
   directly, not guessed from memory) to confirm which flag actually
   determines a `hostNetwork: true` Pod's `status.PodIP`.
   `pkg/kubelet/kubelet_pods.go`'s `generateAPIPodStatus` calls
   `kl.getHostIPsAnyWay(ctx)` -> `utilnode.GetNodeHostIPs(node)`, and for
   `IsHostNetworkPod(pod)`, sets `s.PodIP = hostIPs[0].String()` when
   unset -- i.e. hostNetwork PodIP mirrors the NODE'S InternalIP as
   recorded in its own Node object. Tracing k3s's own flag wiring
   (`pkg/daemons/agent/agent_linux.go`: `argsMap["node-ip"] = cfg.NodeIP`)
   confirms kubelet's real `--node-ip` argument -- fed by
   `agentConfig.NodeIP`, NOT `agentConfig.NodeExternalIP` -- is what
   becomes that InternalIP. **`NodeExternalIP` (what the already-shipped
   BYO-VM Mesh feature sets) has NO effect on hostNetwork PodIP at
   all** -- it feeds a separate `ExternalIP` node-status field and
   annotation, used by `target.ts`'s `resolveKubeletTarget` for a
   completely different purpose (the gateway dialing a BYO VM's kubelet
   through the `MESH` vpc_networks binding), which this Containers
   backend doesn't use in the first place (it reaches kubelet through
   `containerFetch`, never through `MESH`). Reusing `NodeExternalIP`
   alone, as literally described, would have shipped a Mesh-joined NodeVM
   whose Pod's `status.PodIP` was silently UNCHANGED (still the VM's
   plain internal IP) -- a plausible, easy-to-miss bug this session
   avoided only by reading the real upstream mechanism rather than
   assuming behavior would "just carry over." Recorded per rule 4 as a
   correction to a design point that was proposed but not yet
   implemented, not a shipped regression.
3. Also confirmed (same read): the fix is scoped so it does NOT touch the
   already-shipped BYO-VM behavior. `--mesh-ip-as-node-ip` is a new,
   separate, default-`false` flag; BYO-VM's `--mesh-connector-token`
   usage (README's documented recipe) is untouched, so flannel
   `wireguard-native`'s reliance on the real internal IP staying the VM's
   own address (a DIFFERENT, already-working use of
   `NodeIP`/`NodeExternalIP`'s split) is undisturbed.
4. `workers/k8flare/src/nodes/meshconnector.ts` (new, Worker-side TS):
   mints a FRESH `warp_connector` per Pod at schedule time
   (`scheduler.ts`'s `reconcile()`, right before booting that Pod's
   NodeVM) via the exact same two API calls gate 2 proved by hand --
   `POST /accounts/{id}/warp_connector`, `GET .../token` -- this runs
   Worker-side (not inside the microVM) because only the Worker holds
   the Cloudflare API credentials (`CLOUDFLARE_API_TOKEN`/
   `CLOUDFLARE_ACCOUNT_ID`, new optional `env.ts` fields; absent either,
   `createMeshConnector` returns `undefined` and the Pod boots without
   Mesh membership -- graceful degradation, matching
   `pkg/vkubeproxy`/`pkg/dnsshim`'s own posture for their optional
   enhancements, not a hard scheduling failure). The resulting token is
   passed to the NodeVM as `MESH_CONNECTOR_TOKEN` (`nodevm.ts`'s `up()`),
   which `cmd/agent` already reads as its `--mesh-connector-token`
   default -- no entrypoint.sh wiring was needed for the token itself,
   only for starting `warp-svc` (see below) and passing
   `--mesh-ip-as-node-ip=true`.
5. **Teardown wired to Pod deletion**, so the 50-connectors/account cap
   doesn't leak per Pod: `TrackedVM` (`scheduler.ts`) gained a
   `meshConnectorId` field; `teardown()` (called from the normal
   pod-gone path AND the FailedScheduling/NODE_READY_TIMEOUT_MS path)
   calls `deleteMeshConnector`; the `/admin/destroy` whole-cluster
   teardown path also deletes every tracked Pod's connector before
   dropping DO state.
6. **Node image**: `workers/k8flare/images/node/Dockerfile` moved from
   `alpine:3.21` to `debian:bookworm-slim` (gate 1's finding: no
   Alpine/musl `cloudflare-warp` build) and installs `cloudflare-warp`
   via gate 1/2's own proven apt recipe, reused verbatim from
   `spikes/s17-mesh-nodevm/Dockerfile`. Debian's `iptables` package
   already provides `ip6tables` (no separate package needed, unlike
   Alpine) -- found by actually building the image, not assumed.
   `entrypoint.sh`'s cgroup-evacuation line used `busybox xargs`
   (Alpine-only); switched to plain `xargs` (GNU findutils, present on
   Debian by default).
7. **New in this backend, absent from the BYO-VM path**: since this
   microVM's entrypoint IS PID 1 with no init system, `warp-svc` (the
   daemon `warp-cli`/`pkg/meshconnector` talk to -- on a real BYO VM
   host, the `cloudflare-warp` .deb's postinst enables a systemd unit
   automatically) must be started and waited-on explicitly.
   `entrypoint.sh` now does, conditionally on `MESH_CONNECTOR_TOKEN`
   being set: `warp-svc &`, then a bounded (10s) poll of
   `warp-cli --accept-tos status` before `exec`-ing `k8flare-agent` --
   same one-time-bounded-wait style as `meshconnector.go`'s own 30s
   `waitForMeshIP` poll, not a resident poll loop (cost invariant #3 is
   about DO alarms, not a boot-sequence wait).

**What was verified, and how (rule 2 -- actually run, not read-and-conclude):**

- Go: `GOOS=linux GOARCH=amd64 go build`/`go vet` on `cmd/agent` and
  `pkg/meshconnector` clean; `gofmt -l` clean;
  `CLOUDFLARE_ACCOUNT_ID=ed17c5c18eb6052e70234ec181709fba go test -count=1
./pkg/apiserver/...` (after `rm -rf .wrangler/state`) passes unchanged
  (this suite runs with Containers disabled, so it does not exercise
  `cf-containers-scheduler`'s runtime behavior either before or after
  this change -- a pre-existing ceiling on what it can catch here, not
  new).
- `bash scripts/build-wasm-chunks.sh`: apiserver 62,863,479 bytes / kcm
  64,019,330 bytes -- byte-for-byte unchanged from the confirmed-clean
  baseline (expected: this change touches `cmd/agent`, which neither
  wasm target's build graph reaches).
- TypeScript: `npx tsc --noEmit -p workers/k8flare/tsconfig.json` and
  `vp check` both clean for the new/changed files (`nodes/meshconnector.ts`,
  `nodes/scheduler.ts`, `nodes/nodevm.ts`, `env.ts`).
- **`nodes/meshconnector.ts`'s control flow was exercised for real**
  against a mocked `fetch` (this repo has no existing TS unit-test
  harness for `workers/k8flare/src` -- its own convention is `go test`
  driving a real `wrangler dev` instead, so this was a throwaway
  scratchpad script, not a committed test): confirmed the
  unconfigured-credentials no-op path, the happy-path POST+GET request
  shapes and returned `{id, token}`, that a failed create makes no
  further calls, and that a failed token-fetch triggers a cleanup DELETE
  of the half-created connector (leak prevention) rather than silently
  dropping it.
- **The full node-image boot sequence was exercised for real** in a
  privileged local Docker container (this sandbox's ARM-Mac +
  no-real-Containers-runtime limitation is pre-existing and unchanged --
  see S16 -- so this is the same "isolated container environment"
  substitute gate 1 already used, not a full `wrangler dev
--enable-containers` run): built the actual switched `Dockerfile`
  (image grew to 1.48GB, see cost-model.md), ran the actual
  `entrypoint.sh` with a real cross-compiled `k8flare-agent` binary
  (`scripts/build-nodes-agent.sh`) and `--privileged --cgroupns=host`,
  and confirmed: `k3s check-config` runs, the cgroup dance and PATH setup
  complete, `warp-svc` starts headless and the readiness loop detects it
  ready (~1s), and `k8flare-agent` reaches `meshconnector.Run` and fails
  fast and cleanly on an intentionally-invalid token (`warp-cli
connector new`: `Error: Failed to parse WARP Connector token`, `exit
  status 1`, surfaced through `log.Fatalf` in well under a second -- not
  a hang, not silently stuck waiting on a not-yet-ready `warp-svc`).
- **Real Cloudflare API resources**: none created or deleted this
  session. No `CLOUDFLARE_API_TOKEN` (Zero Trust/Tunnel-scoped) was
  available in this sandbox. A `wrangler`-OAuth session WAS available
  (KOOFFICE account `ed17c5c18eb6052e70234ec181709fba`, `connectivity
  (admin)` scope, confirmed via `wrangler whoami`), but this session
  declined to extract/repurpose that token for raw `warp_connector` API
  calls outside `wrangler`'s own CLI surface (`wrangler` itself has no
  `warp_connector`/Mesh subcommand, only `wrangler tunnel`, a different
  resource type) -- the same reservation this doc's own 2026-07-07
  correction already recorded ("feels like the wrong way to use that
  credential"), applied consistently rather than relaxed under this
  task's time pressure.

**Open gap, honestly recorded (not a blocker, a follow-up):** the one
thing NOT verified end-to-end is a real Pod's `status.podIP` actually
becoming a live Mesh IP against the real KOOFFICE account and a real
Cloudflare Containers instance. Everything upstream of that (the Go flag
wiring, the exact kubelet mechanism it targets, the Worker-side API call
shapes, the full node-image boot sequence up to the point a real
connector token would be consumed) was verified for real by the methods
above; only "mint one real connector, boot one real NodeVM against it,
`kubectl get pod -o wide` and see the Mesh IP" remains, gated on
`CLOUDFLARE_API_TOKEN` access this sandbox didn't have. Whoever resumes
with that access: run one real Pod through this backend with
`CLOUDFLARE_API_TOKEN`/`CLOUDFLARE_ACCOUNT_ID` set, confirm the Pod's
`status.podIP` is a `100.96.0.0/12` address, then `DELETE` the connector
and confirm via `GET /accounts/{id}/warp_connector` that it's gone (this
task's own instruction for how to close this gap cleanly).

### Update (2026-07-08): the Worker-side API call shapes are now verified for real

A real `CLOUDFLARE_API_TOKEN` was made available for one verification
pass (outside this repo's own dev config/CI, per the operator directly).
`nodes/meshconnector.ts`'s exact three request shapes were run against
the real KOOFFICE account (same account as gate 2), independent of any
Worker/DO code -- plain `curl`, mirroring gate 2's own original proof
method:

1. `POST /accounts/{id}/warp_connector {"name": "k8flare-verify-test-<ts>"}`
   -- `"success":true`, a real connector id returned
   (`3ef26518-24d4-4ff9-a8b6-09cb5a4d12e1`).
2. `GET /accounts/{id}/warp_connector/{id}/token` -- `"success":true`.
3. `DELETE /accounts/{id}/warp_connector/{id}` -- `"success":true`,
   `deleted_at` set.
4. Follow-up `GET /accounts/{id}/warp_connector` (list) confirmed the
   connector no longer appears -- no leaked capacity against the
   50-node cap from this verification.

**Operational note, recorded honestly:** the verification script's own
output-redaction (meant to hide the connector's secret/token fields
before they reached the terminal/transcript) targeted the wrong JSON key
(`"secret"` instead of the API's actual `"TunnelSecret"`/`"token"`
fields), so both were briefly printed in full during step 1's output.
The connector was deleted moments later in step 3 and confirmed gone in
step 4, so the exposed credential is no longer valid against any real
Mesh network -- but the redaction logic itself should be fixed (match
the real response shape above) before reusing this script for a
non-cleanup-scoped test.

**Still open**: this only closes the Worker-side API client half of the
gap above. `cf-containers-scheduler`'s actual `reconcile()` call site,
a real booted NodeVM, and confirming a real Pod's `status.podIP` becomes
a live Mesh IP remain unverified -- the same remaining gap described
just above this update, not yet closed.

### Update (2026-07-08, later same day): deployed to real Cloudflare Containers, found and fixed two real bugs, then found the actual root cause -- a likely platform-level networking constraint

User authorization: "Cloudflare Containersにデプロイしていいので検証してください" (deploy to Cloudflare Containers, it's fine, please verify), with an explicit requirement to leave no Container running afterward. This entry records everything done against the real, live `k8flare` Worker (`https://k8flare.kooffice.workers.dev`, the real KOOFFICE account) -- every resource created was deleted again; see the cleanup confirmation at the end.

**Bug 1 (found and fixed): `cf-containers-scheduler`'s new-pod loop let two overlapping `reconcile()` invocations both try to mint a Mesh connector for the same Pod.** A DO's single-threaded execution can still interleave two invocations at an `await` boundary (e.g. a pod-create poke and the 15s safety-net alarm landing close together). The old code only persisted `tracked[uid]` to storage at the very end of `reconcile()`, so both invocations read the same pre-loop snapshot, both saw the pod as unclaimed, and both called `createMeshConnector`. Reproduced live twice in a row (`mesh-verify-test`/`mesh-verify-test2`): the second, interleaved call's `POST .../warp_connector` 409'd ("You already have a tunnel with this name"), returned `mesh: undefined`, and overwrote the first (successful) connector's id with `undefined` when both invocations' writes landed -- leaking it past teardown (`if (vm.meshConnectorId)` found nothing to delete). **Fix**: claim the pod (persist `tracked[uid]` to storage) immediately after generating its `nodeName`, before any awaited call -- a concurrent invocation's re-read now sees the claim and skips it via the existing `if (tracked[uid]) continue` guard.

**Bug 2 (found and fixed, same root cause, different window): deleting a Pod while its Mesh connector creation was still in flight also lost the connector id.** Even with Bug 1 fixed, a second race remained: `tracked[uid].meshConnectorId = mesh?.id` was only written *after* the much slower `stub.up()` call (which waits for the container's ports, can take seconds). Deleting the Pod during that window let a second, delete-triggered `reconcile()` process `podGone` with `meshConnectorId` still unset. Reproduced live (`mesh-verify-test3`). **Fix**: persist `meshConnectorId` immediately once `createMeshConnector` resolves, before calling `stub.up()`, not after.

**Bug 3 (found and fixed, a design flaw, not a race): `cmd/agent`'s hard `log.Fatalf` on Mesh-join failure could leave a Pod stuck in `Pending` forever with no Node ever registering.** `meshconnector.Run`'s `waitForMeshIP` already has its own bounded 30s timeout and returns an ordinary error on failure -- but `cmd/agent/main.go` treated that error as fatal, killing the *entire* agent process (kubelet + containerd embed included) before it got anywhere near Node registration. Reproduced live across five consecutive Pod attempts (`mesh-verify-test`, `test2`, `test3`, `test4`, `test5`): every one sat in `Pending` with no `Node` object ever created, because the container process was crashing (silently, from this session's vantage point -- no console/log access) during the Mesh-join attempt and never got further. **Fix**: made this non-fatal, matching `pkg/vkubeproxy`/`pkg/dnsshim`'s existing "log and continue without the optional enhancement" posture -- Mesh is meant to be additive on top of a working cluster, not a precondition for one. After this fix, `mesh-verify-test6` immediately registered a Node and bound successfully (kubelet became healthy ~40-56s after boot, consistent with typical Containers cold-start).

**Root cause of the actual remaining question ("does a Pod ever get a real Mesh IP?"), pinned down with real evidence, not guessed:** with the above three fixes deployed, Pods now schedule and bind successfully -- but the Mesh connection itself never establishes. A temporary diagnostic HTTP endpoint (added to `cmd/agent`'s existing kubelet plain-HTTP proxy, gated behind the same cluster-token auth as every other route through it, removed again once this was root-caused -- see the reverted diff, not shipped) surfaced real `warp-svc` logs and live `warp-cli`/`ip` output from inside a genuinely running Containers instance (`mesh-verify-test6`/`test8`):

- `warp-cli settings` shows the account's Zero Trust policy locks `WARP tunnel protocol: MASQUE` (a QUIC/HTTP-3-based transport, not classic WireGuard-over-UDP, but still fundamentally a UDP-carried protocol).
- `ip addr show CloudflareWARP` → **"Device does not exist"** -- the tunnel interface is never created at all, at any point.
- `warp-cli status` (after the connect attempt) → **"Status update: Disconnected, Reason: Manual Disconnection"**.
- warp-svc's own internal watchdog logs **"Watchdog reports hung daemon (watchdog_name=main loop, hang_count=1)"** -- its main event loop got stuck.
- Meanwhile, from the *same* container, a plain `curl -m 5 https://www.cloudflare.com` succeeds (`http_code=200`) -- ordinary HTTPS/TCP egress on port 443 works completely normally.

This is a clean, telling contrast: TCP/HTTPS egress works; the QUIC/UDP-carried WARP tunnel does not, ever, on this backend. It matches Cloudflare's own Containers documentation (`developers.cloudflare.com/containers/platform-details/outbound-traffic/`), which describes outbound traffic handling entirely in terms of HTTP/HTTPS on ports 80/443 plus DNS, with no mention of UDP or QUIC egress capability anywhere. **Most likely explanation: Cloudflare Containers' current network egress model does not carry the QUIC/UDP-based MASQUE tunnel WARP requires** -- a platform-level constraint, not a bug in this repo's code. Recorded as the most likely explanation, not an absolutely confirmed one: no Cloudflare support channel was consulted directly, and the documentation excerpts fetched this session describe an *optional* JS-side "outbound handler" proxy feature this repo doesn't even use, which may or may not be the same restriction that applies to a container's raw network stack by default. **Whoever revisits this: the concrete next step is asking Cloudflare directly (or testing with `enableInternet`/`allowedHosts` explicitly configured) before concluding this can never work**, since the exact mechanism (handler-level restriction vs. a hard network-level UDP block) was not distinguished with full certainty this session.

**Verification performed this session, all against real infrastructure (rule 2):**

- `wrangler secret put CLOUDFLARE_API_TOKEN`/`CLOUDFLARE_ACCOUNT_ID` set on the live `k8flare` Worker (previously unset).
- `wrangler deploy` run five times total (once per fix, plus once to remove the temporary diagnostic endpoint) against the real KOOFFICE account -- each time rebuilding `workers/k8flare/images/node/k8flare-agent` via `scripts/build-nodes-agent.sh` first. **Operational lesson recorded honestly**: this script must run before every deploy that touches `cmd/agent` -- the first attempt at fixing Bug 3 silently deployed a stale binary (Docker reused a cached `COPY` layer) because this step was skipped, wasting a full test cycle before the mismatch was caught by checking the prebuilt binary's own mtime.
- **Real Pods created and deleted end-to-end nine times** (`mesh-verify-test` through `mesh-verify-final`) against the live cluster, using the real cluster bearer token (`.secrets/token`, confirmed to be the actually-deployed `K3S_TOKEN` -- a second candidate file, `.secrets/k3s-token`, turned out to be a stale/different value and returned 401).
- **A newly-noticed, real Containers platform characteristic**: a freshly `wrangler deploy`ed image is not always picked up by the very next Pod schedule -- one attempt (`mesh-verify-test7`) ran against a stale cached image for several minutes before a retry (after an extra ~60s wait) picked up the new one. Not fully characterized (how long this propagation can take in the worst case), but real and worth remembering before concluding a fresh deploy "didn't work."
- **Confirmed the fixes hold up even under a slow/failing run**: the final verification Pod (`mesh-verify-final`) took long enough to hit `NODE_READY_TIMEOUT_MS` twice (two real `FailedScheduling` events, 5 minutes apart) before being manually deleted -- and its Mesh connector was still fully cleaned up (confirmed absent from `GET .../warp_connector`'s list, not merely `deleted_at`-stamped) through both the timeout-driven teardown and the final manual delete. This is likely ordinary Containers cold-start variance (the debian-based image is ~1.48GB, per the cost-model.md entry), not a regression -- `mesh-verify-test6`/`test8` succeeded in ~40-56s using the same code shape.
- **Full account cleanup confirmed at the end**: `GET /api/v1/namespaces/default/pods` and `GET /api/v1/nodes` both empty; `GET .../warp_connector` shows only the pre-existing `k8flare-mesh-node1` (from this file's own gate 2 verification, unrelated to this session, not touched); all three NodeVM tiers' `wrangler containers instances` show zero currently-running instances.
- `go build`/`gofmt`/`go vet` clean for `cmd/agent` (`GOOS=linux GOARCH=amd64`, its real target); `npx tsc --noEmit` and `vp check` clean for `workers/k8flare/src/nodes/scheduler.ts`; `bash scripts/build-wasm-chunks.sh` unchanged at the confirmed-clean apiserver (62,863,479 bytes) / kcm (64,019,330 bytes) baselines (this session's changes don't touch either wasm target's build graph); `CLOUDFLARE_ACCOUNT_ID=... go test -count=1 ./pkg/apiserver/...` (after `rm -rf .wrangler/state`) green.

**Net conclusion**: the two concurrency bugs and the fatal-crash design flaw are real, fixed, and worth keeping regardless of Mesh's fate, since they'd affect any future retry of this feature (or anything else that follows this same "mint an external resource per Pod, tear it down on delete" shape). The feature's actual goal -- a real Mesh IP as a Pod's `status.podIP` -- is not achieved, and is now blocked on a specific, well-evidenced (though not 100% Cloudflare-confirmed) platform constraint: Cloudflare Containers' egress appears not to carry WARP's QUIC/UDP-based MASQUE tunnel. This is the resume point for whoever picks this up next, not a dead end -- the recommended next step is confirming the exact egress restriction with Cloudflare directly.
