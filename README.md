# k8flare

Kubernetes control plane on Cloudflare Workers.

Runs a minimal K8s API server as a Cloudflare Worker with Durable Objects (SQLite-backed etcd-like storage), enabling `kubectl` to manage workloads without traditional infrastructure.

## Architecture

```
kubectl / kubelet (BYO VM: cmd/agent, an unmodified k3s agent embed)
   │ HTTPS + token
   ▼
gateway (TypeScript Worker — the only public one)
   │ auth, watch streaming, kubelet proxy (VPC)
   ├──► apiserver (Go, compiled to WASM)
   ├──► runtime (TypeScript: CRDs, DynamicWorker/WorkerTrigger)
   ▼ Durable Object binding
storage (TypeScript)
   ├─ Cluster DO — revision authority, kine-compatible log, per-namespace Facets
   └─ WatchHub DO — watch fan-out over hibernating WebSockets

nodes (TypeScript + Cloudflare Containers, optional, talks to apiserver directly)
   └─ VirtualNode DO — registers cf-containers-<pool>, renews its Lease,
      reconciles Pods onto PodContainerSmall/Medium/Large DOs
```

**Components** — 5 Cloudflare Workers, each its own deploy unit with its own
`wrangler.jsonc` (`workers/gateway`, `workers/apiserver`, `workers/storage`,
`workers/runtime`, `workers/nodes`):

- **gateway** — the only public Worker: authentication, watch streaming, kubelet proxying
- **apiserver** — Go compiled to WASM; a table-driven API server built from real `k8s.io/kubernetes` types, not a hand-rolled subset of the wire format
- **storage** — the `Cluster` Durable Object (kine-compatible revision log, per-namespace storage via Durable Object Facets) and the `WatchHub` Durable Object (watch fan-out, hibernating WebSockets)
- **runtime** — CRDs, `DynamicWorker`/`WorkerTrigger` custom resources
- **nodes** — optional virtual-kubelet-style Pod backend on Cloudflare Containers, for clusters that don't want to run a BYO VM agent just to try a Pod. See "Node backends" below for its allowlist and networking limitations.

**`cmd/agent`** is an unmodified k3s agent (kubelet + containerd + flannel) you run yourself (EC2 or any Linux host) — see Agent Setup below.

**Pod scheduling and workload controllers need `cmd/scheduler` and
`cmd/controller-manager` running too**, alongside the agent — the real,
unmodified upstream `kube-scheduler`/`kube-controller-manager` binaries.
These currently run as host processes, not inside a Worker:
`kube-scheduler` doesn't compile for Cloudflare's Go/WASM target at all,
and real `kube-controller-manager` code doesn't fit a Worker's 10MiB
deploy budget once linked against `client-go`. See
[`docs/platform-verification.md`](docs/platform-verification.md)'s S8
section for the full investigation. (A `workers/controllers` Worker
exists in the repo and runs correctly end-to-end against local
`wrangler dev`, but isn't part of the release for exactly that size
reason — Endpoints/EndpointSlice generation and Node lifecycle don't need
it, though: those run as synchronous Go inside `apiserver` itself, not as
a controller-manager controller. See the Controllers table below.)

## Kubernetes API Support

