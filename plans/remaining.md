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
4. **`pods/exec`, `attach`, `portforward`.** Same NodeTunnel as logs;
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

- Three workers are over the 64 MiB Loader cap: `workloads` at 108 MB,
  `attachdetach` at 70 MB and `openapi` at 69 MB. `workloads` has been over
  since before the cap check was repaired; the other two sat at 99.9% and
  crossed when the admissionregistration group entered the shared js
  clientset. They need splitting or trimming, or the cap constant needs to
  be checked against the real Loader limit. `make wasm CAP=<bytes>` builds
  them meanwhile.
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

## Not doing

- Cron or always-on alarms.
- A Cloudflare-native CoreDNS.
- Hand-rolling kube-aggregator.
- Splitting remaining work into topic branches up front.

## This session

Wave A.1 ClusterIP allocation, then A.2 scale if the first lands clean.
