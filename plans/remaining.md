# Remaining work

Inventory of `plans/minimal-control-plane.md`, the queue remapping plan,
and the admission/webhook plan, against the current tree. Done items stay
done. Work proceeds in waves: close Kubernetes API holes first, then edge,
then Cloudflare-only compute, then research.

Constraints that stay true for every wave: Worker is stateless, Durable
Objects hold state, Queues drive async work, no cron, alarms only at a
known deadline.

## Already done

- Repository layout (`plans/repository-layout.md`).
- Queue rewrite (scheduler, workloads, accounts, CRDs, GC, extensions).
- Resident controllers / run-window / poke / keepalive alarm removed.
- Admission: `admissionregistration.k8s.io`, RemoteAdmit, VAP, webhooks
  (`https://k8flare.com/worker/<name>` or `clientConfig.service` via
  NodeTunnel `/dial`).
- CRD `k8flare.io/controller` → `k8flare-extensions`.
- LoadBalancer first cut: status hostname
  `{name}--{namespace}.k8flare.com`, front proxy on that host or
  `/svc/{namespace}/{name}`.
- NodeTunnel hibernation rebind; supervisor `EgressSelectorMode=cluster`
  so the agent may dial pod IPs.
- SSAR / SAR / TokenReview talk to the real RBAC authorizer (the old
  always-allow stub is gone).
- Namespaces no longer advertise `deletecollection`.

## Wave A — API holes that block kubectl and official e2e

1. **ClusterIP allocation.** Creating a Service leaves `spec.clusterIP`
   empty, so the endpoint controller marks it headless. Same shape as
   node PodCIDR: on create, pick a free address from
   `supervisor.ServiceCIDR` (`10.43.0.0/16`), skip `.0`, `.1`
   (kubernetes), `.10` (cluster DNS), and `ClusterIP=None` /
   `ExternalName`.
2. **`deployments/scale` (and RS/RC/STS if cheap).** `kubectl scale`
   is NotFound. Add the scale subresources to `Served` and a Scale REST
   that reads/writes `spec.replicas`.
3. **Official AdmissionWebhook e2e via `clientConfig.service`.** Worker
   echo path is proven. The service path needs a Ready node and ClusterIP
   (A1). Run `make e2e SET=admission` against prod.
4. **`pods/exec`, `attach`, `portforward`, `log`.** Same NodeTunnel as logs;
   SPDY/WebSocket upgrade through the remotedialer. Needed for a large
   slice of official e2e.
5. **Node authorizer.** `system:node` is a static group bind. Upstream
   node authorizer is the missing piece for kubelet isolation.

## Wave B — edge

6. **LoadBalancer finish.** One-label host
   `{name}--{namespace}.k8flare.com` (Universal SSL, no ACM wildcard).
   Host dispatch plus zone route `*.k8flare.com/*`. workers.dev rejects
   a foreign Host with 403. Multi-port and HTTP/2 later.
7. **Gateway API.** Types from `sigs.k8s.io/gateway-api`. Host/path
   dispatch is another Loader worker, same as `/svc`. Domain assignment
   is the one-label host from B6, not a Provisioner.
8. **Cloudflare Access authenticator.** `Cf-Access-Jwt-Assertion` →
   JWKS, `iss`/`aud`, `user.Info{Name: email}`. RBAC binds the email.
   kubectl needs a service token or exec plugin. Alongside admin/node
   tokens, not instead of them.

## Wave C — Cloudflare compute and durability

9. **Pod-on-Containers.** Revive from `origin/main` (`workers/nodes`),
   rewritten for queues: no 10s reconcile alarm; Pod writes enqueue;
   start/Ready waits are Workflow `step.sleep`. Read the recorded
   regression on `main` first. v1 gaps: no Pod IP, `restartCount` 0, no
   UDP.
10. **R2 for those Pods.** Mint scoped credentials into the container
    env; FUSE mount is optional.
11. **Cluster DO export.** Native SQLite PITR covers 30 days. First
    admin endpoint `POST /restore?to=<timestamp>`. R2 snapshot is
    second (retention beyond 30 days). Schema is still moving; do not
    freeze an export format yet.

## Wave D — later / research

12. **Workers VPC + Mesh instead of remotedialer.** Spike first: can a
    Durable Object `connect()` through `vpc_services`? If no, keep
    NodeTunnel.
13. **Multi-node pod net.** Two VMs on Mesh, `ping`/`iperf3`, then
    flannel `host-gw` with Mesh IPs. Single-node is the only topology
    exercised today.
14. **Cluster DNS.** No Worker substitute for UDP/53. Real CoreDNS
    Deployment once kube-proxy or an equivalent exists. Until then
    `dnsPolicy: Default`.
15. **metrics-server** as a fixed `METRICS` worker via kubelet
    `/metrics/resource`. Needs a periodic wake (same shape as CronJob,
    not an always-on alarm). HPA after that.
16. **kube-aggregator.** Do not import it. Runtime `APIService` backends
    do not map onto deploy-time Service Bindings.
17. **ClusterUpgrade.** `wrangler deploy` is the cutover. Add a kine
    schema version before locking production data. Agent skew is
    upstream's problem.
18. **Multi-tenancy.** `idFromName("default")` swap is not enough:
    Worker-wide tokens, NodeTunnel names collide, name reuse bleeds
    SQLite. Per-cluster vault + UID-suffixed DO names, from the old
    `clusterop` design.
19. **DO Facets.** Isolation inside one Cluster DO (`ca-vault`,
    events). Not throughput. Trigger: `/stats` approaching 10 GB.

## Known gaps in the dev stack