k8flare implements a subset of the Kubernetes API, not a full distribution. This
reflects what's actually verified against the official
[`sig-scheduling`/`sig-api-machinery` conformance suite](https://github.com/kubernetes/kubernetes/tree/master/test/e2e)
as of this writing, plus direct inspection of the registered API scheme.

### Core resources

| Resource                                                                                                               | Status                                                                                                                                                              |
| ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Namespace, ConfigMap, Secret, Pod, Node, ServiceAccount, Endpoints, Service, Event, Lease, LimitRange                  | ✅ CRUD, watch, label/field selectors                                                                                                                               |
| CSIDriver, CSINode, RuntimeClass                                                                                       | ✅ CRUD, watch                                                                                                                                                      |
| `DynamicWorker`, `WorkerTrigger` (custom resources)                                                                    | ✅ CRUD, watch                                                                                                                                                      |
| ReplicaSet, Deployment, Job, CronJob, DaemonSet                                                                        | ✅ Backed by the real, unmodified `kube-controller-manager` binary (`cmd/controller-manager`, same embed pattern as `cmd/scheduler`), verified end-to-end           |
| StatefulSet, ReplicationController, PodDisruptionBudget, ResourceClaim, ResourceSlice, DeviceClass, ControllerRevision | ⚠️ Registered but always empty — exist only so the real kube-scheduler's/kube-controller-manager's informers for these types can sync; not backed by any controller |
| PersistentVolume, PersistentVolumeClaim, StorageClass                                                                  | ❌ Not implemented                                                                                                                                                  |
| Generic `CustomResourceDefinition` (dynamic CRDs)                                                                      | ❌ Only the two built-in custom resources above; no generic CRD registration mechanism                                                                              |

### What's missing for general-purpose use

The table above covers the API surface; this is about whether a typical
workload actually _runs_ the way it would on a normal cluster. These are the
gaps that matter most for everyday use, roughly in the order most users would
hit them:

| Area                                     | Gap                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Deploying anything beyond a bare Pod** | ReplicaSet, Deployment, Job, CronJob, and DaemonSet all work, backed by the real `kube-controller-manager` (`cmd/controller-manager`) rather than a reimplementation — verified end-to-end (create, rolling update, scale, run-to-completion, scheduling, per-node placement). No OwnerReference cascading deletion yet (deleting a Deployment doesn't delete its ReplicaSets/Pods) — the real controller-manager's `garbagecollector` controller would cover this, not yet enabled. |
| **Service networking**                   | `Service` objects get a real `ClusterIP` and correct `EndpointSlice`/`Endpoints` objects now, and kube-proxy is enabled and joins successfully, but actual traffic routing (`curl` a `ClusterIP` and reach the backing Pod) isn't proven end-to-end yet — see [`docs/general-purpose-k8s-plan.md`](docs/general-purpose-k8s-plan.md#phase-1--service-networking-object-model-done--verified-kube-proxy-enabled-real-traffic-routing-not-yet-proven-end-to-end).                      |
| **DNS**                                  | No CoreDNS/ClusterDNS. Confirmed via real kubelet logs during testing (`MissingClusterDNS: kubelet does not have ClusterDNS IP configured`) — Pods fall back to the node's own DNS policy, so Service/Pod name resolution inside the cluster doesn't work.                                                                                                                                                                                                                           |
| **Storage**                              | No PersistentVolume/PersistentVolumeClaim/StorageClass, no dynamic provisioning. Only `emptyDir`-style ephemeral storage works.                                                                                                                                                                                                                                                                                                                                                      |
| **Autoscaling**                          | No Metrics API (`metrics.k8s.io`), so no HorizontalPodAutoscaler/VerticalPodAutoscaler, and no Cluster Autoscaler equivalent.                                                                                                                                                                                                                                                                                                                                                        |
| **`kubectl logs` / `kubectl exec`**      | Off by default — requires the optional Cloudflare Tunnel + VPC Service setup below.                                                                                                                                                                                                                                                                                                                                                                                                  |
| **RBAC**                                 | The cluster token is all-or-nothing; there's no per-user/per-namespace authorization.                                                                                                                                                                                                                                                                                                                                                                                                |
| **Ingress / NetworkPolicy**              | Not implemented — no ingress controller, no network policy enforcement.                                                                                                                                                                                                                                                                                                                                                                                                              |
| **API compatibility details**            | No server-side apply, no protobuf wire format (JSON only), no dry-run. OpenAPI schema is served (`kubectl apply` no longer needs `--validate=false`), but there's no server-side strict field validation, so an unknown field is silently accepted rather than rejected the way a real cluster's `fieldValidation=Strict` would.                                                                                                                                                     |
| **Node self-healing**                    | No node lifecycle controller — if an agent's process dies, its last-reported `Ready` status is never corrected and Pods "on" it are never rescheduled.                                                                                                                                                                                                                                                                                                                               |

None of this is hidden complexity — see
[`docs/general-purpose-k8s-plan.md`](docs/general-purpose-k8s-plan.md) for the
dependency-ordered plan that closes every row above (measured by growing the
conformance CI's required focus set), and
[`docs/control-plane-architecture.md`](docs/control-plane-architecture.md) for
how each gap maps onto Cloudflare's execution model.

### Scheduling

Pods are bound by the actual, unmodified `k8s.io/kubernetes/cmd/kube-scheduler`
binary (`cmd/scheduler`) running against this apiserver — not a hand-written
subset. See
[`docs/control-plane-architecture.md`](docs/control-plane-architecture.md#migrating-to-the-real-kube-scheduler)
for how it's wired up. Rows below reflect gaps in what this apiserver stores
and serves, not scheduler limitations.

| Feature                                                            | Status                                                            |
| ------------------------------------------------------------------ | ----------------------------------------------------------------- |
| Node `Ready`/`unschedulable` filtering, `nodeSelector`, `hostPort` | ✅                                                                |
| CPU/memory/ephemeral-storage request vs. allocatable capacity      | ✅                                                                |
| Node/pod affinity & anti-affinity, taints/tolerations              | ✅                                                                |
| `PodScheduled` condition + `Scheduled`/`FailedScheduling` events   | ✅                                                                |
| LimitRange `Default`/`DefaultRequest`/Min-Max enforcement          | ✅                                                                |
| Priority via Pod's own `spec.priority`, preemption                 | ✅                                                                |
| Priority via a named `PriorityClass`                               | ❌ `scheduling.k8s.io` not registered, so the name never resolves |

### Controllers

| Controller                                                          | Status                                                                                                                                                                                                                                                                        |
| ------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Namespace cascading deletion                                        | ✅                                                                                                                                                                                                                                                                            |
| Default `ServiceAccount` auto-provisioning                          | ✅ (object only — no token/Secret issuance; nothing in this stack consumes SA tokens today)                                                                                                                                                                                   |
| Node lifecycle (lease-staleness → `Unknown` + taint + pod eviction) | ✅ Synchronous Go inside `apiserver` (`pkg/apiserver/nodelifecycle.go`), not `kube-controller-manager` — no BYO VM process needed for this one. Verified end-to-end: kill a real agent, node flips `Unknown`/tainted within ~48s, its Pods are deleted past the 5-minute mark |
| Endpoints / EndpointSlice (from Service + Pod selectors)            | ✅ Synchronous Go inside `apiserver` (`pkg/apiserver/endpoints.go`), not `kube-controller-manager` — no BYO VM process needed for this one either. ClusterIP allocation + both object types verified; real traffic routing via kube-proxy not yet proven end-to-end           |
| ReplicaSet / Deployment / Job / CronJob / DaemonSet                 | ✅ The real `kube-controller-manager` binary (`cmd/controller-manager`), not a hand-written reimplementation — verified end-to-end                                                                                                                                            |
| Garbage collection (owner references)                               | ❌ Not implemented — the real controller-manager has a generic `garbagecollector` controller for this, not yet enabled                                                                                                                                                        |

### Auth & admission

| Feature                                                 | Status                                                                                                               |
| ------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| Bearer token auth (static cluster token)                | ✅                                                                                                                   |
| RBAC                                                    | ❌ Not implemented — the static token is all-or-nothing                                                              |
| Admission webhooks                                      | ❌ Not implemented                                                                                                   |
| OpenAPI schema (`kubectl apply` client-side validation) | ✅ Served as Static Assets, verified with a real `kubectl apply` (no `--validate=false`) against a running dev stack |

## Node backends

Two ways to get Pods actually running, mix-and-match per cluster:

| Backend                                                        | Setup                                                                 | Image                                                              | Networking                                                                 |
| -------------------------------------------------------------- | --------------------------------------------------------------------- | ------------------------------------------------------------------ | -------------------------------------------------------------------------- |
| **BYO VM** (`cmd/agent`)                                       | Run the real, unmodified k3s agent yourself (EC2 or any Linux host)   | Any OCI image, pulled by containerd like any other Kubernetes node | Full — flannel/kube-proxy, `kubectl logs`/`exec` via the VPC kubelet proxy |
| **`workers/nodes`** (Pod-on-Containers, virtual-kubelet-style) | `wrangler deploy --config workers/nodes/wrangler.jsonc`, no VM to run | **Allowlisted only** — see below                                   | Limited — see below                                                        |

### `workers/nodes`: what it is and its hard limits

`workers/nodes` registers a virtual Node (`cf-containers-<pool>`), renews its
Lease, and runs each Pod scheduled to it as its own Cloudflare Containers
instance. It exists for clusters that want to try a Pod without standing up a
VM — it is not a general-purpose node backend, and the limits below are
platform constraints (`spikes/s3-containers/FINDINGS.md`), not gaps this
project intends to close later:

- **Image allowlist only, no arbitrary `kubectl run --image=...`.** Cloudflare
  Containers fixes a container's image (and its instance size) at Worker
  deploy time (`wrangler.jsonc`'s `containers[]`), not at Pod-run time — there
  is no API for a Worker to pick an arbitrary image at runtime. A Pod's
  `spec.containers[0].image` must exactly match one of `workers/nodes/src/
images.ts`'s `ALLOWED_IMAGES` (one demo image in v1,
  `workers/nodes/images/demo`); anything else is rejected (`status.phase =
Failed`). Adding a base image to the allowlist means adding a Dockerfile,
  a `containers[]`/`durable_objects` entry per size tier it should support,
  and a matching `PodContainer*` class — see `images.ts`'s header comment.
- **Size is rounded to one of three fixed tiers, not requested freely.** For
  the same deploy-time-fixed-instance-size reason, a Pod's
  `resources.requests` (summed across containers) is rounded up to the
  nearest of `small`/`medium`/`large` (Cloudflare Containers' `lite`/`basic`/
  `standard-1` named instance types: 1/16, 1/4, and 1/2 vCPU respectively). A
  Pod that asks for more than `large` provides, or that has anything other
  than exactly one container, is rejected the same way.
- **No UDP, inbound or outbound, at all** (official Cloudflare Containers
  limit, confirmed empirically not enforceable/observable in local `wrangler
dev` — see `spikes/s3-containers/FINDINGS.md` item 5). A Pod that needs UDP
  (DNS clients, QUIC, etc.) will not work on this backend. This also means
  **CoreDNS cannot be placed on a `workers/nodes`-backed node** — cluster DNS
  needs a UDP:53 listener, so CoreDNS deployments should target a BYO VM node
  (tracked as an open design question for clusters with no BYO VM node at
  all, see `docs/platform-verification.md`'s Phase 9 section).
- **No routable Pod IP, no `kubectl logs`/`exec`/`attach`.** A Pod's `status`
  reports `Running` with container statuses once its container starts, but
  does not get a real cluster-routable IP (Cloudflare Containers doesn't
  expose one to the hosting Durable Object) — Service traffic to a Pod on
  this backend is not proven end-to-end. `logs`/`exec`/`attach` are handled by
  `workers/gateway`'s kubelet proxy today, which dials a real kubelet over a
  VPC service binding (port 10255) — it has no branch for a virtual node, so
  these subresources don't work against a `workers/nodes`-backed Pod in v1.
- **Pod scheduling/deletion latency is ~10s, not sub-second.** `workers/nodes`
  discovers Pods bound to its node by listing them on its own Lease-renewal
  alarm (every ~10s) rather than via a push/watch — see `virtualnode.ts`'s
  header comment for why, and `docs/cost-model.md`'s "Phase 7 (nodes)
  implementation" section for the cost reasoning. Acceptable given
  Containers' own cold start is already 1–3s+.
- **`restartPolicy` is supported** (`Always`/`OnFailure`/`Never`), evaluated
  the same ~10s tick: a container that exited is restarted, left stopped, or
  marked `Succeeded`/`Failed` accordingly.

### Deploying `workers/nodes`

Separate from the 4-Worker `npm run deploy` above (it needs Docker/Containers
support, so it isn't bundled into the default dev/deploy flow):

```bash
npx wrangler secret put K3S_TOKEN --config workers/nodes/wrangler.jsonc
npx wrangler deploy --config workers/nodes/wrangler.jsonc

# One-time bootstrap: any request wakes VirtualNode's alarm loop for the
# first time (see workers/nodes/src/virtualnode.ts), registering the Node
# and starting its Lease-renewal/Pod-reconcile cycle.
curl https://k8flare-nodes.<your-subdomain>.workers.dev/healthz
```

`NODE_POOL` (defaults to `"default"`, registering `cf-containers-default`) can
be set as a Worker variable in `workers/nodes/wrangler.jsonc` to run more than
one pool — deploy a second copy of the Worker with a different `name` and
`NODE_POOL` for each.

## Roadmap

Two detailed plans drive the work from here:

- [`docs/general-purpose-k8s-plan.md`](docs/general-purpose-k8s-plan.md) —
  closing the gap table above in dependency order, with the conformance CI's
  required focus set as the definition of done.
- [`docs/multi-tenancy-and-hosting.md`](docs/multi-tenancy-and-hosting.md) —
  multi-cluster isolation, scale, and the hosted `k8flare.com` product.

### Track A — general-purpose Kubernetes

1. ~~**Real `kube-scheduler`**~~ — done. `cmd/scheduler` runs the actual,
   unmodified upstream binary against this apiserver, verified on the live
   cluster and by the sig-scheduling conformance suite. See
   [`docs/control-plane-architecture.md`](docs/control-plane-architecture.md#migrating-to-the-real-kube-scheduler)
   for the full trace, including the real bugs that only surfaced by running
   it (scheduler-informer coverage, a field-selector parsing bug).
2. **Service networking** (mostly done) — ~~ClusterIP allocation~~,
   ~~an EndpointSlice controller~~ (kube-proxy in v1.36 consumes
   EndpointSlices, not Endpoints), and ~~enabling the agent's embedded
   kube-proxy~~ are done and verified — a real kubelet + kube-proxy joins,
   reaches `Ready`, and gets correct `ClusterIP`/`EndpointSlice`/`Endpoints`
   objects for a real Service+Pod. What's left: proving actual traffic
   routing end-to-end (`curl` a `ClusterIP`, reach the backing Pod) — one
   attempt hit an unrelated, not-yet-root-caused watch-reliability issue
   before getting that far (see
   [`docs/general-purpose-k8s-plan.md`](docs/general-purpose-k8s-plan.md#phase-1--service-networking-object-model-done--verified-kube-proxy-enabled-real-traffic-routing-not-yet-proven-end-to-end)
   for the full trace, including a correction of an earlier, wrong
   conclusion that kube-proxy itself caused a Worker hang — it doesn't; that
   hang is a pre-existing CI-environment issue, confirmed to reproduce even
   on a bare `main` checkout). A Worker-side HTTP path for external Service
   exposure is still ahead too.
3. ~~**Node lifecycle**~~ — lease staleness → `Unknown` + taints → pod GC
   (`workers/storage/src/nodelifecycle.ts`), verified end-to-end by killing a
   real agent process: the node flipped `Unknown`/tainted within 48s and its
   Pod was deleted once staleness passed the 5-minute mark. Pod
   _recreation_ still needs the next item's workload controllers.
4. **Workload controllers + GC** — ~~ReplicaSet~~ → ~~Deployment~~ →
   ~~Job/CronJob~~ → ~~DaemonSet~~ → ownerReference cascading deletion; the
   largest single jump in official conformance coverage. ReplicaSet,
   Deployment, Job, CronJob, and DaemonSet are all done and verified
   end-to-end — backed by the **real, unmodified `kube-controller-manager`
   binary** (`cmd/controller-manager`), the same embed pattern as
   `cmd/scheduler`'s real kube-scheduler, not a hand-written
   reimplementation. `--controllers=replicaset,deployment,daemonset,job,cronjob`
   enables exactly this subset; `garbagecollector` (generic OwnerReference
   cascading deletion) and `statefulset` are already registered by the same
   binary but not yet enabled, pending their own verification pass.

   An earlier pass hand-wrote each of these five as a TypeScript reconciler
   in `workers/storage/src/*.ts`, individually verified end-to-end and
   documented in detail in
   [`docs/general-purpose-k8s-plan.md`](docs/general-purpose-k8s-plan.md#phase-3--workload-controllers--garbage-collection-self-healing-part-2)
   — but each was a simplified subset of real upstream semantics (no Indexed
   Jobs, no `PodFailurePolicy`, no Pod adoption, no `/scale`/`/status`
   subresources, no true proportional scaling, no generic
   OwnerReference GC). Once embedding the real controller-manager was
   confirmed feasible the same way the scheduler was, all five `.ts` files
   were deleted in favor of it. Getting the real binary working against this
   apiserver required real, concrete fixes — found by actually running it,
   not by reading its source first: `/status` subresources for all five
   types (real controllers call `UpdateStatus()`, which 404s without them);
   a `ControllerRevision` stub type (its absence hung DaemonSet's informer
   `WaitForCacheSync` forever, blocking every controller in the process);
   a missing `RESOURCE_KINDS` entry for five types, breaking their watch
   bookmarks exactly like the earlier EndpointSlice gap; the real upstream
   `apps/v1`/`batch/v1` admission defaulters registered onto `Scheme` (a
   Deployment with unset `spec.strategy.type` made the real controller
   hard-error forever, since real clients rely on apiserver-side defaulting
   to fill it in); `metadata.generateName` support in `Create()`; and Job's
   `spec.selector`/`controller-uid` label auto-generation (without it, the
   real Job controller endlessly disowned and replaced the Pods it had just
   created). Full trace of each in the plan doc.

5. **Cluster DNS** — CoreDNS as a Deployment, `kube-dns` Service at
   `10.43.0.10`, then (and only then) advertise `cluster-dns` to kubelets.
6. **API & auth parity** — ~~OpenAPI discovery (no more `--validate=false`)~~
   done (see [`docs/k8s-version-bump.md`](docs/k8s-version-bump.md)),
   PriorityClass, TokenRequest/TokenReview, RBAC.

### Track B — multi-tenancy, scale, and hosted `k8flare.com`

Target architecture (detailed in
[`docs/multi-tenancy-and-hosting.md`](docs/multi-tenancy-and-hosting.md)):
**one `Cluster` Durable Object per cluster (revision authority,
kine-compatible log), Durable Object Facets for per-namespace storage
isolation (`ns/<name>`, plus `events-log`/`ca-vault`), a `WatchHub` Durable
Object for watch fan-out** — writes serialize per cluster (the same
single-writer shape as etcd itself), and every namespace gets its own Facet
(its own SQLite DB, its own 10 GB).

> **Correction:** the original design here was "one follower Durable Object
> per namespace, applying writes forwarded from an ordering DO" (items 2-3
> below, as originally written). Durable Object Facets — opened to Open Beta
> after this plan was first written — replaced that with per-namespace
> storage inside the _same_ `Cluster` DO instead of a separate DO per
> namespace: simpler (no forward/apply/value-trimming machinery to write
> and maintain), at the cost of namespace reads also funneling through the
> parent DO's single dispatch thread rather than scaling out independently.
> See [`docs/multi-tenancy-and-hosting.md`](docs/multi-tenancy-and-hosting.md)
> for the full reasoning and trade-off. Items 2 and 3 are done, not
> upcoming — left in this numbered list so the roadmap's overall shape and
> history stay legible.

1. Cluster resolution + per-cluster tokens (retire the single hardcoded
   `"default"` cluster; zero-config single-cluster mode stays).
2. ~~Namespace follower DOs + value trimming~~ — superseded by Durable
   Object Facets (see correction above). Done: per-namespace storage
   (`ns/<name>` Facets) and `WatchHub` fan-out are both live.
3. ~~Durable Object Facets (open beta) for Event churn and CA-vault
   isolation~~ — done: `events-log` and `ca-vault` Facets are both live
   alongside the per-namespace ones.
4. `k8flare.com`: wildcard routing, provisioning API, metering → billing —
   a hosted control plane that costs ~nothing while idle because it scales
   to zero.
5. ~~Managed node pools on Cloudflare Containers~~ — done (v1): `workers/nodes`,
   a virtual-kubelet-style Pod backend, alongside BYO agents. See "Node
   backends" above for its image/size allowlist and networking limits (no
   UDP, no routable Pod IP, no `logs`/`exec` yet).

### Track C — Cloudflare-native surface

- **More compute backends** — `workers/nodes` (Cloudflare Containers) is the
  first one, alongside EC2/GCE/on-prem BYO VMs, mix-and-match per cluster;
  broadening its image allowlist and closing its networking gaps (Pod IP,
  `logs`/`exec`) remain open.
- **Cloudflare Mesh for cross-cloud node networking** — technically capable
  of replacing "all nodes in one VPC" for flannel's `host-gw` backend (true
  L3 routing, CIDR route advertisement, scriptable enrollment), but not a
  clean drop-in: every packet detours through a Cloudflare PoP (no direct
  peer-to-peer path), with real throughput cost and UDP-loss tradeoffs, and
  no existing Kubernetes integration or case study. See
  [`docs/cloudflare-mesh-networking.md`](docs/cloudflare-mesh-networking.md)
  for the full evaluation — recommended as a lower-throughput/dev-test/
  geographically-dispersed option, not the default for performance-sensitive
  clusters.
- **`k8f` CLI** — standalone OAuth 2.0 PKCE login (independent of
  Cloudflare's `cf` CLI): log in, provision, get a kubeconfig in one
  command.
- **Cloudflare Access integration** — connectivity and identity for the API,
  mapped onto RBAC once Track A lands it.
- **Worker / Durable Object / DynamicWorker / Facets as Kubernetes
  resources** — manage real Cloudflare primitives with `kubectl`.

## Quick Start

### From Release

```bash
# Download and extract the Workers bundle (gateway/apiserver/storage/runtime --
# workers/controllers isn't included, see Architecture above)
gh release download -R k8flare/k8flare -p 'k8flare-workers-*.tar.gz'
tar xzf k8flare-workers-*.tar.gz
cd k8flare-workers
pnpm install

# Set your cluster token on every Worker that checks it (apiserver, gateway, runtime)
for w in apiserver gateway runtime; do
  npx wrangler secret put K3S_TOKEN --config "workers/$w/wrangler.jsonc"
done

# Deploy all 4 (order matters -- see package.json's "deploy" script)
npm run deploy
```

### From Source

```bash
git clone https://github.com/k8flare/k8flare.git
cd k8flare
pnpm install  # plain `npm install` fails here -- package.json uses pnpm workspace:* deps

# Build Go WASM (needs a local Go toolchain; skip this and use the release
# tarball above if you'd rather not install one)
npm run build:wasm:apiserver

# Deploy
npm run deploy
```

### Agent, Scheduler, and Controller Manager Setup

A cluster needs three things running outside the Workers above, on your own
VM (EC2 or any Linux host) — the agent for kubelet/containerd, and the real
upstream scheduler/controller-manager binaries for Pod placement and
workload controllers (ReplicaSet/Deployment/Job/CronJob/DaemonSet). See
Architecture above for why these aren't Workers.

```bash
# Download the binaries
gh release download -R k8flare/k8flare -p 'k8flare-agent-*' -p 'k8flare-scheduler-*' -p 'k8flare-controller-manager-*'
chmod +x k8flare-agent-linux-* k8flare-scheduler-linux-* k8flare-controller-manager-linux-*

# Run all three against the same Worker deployment and token
./k8flare-agent-linux-arm64 \
  --server https://your-k8flare-gateway.workers.dev \
  --token YOUR_K3S_TOKEN &

./k8flare-scheduler-linux-arm64 \
  --server https://your-k8flare-gateway.workers.dev \
  --token YOUR_K3S_TOKEN &

./k8flare-controller-manager-linux-arm64 \
  --server https://your-k8flare-gateway.workers.dev \
  --token YOUR_K3S_TOKEN &
```

See [scripts/ec2-user-data.sh](scripts/ec2-user-data.sh) for automated EC2 bootstrap (currently automates the agent only).

## Configuration

### Worker Environment Variables

| Variable    | Required | Description                                                        |
| ----------- | -------- | ------------------------------------------------------------------ |
| `K3S_TOKEN` | Yes      | Cluster authentication token. Generate with `openssl rand -hex 32` |

### Optional: VPC Service (for kubelet proxy)

To enable `kubectl logs` and `kubectl exec`, set up a Cloudflare Tunnel + VPC Service:

```bash
./scripts/setup-tunnel.sh
```

Then add the VPC binding to `workers/gateway/wrangler.jsonc`:

```jsonc
"vpc_services": [{ "binding": "KUBELET_VPC", "service_id": "YOUR_SERVICE_ID" }]
```

## Development

```bash
pnpm install           # plain `npm install` fails -- package.json uses pnpm workspace:* deps
npm run build:wasm    # Build Go WASM binaries (apiserver + controllers)
npm run dev           # Start local dev server (all 4 deployable Workers, multi-config)
npm run check         # Lint + format + typecheck (vp)
npm run test          # Run tests
go test ./pkg/...     # Run Go tests
```

## License

MIT
