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

| Resource                                                                                                       | Status                                                                                                                                    |
| -------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| Namespace, ConfigMap, Secret, Pod, Node, ServiceAccount, Endpoints, Service, Event, Lease, LimitRange          | ✅ CRUD, watch, label/field selectors                                                                                                     |
| CSIDriver, CSINode, RuntimeClass                                                                               | ✅ CRUD, watch                                                                                                                            |
| `DynamicWorker`, `WorkerTrigger` (custom resources)                                                            | ✅ CRUD, watch                                                                                                                            |
| ReplicaSet, StatefulSet, ReplicationController, PodDisruptionBudget, ResourceClaim, ResourceSlice, DeviceClass | ⚠️ Registered but always empty — exist only so the real kube-scheduler's informers for these types can sync; not backed by any controller |
| Deployment, DaemonSet, Job, CronJob                                                                            | ❌ Not registered — no workload controllers exist yet                                                                                     |
| PersistentVolume, PersistentVolumeClaim, StorageClass                                                          | ❌ Not implemented                                                                                                                        |
| Generic `CustomResourceDefinition` (dynamic CRDs)                                                              | ❌ Only the two built-in custom resources above; no generic CRD registration mechanism                                                    |

### What's missing for general-purpose use

The table above covers the API surface; this is about whether a typical
workload actually _runs_ the way it would on a normal cluster. These are the
gaps that matter most for everyday use, roughly in the order most users would
hit them:

| Area                                     | Gap                                                                                                                                                                                                                                                        |
| ---------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Deploying anything beyond a bare Pod** | No Deployment/ReplicaSet/DaemonSet/Job/CronJob — every Pod has to be created directly and re-created by hand if it dies. This is the single biggest gap to "feels like normal Kubernetes."                                                                 |
| **Service networking**                   | `Service` objects can be created, but with no Endpoints controller and no kube-proxy, nothing actually load-balances or routes traffic to them — a `ClusterIP` Service doesn't work as one today.                                                          |
| **DNS**                                  | No CoreDNS/ClusterDNS. Confirmed via real kubelet logs during testing (`MissingClusterDNS: kubelet does not have ClusterDNS IP configured`) — Pods fall back to the node's own DNS policy, so Service/Pod name resolution inside the cluster doesn't work. |
| **Storage**                              | No PersistentVolume/PersistentVolumeClaim/StorageClass, no dynamic provisioning. Only `emptyDir`-style ephemeral storage works.                                                                                                                            |
| **Autoscaling**                          | No Metrics API (`metrics.k8s.io`), so no HorizontalPodAutoscaler/VerticalPodAutoscaler, and no Cluster Autoscaler equivalent.                                                                                                                              |
| **`kubectl logs` / `kubectl exec`**      | Off by default — requires the optional Cloudflare Tunnel + VPC Service setup below.                                                                                                                                                                        |
| **RBAC**                                 | The cluster token is all-or-nothing; there's no per-user/per-namespace authorization.                                                                                                                                                                      |
| **Ingress / NetworkPolicy**              | Not implemented — no ingress controller, no network policy enforcement.                                                                                                                                                                                    |
| **API compatibility details**            | No server-side apply, no protobuf wire format (JSON only), no OpenAPI schema (`kubectl apply` needs `--validate=false`), no dry-run.                                                                                                                       |
| **Node self-healing**                    | No node lifecycle controller — if an agent's process dies, its last-reported `Ready` status is never corrected and Pods "on" it are never rescheduled.                                                                                                     |

None of this is hidden complexity — see
[`docs/control-plane-architecture.md`](docs/control-plane-architecture.md) for
how each gap maps onto Cloudflare's execution model and the plan for closing
it (an Endpoints controller and the rest of a controller-manager are the next
planned pieces — the real kube-scheduler is already done, see the Roadmap
below).

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

