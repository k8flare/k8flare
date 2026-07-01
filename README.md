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

| Resource                                                                                              | Status                                                                                 |
| ----------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| Namespace, ConfigMap, Secret, Pod, Node, ServiceAccount, Endpoints, Service, Event, Lease, LimitRange | ✅ CRUD, watch, label/field selectors                                                  |
| CSIDriver, CSINode, RuntimeClass                                                                      | ✅ CRUD, watch                                                                         |
| `DynamicWorker`, `WorkerTrigger` (custom resources)                                                   | ✅ CRUD, watch                                                                         |
| Deployment, ReplicaSet, StatefulSet, DaemonSet, Job, CronJob                                          | ❌ Not registered — no workload controllers exist yet                                  |
| PersistentVolume, PersistentVolumeClaim, StorageClass                                                 | ❌ Not implemented                                                                     |
| Generic `CustomResourceDefinition` (dynamic CRDs)                                                     | ❌ Only the two built-in custom resources above; no generic CRD registration mechanism |

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
it (workload controllers and a real kube-scheduler/controller-manager are the
next planned pieces).

### Scheduling

| Feature                                                          | Status      |
| ---------------------------------------------------------------- | ----------- |
| Node `Ready`/`unschedulable` filtering                           | ✅          |
| `nodeSelector`                                                   | ✅          |
| CPU request vs. allocatable capacity                             | ✅          |
| `hostPort` conflict detection                                    | ✅          |
| `PodScheduled` condition + `Scheduled`/`FailedScheduling` events | ✅          |
| LimitRange `Default`/`DefaultRequest`/Min-Max enforcement        | ✅          |
| Memory / ephemeral-storage aware scheduling                      | ❌ CPU only |
| Node/pod affinity & anti-affinity, taints/tolerations            | ❌          |
| Priority & preemption                                            | ❌          |

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