- Pod logs over websocket upgrade and open, but no bytes reach the client:
  `[sig-node] Pods should support retrieting logs from the container over
  websockets` now fails in 6 s with `Unexpected websocket logs:` and an empty
  payload, where it used to hang for 30 s. Routing is fixed -- only the shell
  can terminate an upgrade, since no Go package has `WebSocketPair`, and
  `isStreamPath` had listed exec, attach and portforward but not log. What is
  left is the transport, not framing: the relay passes bytes straight through
  (`podstream.ts:28-30`) and the channel prefix exec and attach carry comes from
  the kubelet. `index.ts:143-160` sets `Upgrade: websocket` and requires a socket
  back, and `node-tunnel/src/index.ts:106` only serves `/node/...` on an upgrade
  -- but `containerLogs` is a plain HTTP GET returning text/plain
  (`apiserver-core/podlog.go:65-71`), so nothing upgrades, no socket comes back
  and the shell closes 1011. The handshake and 101 are sent before the waitUntil
  work runs, which is why the client sees an opened-then-closed socket with an
  empty payload rather than an error. See `plans/podlogs-websocket.md`.

- A full suite run produces around 200 `bridge: fetch timed out` and the specs
  that fail are not the same twice. They are load-dependent rather than defects:
  the six that failed one run pass five-of-six when run alone at the same
  `procs=4`, and both `EndpointSliceMirroring` and `EndpointsController` were
  verified by hand to work in 18 s and 15 s. The failing fetch that was measured
  properly had `inflight=1`, so it is not contention, and `turn=true`, so it is
  not the turn machinery. Judging the remaining specs needs a run against real
  Cloudflare rather than `wrangler dev --local`.

- The attach/detach controller is deployed again, built with `-tags
  grpcnotrace`. The 69.9 MB worker was not a fat client base: grpc imports
  `golang.org/x/net/trace`, which imports `html/template`, and template's
  `MethodByName` makes the linker keep every exported method of every reachable
  type (54,006 functions against 26,239 without it). With the tag the optimized
  worker is 44.1 MB. `make wasm` has not been run against it yet, so the worker
  has still never loaded on a real runtime.

- `make wasm` while `wrangler dev` is watching the assets directory kills the
  dev server: its reload stats a chunk that the build is still rewriting and
  it exits with ENOENT. Nothing restarts it, and the node agents then fail
  every lease write, so the cluster dies silently. Stop the dev server before
  a rebuild, or build to a staging directory and move the chunks into place.
- The node lease write fails constantly against `wrangler dev` with
  `an error on the server ("Error: Network connection lost.")` -- 1879 times
  in 45 minutes, while the lease still lands every 6-10s. On 2026-09-25 the
  same path wedged on repeated 409s instead and the cluster ran for twelve
  hours with two dead kubelets, both nodes still reporting Ready.

## Worker size

Every worker is under the 64 MiB Loader cap, which the runtime enforces
exactly: `Dynamic Worker code size (N bytes) exceeds the maximum allowed size
of 67108864 bytes`. `make sizes` prints bytes and linked function count per
worker; the cap is really a budget of about 45,000 linked Go functions at
1.25-1.65 KB each. `wasm-opt` cannot help past a point -- it rewrites Code and
leaves Data, which is 45-53% of every binary, byte-identical.

What the headroom came from:

- **Controllers are a shard a worker opts into.** `packages/workloads` holds the
  sync harness and exposes `Deps` plus `Register`; the controllers live in
  `shards/*` with the tests for their own controllers, and a worker picks them
  up by importing them. `workloads` went 161% -> 86% of cap this way.
  ValidatingAdmissionPolicy is the one shard kept separate: its upstream
  TypeChecker closure is 13 MB and nothing else wants it, so a pod write should
  never pay for CEL. Split only as far as the cap forces -- each extra worker
  pays ~46 MB of duplicated base and another cold start.
- **Marginal cost is not standalone cost.** Taking resourcequota out of a shard
  bought 2.25 MB though a quota-only worker is 49 MB, because its co-tenants
  already pull those groups. Measure each removal; do not extrapolate.
- **openapi serves a document baked at build time.** It used to call
  `installer.Install` to stand up the whole REST surface purely to enumerate
  routes, and `zz_generated_groups.go` blank-imported every API group for the
  registration side effects. 102% -> 21%. `bakeopenapi` renders it under
  `make gen`; the spec builder stays behind `//go:build !js` as `SpecHandler`.
- **A controller can hold a plugin it cannot use.** `volumeexpand` linked 30 MB
  of `pkg/volume/csi` through `csi.ProbeVolumePlugins()`, but
  `ExpandableVolumePlugin` needs `ExpandVolumeDevice`, which `pkg/volume/csi`
  does not define, so the controller always fell through to its
  `ExternalExpanding` branch. Passing nil is behaviour-identical and 101% -> 56%.

Rejected, with the reason, so it is not retried blindly:

- **Stripping the generated protobuf codecs from the js build.** Worth 9.15 MB
  on every worker, and it breaks kubectl: kubectl sends protobuf request
  bodies, ContentType is not negotiated the way Accept is, and there is no flag
  to change it, so every create, apply and patch returns UnsupportedMediaType.
  Reads negotiate down and keep working, which makes the damage easy to miss.
- **Folding attachdetach into workloads.** Merging adds code to one worker
  rather than removing it; measured at 128% of cap.

## Not doing

- Cron or always-on alarms.
- A Cloudflare-native CoreDNS.
- Hand-rolling kube-aggregator.
- Splitting remaining work into topic branches up front.

## This session

Wave A.1 ClusterIP allocation, then A.2 scale if the first lands clean.