| Controller                                        | Status                                                                                      |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| Namespace cascading deletion                      | ✅                                                                                          |
| Default `ServiceAccount` auto-provisioning        | ✅ (object only — no token/Secret issuance; nothing in this stack consumes SA tokens today) |
| Node lifecycle (lease-staleness → `NotReady`)     | ❌ Planned, not yet implemented — a dead agent's last-reported status is never corrected    |
| Endpoints (from Service + Pod selectors)          | ❌ Not implemented                                                                          |
| Workload controllers (ReplicaSet/Deployment/etc.) | ❌ Blocked on the missing workload types above                                              |
| Garbage collection (owner references)             | ❌ Not implemented                                                                          |

### Auth & admission

| Feature                                                 | Status                                                  |
| ------------------------------------------------------- | ------------------------------------------------------- |
| Bearer token auth (static cluster token)                | ✅                                                      |
| RBAC                                                    | ❌ Not implemented — the static token is all-or-nothing |
| Admission webhooks                                      | ❌ Not implemented                                      |
| OpenAPI schema (`kubectl apply` client-side validation) | ❌ Use `--validate=false`                               |

## Roadmap

Beyond closing the gaps above, these larger initiatives are planned, roughly
in this order:

### 1. ~~Real `kube-scheduler` instead of the built-in scheduler~~ — done

`cmd/scheduler` runs the actual, unmodified
`k8s.io/kubernetes/cmd/kube-scheduler` binary against this apiserver — the
same embedding pattern k3s itself uses for kubelet, authenticated via the
existing shared-token middleware (no client-cert bootstrap needed), with
leader election turned off (`--leader-elect=false`, the same flag k3s uses).
All required Kubernetes modules were already indirect dependencies via k3s,
so no new dependency versions were needed. Verified against the live
cluster: a real Pod gets bound with a `Scheduled` event and runs, and the
official sig-scheduling conformance suite passes end-to-end. See
[`docs/control-plane-architecture.md`](docs/control-plane-architecture.md#migrating-to-the-real-kube-scheduler)
for the full trace, including two real bugs (informers for types beyond
`ResourceClaim`/`ResourceSlice`, and a field-selector parsing bug) that only
surfaced once a real scheduler was actually run against a live cluster
rather than reasoned about from source.

### 2. Endpoints controller and Service HTTP exposure (next)

A real Endpoints controller (Service + Pod label selectors → Endpoints),
paired with a Worker-side HTTP path that resolves a Service to one of its
backing Pod IPs, so `ClusterIP` Services actually route traffic and can be
reached over HTTP through the Worker. This is the most-hit gap in the table
above and unblocks anything that depends on in-cluster service discovery
working end-to-end.

### 3. More compute backends: Cloudflare Containers and Cloudflare Mesh

Both land as additional node backends alongside the existing EC2/GCE/on-prem
agent setup — parallel options, not replacements. A cluster will be able to
mix and match:

- **Cloudflare Containers** (GA since April 2026) — run the k3s agent
  (kubelet + containerd) inside a Cloudflare Container instead of a VM, so a
  node can exist with no external cloud account at all.
- **Cloudflare Mesh** — bridge AWS EC2, GCP, and on-prem machines into one
  private network so they can join as nodes without the manual VPC/security
  group setup the current EC2 script needs.

### 4. `k8f` CLI

A standalone CLI with its own OAuth 2.0 Authorization Code + PKCE login flow
(in the spirit of `wrangler login`, but independent of Cloudflare's own `cf`
CLI) that logs in, provisions the Worker + Durable Object, and writes a
working kubeconfig in one command.

### 5. Deeper Cloudflare platform integration

- **Cloudflare Access** — native integration for both connectivity (reaching
  the API/exec/logs endpoints) and RBAC (mapping Access identities/groups to
  per-user/per-namespace authorization), replacing today's all-or-nothing
  static token.
- **Worker / Durable Object / DynamicWorker / Durable Object Facets as
  Kubernetes resources** — both a user-facing resource kind, so `kubectl` can
  create and manage real Workers and Durable Objects directly, and internal
  use of Facets to give each namespace/tenant its own isolated child Durable
  Object instead of sharing the single global `Etcd` instance.

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
