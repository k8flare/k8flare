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
