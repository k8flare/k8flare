# k8flare

Kubernetes control plane on Cloudflare Workers.

Runs a minimal K8s API server as a Cloudflare Worker with Durable Objects (SQLite-backed etcd-like storage), enabling `kubectl` to manage workloads without traditional infrastructure.

## Architecture

```
kubectl → Cloudflare Worker (Go WASM + TypeScript)
              ↓
         Durable Object (Etcd) — SQLite-backed K8s state store
              ↓
         k3s Agent (EC2) — kubelet + containerd + flannel
```

**Components:**

- **Worker** — TypeScript routing layer + Go WASM K8s API server
- **Etcd** — Durable Object with SQLite, implements kine-compatible storage
- **Agent** — k3s agent binary with Cloudflare-specific adaptations (CA replacement, token auth, flannel bypass)

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
| **API compatibility details**            | No server-side apply, no protobuf wire format (JSON only), no OpenAPI schema (`kubectl apply` needs `--validate=false`), no dry-run.                                                                                                                                                                                                                                                                                                                                                 |
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

| Controller                                                          | Status                                                                                                                             |
| ------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| Namespace cascading deletion                                        | ✅                                                                                                                                 |
| Default `ServiceAccount` auto-provisioning                          | ✅ (object only — no token/Secret issuance; nothing in this stack consumes SA tokens today)                                        |
| Node lifecycle (lease-staleness → `Unknown` + taint + pod eviction) | ✅ Verified end-to-end: kill a real agent, node flips `Unknown`/tainted within ~48s, its Pods are deleted past the 5-minute mark   |
| Endpoints / EndpointSlice (from Service + Pod selectors)            | ✅ ClusterIP allocation + both object types verified; real traffic routing via kube-proxy not yet proven end-to-end                |
| ReplicaSet / Deployment / Job / CronJob / DaemonSet                 | ✅ The real `kube-controller-manager` binary (`cmd/controller-manager`), not a hand-written reimplementation — verified end-to-end |
| Garbage collection (owner references)                               | ❌ Not implemented — the real controller-manager has a generic `garbagecollector` controller for this, not yet enabled             |

### Auth & admission

| Feature                                                 | Status                                                  |
| ------------------------------------------------------- | ------------------------------------------------------- |
| Bearer token auth (static cluster token)                | ✅                                                      |
| RBAC                                                    | ❌ Not implemented — the static token is all-or-nothing |
| Admission webhooks                                      | ❌ Not implemented                                      |
| OpenAPI schema (`kubectl apply` client-side validation) | ❌ Use `--validate=false`                               |

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
   (`packages/etcd/src/nodelifecycle.ts`), verified end-to-end by killing a
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
   in `packages/etcd/src/*.ts`, individually verified end-to-end and
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
6. **API & auth parity** — OpenAPI discovery (no more `--validate=false`),
   PriorityClass, TokenRequest/TokenReview, RBAC.

### Track B — multi-tenancy, scale, and hosted `k8flare.com`

Target architecture (decided July 2026, detailed in
[`docs/multi-tenancy-and-hosting.md`](docs/multi-tenancy-and-hosting.md)):
**one ordering Durable Object per cluster, one follower Durable Object per
namespace, Facets for churn/CA isolation, WatchHub DOs for fan-out** —
writes serialize per cluster (the same single-writer shape as etcd itself),
reads/watchers/storage scale horizontally, and every namespace gets its own
10 GB.

1. Cluster resolution + per-cluster tokens (retire the single hardcoded
   `"default"` cluster; zero-config single-cluster mode stays).
2. Namespace follower DOs + value trimming (per-namespace 10 GB) and
   WatchHub fan-out.
3. Durable Object Facets (open beta) for Event churn and CA-vault isolation.
4. `k8flare.com`: wildcard routing, provisioning API, metering → billing —
   a hosted control plane that costs ~nothing while idle because it scales
   to zero.
5. Managed node pools on Cloudflare Containers (4 vCPU / 12 GiB per node,
   scale-to-zero), alongside BYO agents.

### Track C — Cloudflare-native surface

- **More compute backends** — Cloudflare Containers as an additional node
  backend alongside EC2/GCE/on-prem, mix-and-match per cluster.
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
# Download the latest release
gh release download -R k8flare/k8flare -p 'k8flare-worker-*.tar.gz'
tar xzf k8flare-worker-*.tar.gz
cd k8flare-worker

# Set your cluster token
npx wrangler secret put K3S_TOKEN

# Deploy
npx wrangler deploy
```

### From Source

```bash
git clone https://github.com/k8flare/k8flare.git
cd k8flare
npm install

# Build Go WASM
npm run build:wasm

# Deploy
npm run deploy
```

### Agent Setup

Deploy the k3s agent on an EC2 instance (or any Linux server):

```bash
# Download agent binary
gh release download -R k8flare/k8flare -p 'k8flare-agent-*'
chmod +x k8flare-agent-linux-*

# Run agent
./k8flare-agent-linux-arm64 \
  --server https://your-k8flare.workers.dev \
  --token YOUR_K3S_TOKEN
```

See [scripts/ec2-user-data.sh](scripts/ec2-user-data.sh) for automated EC2 bootstrap.

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

Then add the VPC binding to `packages/worker/wrangler.jsonc`:

```jsonc
"vpc_services": [{ "binding": "KUBELET_VPC", "service_id": "YOUR_SERVICE_ID" }]
```

## Development

```bash
npm install
npm run build:wasm    # Build Go WASM binary
npm run dev           # Start local dev server
npm run check         # Lint + format + typecheck (vp)
npm run test          # Run tests
go test ./pkg/...     # Run Go tests
```

## License

MIT
