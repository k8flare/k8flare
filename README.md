# k8flare

Kubernetes control plane on Cloudflare Workers.

Runs a minimal K8s API server as a Cloudflare Worker with Durable Objects (SQLite-backed etcd-like storage), enabling `kubectl` to manage workloads without traditional infrastructure.

## Architecture

```
kubectl / kubelet (BYO VM: cmd/agent, an unmodified k3s agent embed)
   │ HTTPS + token
   ▼
k8flare — ONE Cloudflare Worker (workers/k8flare, the single deploy unit)
   public fetch: auth, watch streaming, kubelet proxy (VPC), /nodes/* ops surface
   ├──► apiserver (real Go/WASM, table-driven from real k8s.io/kubernetes types) —
   │      shipped as Static Assets chunks, run as a Worker Loader dynamic worker
   ├──► runtime/ (TypeScript: CRDs, DynamicWorker/WorkerTrigger; user code via Loader)
   ├─ Cluster DO — revision authority, kine-compatible log, per-namespace Facets
   ├─ WatchHub DO — watch fan-out over hibernating WebSockets
   ├─ Controllers DO — hosts the real kube-controller-manager (Go/WASM) as a
   │      Loader dynamic worker (same Assets-chunks supply channel)
   └─ CFContainersScheduler + NodeVM DOs (Cloudflare Containers) — optional
        per-Pod microVM node backend
```

**One Worker, one `wrangler.jsonc`** (`workers/k8flare`). The former
6-Worker split (gateway/apiserver/storage/runtime/controllers/nodes)
was consolidated 2026-07-06 — same components, now subtrees of one
script (`workers/k8flare/src/{gateway,storage,runtime,controllers,nodes}`),
with the cross-Worker service bindings replaced by direct calls and the
Go binaries loaded through the Worker Loader (see
[`docs/platform-verification.md`](docs/platform-verification.md)'s S19
section for the feasibility gates):

- **gateway/** — the public routing: authentication, watch streaming, kubelet proxying
- **apiserver** (`cmd/apiserver-wasm` + `pkg/apiserver`) — Go compiled to WASM; a table-driven API server built from real `k8s.io/kubernetes` types, not a hand-rolled subset of the wire format. Ships as ≤24MiB Static Assets chunks and runs as a Loader dynamic worker (no more 10MiB-gzip script budget).
- **storage/** — the `Cluster` Durable Object (kine-compatible revision log, per-namespace storage via Durable Object Facets) and the `WatchHub` Durable Object (watch fan-out, hibernating WebSockets)
- **runtime/** — CRDs, `DynamicWorker`/`WorkerTrigger` custom resources
- **nodes/** — optional virtual-kubelet-style Pod backend on Cloudflare Containers, for clusters that don't want to run a BYO VM agent just to try a Pod. See "Node backends" below.

**`cmd/agent`** is an unmodified k3s agent (kubelet + containerd + flannel) you run yourself (EC2 or any Linux host) — see Agent Setup below.

**Pod scheduling needs `cmd/scheduler` running too**, alongside the
agent — the real, unmodified upstream `kube-scheduler` binary. It runs
as a host process, not inside a Worker: `kube-scheduler` doesn't compile
for Cloudflare's Go/WASM target at all. See
[`docs/platform-verification.md`](docs/platform-verification.md)'s S8
section for the full investigation.

**Workload controllers run inside the Worker**: the real, unmodified
`kube-controller-manager` compiled to Go/WASM (`cmd/kcm-wasm`), served
from Static Assets in ≤24MiB chunks and loaded at runtime through the
Worker Loader (which imposes its own 64MiB cap, met via `wasm-opt`; see
platform-verification.md's S14 section). Endpoints/EndpointSlice
generation and Node lifecycle don't need it, though: those run as
synchronous Go inside the apiserver itself. See the Controllers table
below.

## Kubernetes API Support

k8flare implements a subset of the Kubernetes API, not a full distribution. This
reflects what's actually verified against the official
[`sig-scheduling`/`sig-api-machinery` conformance suite](https://github.com/kubernetes/kubernetes/tree/master/test/e2e)
as of this writing, plus direct inspection of the registered API scheme.

### Core resources

| Resource                                                                                                               | Status                                                                                                                                                                                                                                                                                                                                                      |
| ---------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Namespace, ConfigMap, Secret, Pod, Node, ServiceAccount, Endpoints, Service, Event, Lease, LimitRange                  | ✅ CRUD, watch, label/field selectors                                                                                                                                                                                                                                                                                                                       |
| CSIDriver, CSINode, RuntimeClass                                                                                       | ✅ CRUD, watch                                                                                                                                                                                                                                                                                                                                              |
| `DynamicWorker`, `WorkerTrigger` (custom resources)                                                                    | ✅ CRUD, watch                                                                                                                                                                                                                                                                                                                                              |
| ReplicaSet, Deployment, Job, CronJob, DaemonSet                                                                        | ✅ Backed by the real, unmodified `kube-controller-manager` binary (`cmd/controller-manager`, same embed pattern as `cmd/scheduler`), verified end-to-end. Deployment and ReplicaSet also support the `scale` subresource (`kubectl scale deployment --replicas=N` works, verified against real `client-go`/`kubectl`, not just this apiserver's own tests) |
| StatefulSet, ReplicationController, PodDisruptionBudget, ResourceClaim, ResourceSlice, DeviceClass, ControllerRevision | ⚠️ Registered but always empty — exist only so the real kube-scheduler's/kube-controller-manager's informers for these types can sync; not backed by any controller. StatefulSet additionally has `/status` and `/scale` subresource support (same as Deployment/ReplicaSet above) even though nothing populates it                                         |
| PersistentVolume, PersistentVolumeClaim, StorageClass                                                                  | ✅ CRUD, watch. R2-backed dynamic provisioning (single `r2` `StorageClass`) — see [Volumes (R2 PV/PVC)](#volumes-r2-pvpvc) below for what's actually provided                                                                                                                                                                                               |
| Role, RoleBinding, ClusterRole, ClusterRoleBinding (`rbac.authorization.k8s.io/v1`)                                    | ✅ CRUD, watch (`kubectl apply -f role.yaml`, `kubectl get roles` work) — **objects only**, not consulted for any access decision; see [Auth & admission](#auth--admission) below                                                                                                                                                                           |
| ResourceQuota                                                                                                          | ✅ CRUD, watch, `/status` subresource — **not enforced**: nothing tracks usage against `spec.hard`, so a Pod that would exceed quota is still admitted                                                                                                                                                                                                      |
| Ingress, IngressClass, NetworkPolicy (`networking.k8s.io/v1`)                                                          | ✅ CRUD, watch (Ingress also has `/status`) — **objects only**, no ingress controller and no network policy enforcement (see [What's missing](#whats-missing-for-general-purpose-use) below)                                                                                                                                                                |
| HorizontalPodAutoscaler (`autoscaling/v2`)                                                                             | ✅ CRUD, watch, `/status` subresource — **objects only**, no Metrics API/controller drives it (see [Autoscaling](#whats-missing-for-general-purpose-use) below)                                                                                                                                                                                             |
| Generic `CustomResourceDefinition` (dynamic CRDs)                                                                      | ❌ Only the two built-in custom resources above; no generic CRD registration mechanism                                                                                                                                                                                                                                                                      |

`kubectl get` also gets the real human-readable column output (not just `NAME`/`AGE`) for
Pod, Node, Deployment, ReplicaSet, Service, and Namespace — verified against real `kubectl`,
which requests this via `Accept: application/json;as=Table;v=v1;g=meta.k8s.io`. Every other
resource falls back to `NAME`/`AGE` columns, the same fallback real `kube-apiserver` uses for
a type with no registered additional printer columns. `kubectl auth can-i` also works end to
end (`SelfSubjectAccessReview`, `authorization.k8s.io/v1`) — it always answers `allowed: true`,
which is the accurate answer for this project's all-or-nothing bearer token (see
[Auth & admission](#auth--admission) below), not a real per-verb/per-resource evaluation.

### What's missing for general-purpose use

The table above covers the API surface; this is about whether a typical
workload actually _runs_ the way it would on a normal cluster. These are the
gaps that matter most for everyday use, roughly in the order most users would
hit them:

| Area                                     | Gap                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| ---------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Deploying anything beyond a bare Pod** | ReplicaSet, Deployment, Job, CronJob, and DaemonSet all work, backed by the real `kube-controller-manager` (`cmd/controller-manager`) rather than a reimplementation — verified end-to-end (create, rolling update, scale, run-to-completion, scheduling, per-node placement). OwnerReference cascading deletion (deleting a Deployment also deletes its ReplicaSets/Pods) works via a temporary apiserver-side substitute (`pkg/apiserver/gc.go`), not the real controller-manager's `garbagecollector` controller — see [`docs/general-purpose-k8s-plan.md`](docs/general-purpose-k8s-plan.md) for why the real one isn't embedded yet.                                                                                                                                                                                                                                                 |
| **Service networking**                   | `Service` objects get a real `ClusterIP` and correct `EndpointSlice`/`Endpoints` objects now, and kube-proxy is enabled and joins successfully, but actual traffic routing (`curl` a `ClusterIP` and reach the backing Pod) isn't proven end-to-end yet — see [`docs/general-purpose-k8s-plan.md`](docs/general-purpose-k8s-plan.md#phase-1--service-networking-object-model-done--verified-kube-proxy-enabled-real-traffic-routing-not-yet-proven-end-to-end).                                                                                                                                                                                                                                                                                                                                                                                                                           |
| **DNS**                                  | `<service>.<namespace>.svc.cluster.local` resolves via a node-local shim (`pkg/dnsshim`, started by `cmd/agent`, no Containers/CoreDNS Deployment) forwarding to a DNS-over-HTTPS synthesis endpoint (`pkg/apiserver`'s `/dns-query`) that reads live Service/EndpointSlice objects — including headless Services resolving to backing Pod IPs. `kubectl` traffic and the Go apiserver's own test suite verify the A-record synthesis directly; end-to-end resolution from inside a real Pod additionally needs the "Service networking" row above's real ClusterIP traffic proof (kube-proxy actually routing packets), which is still open. Pod hostname/subdomain records and SRV records aren't implemented yet.                                                                                                                                                                      |
| **Storage**                              | R2-backed dynamic provisioning exists (single `r2` `StorageClass`, see [Volumes (R2 PV/PVC)](#volumes-r2-pvpvc)), but it's S3-API access via injected env vars, not a real mounted filesystem — an app expecting a POSIX path at its `volumeMounts[].mountPath` won't find one on `workers/nodes` in v1. `emptyDir`-style ephemeral storage also works.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| **Autoscaling**                          | `HorizontalPodAutoscaler` (`autoscaling/v2`) objects are CRUD/watch-registered (`kubectl get hpa`, `kubectl apply -f hpa.yaml` work) but no Metrics API (`metrics.k8s.io`) exists and nothing reconciles them — creating an HPA does not scale anything. No VerticalPodAutoscaler or Cluster Autoscaler equivalent either.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| **`kubectl logs` / `kubectl exec`**      | Off by default — requires joining the BYO VM node to Cloudflare Mesh (recommended, see below) or the legacy Cloudflare Tunnel + VPC Service setup, both optional.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| **RBAC**                                 | Enforced by the real upstream `RBACAuthorizer` + real bootstrap policy, evaluating live Role/RoleBinding/ClusterRole/ClusterRoleBinding objects (`kubectl auth can-i` answers from the same authorizer). The cluster token itself maps to `system:masters` and bypasses RBAC (upstream's own SystemPrivilegedGroup behavior). Non-admin identities: the `X-Remote-User` TLS-proxy path, and real ServiceAccount tokens (`TokenRequest`, signed/validated with upstream's own JWT code) — `kubectl create token`, kubelet's own projected-volume tokens, and anything else following the standard client-go flow all work. Deviations (bootstrap policy unioned at read, pre-1.8 `system:nodes` group bindings in place of the Node authorizer, no OIDC discovery document backing the token issuer) are documented in `pkg/apiserver/rbac.go` and `pkg/apiserver/serviceaccounttoken.go`. |
| **Ingress / NetworkPolicy**              | Ingress/IngressClass/NetworkPolicy (`networking.k8s.io/v1`) objects are CRUD/watch-registered (`kubectl apply -f ingress.yaml` works) but nothing reads them — no ingress controller, no network policy enforcement. (The project's own view is that the Worker itself should be the ingress — see Track A/C in Roadmap below — not that a per-cluster ingress controller is coming.)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| **API compatibility details**            | No server-side apply, no protobuf wire format (JSON only), no dry-run. OpenAPI schema is served (`kubectl apply` no longer needs `--validate=false`), but there's no server-side strict field validation, so an unknown field is silently accepted rather than rejected the way a real cluster's `fieldValidation=Strict` would.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| **Node self-healing**                    | No node lifecycle controller — if an agent's process dies, its last-reported `Ready` status is never corrected and Pods "on" it are never rescheduled.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |

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

| Controller                                                          | Status                                                                                                                                                                                                                                                                                                                                                                                                   |
| ------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Namespace cascading deletion                                        | ✅                                                                                                                                                                                                                                                                                                                                                                                                       |
| Default `ServiceAccount` auto-provisioning                          | ✅ (object only — no token/Secret issuance; nothing in this stack consumes SA tokens today)                                                                                                                                                                                                                                                                                                              |
| Node lifecycle (lease-staleness → `Unknown` + taint + pod eviction) | ✅ Synchronous Go inside `apiserver` (`pkg/apiserver/nodelifecycle.go`), not `kube-controller-manager` — no BYO VM process needed for this one. Verified end-to-end: kill a real agent, node flips `Unknown`/tainted within ~48s, its Pods are deleted past the 5-minute mark                                                                                                                            |
| Endpoints / EndpointSlice (from Service + Pod selectors)            | ✅ Synchronous Go inside `apiserver` (`pkg/apiserver/endpoints.go`), not `kube-controller-manager` — no BYO VM process needed for this one either. ClusterIP allocation + both object types verified; real traffic routing via kube-proxy not yet proven end-to-end                                                                                                                                      |
| ReplicaSet / Deployment / Job / CronJob / DaemonSet                 | ✅ The real `kube-controller-manager` binary (`cmd/controller-manager`), not a hand-written reimplementation — verified end-to-end                                                                                                                                                                                                                                                                       |
| Garbage collection (owner references)                               | ⚠️ Synchronous Go inside `apiserver` (`pkg/apiserver/gc.go`), not `kube-controller-manager`'s real `garbagecollector` controller — a temporary substitute pending a PartialObjectMetadata client + RESTMapper in `pkg/leanclient` (see `docs/general-purpose-k8s-plan.md`). Covers `kubectl delete` cascading through ownerReferences and `propagationPolicy: Orphan`, not cluster-scoped owners or CRDs |

### Auth & admission

| Feature                                                        | Status                                                                                                                                                                                                                                                                                                                                                              |
| -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Bearer token auth (static cluster token)                       | ✅                                                                                                                                                                                                                                                                                                                                                                  |
| RBAC objects (Role/RoleBinding/ClusterRole/ClusterRoleBinding) | ✅ CRUD/watch, and consulted for real access decisions (below)                                                                                                                                                                                                                                                                                                      |
| RBAC enforcement (an authorizer in front of the stores)        | ✅ The real upstream `RBACAuthorizer` + bootstrap policy (`pkg/apiserver/rbac.go`); cluster token = `system:masters` bypass, exactly like upstream. Watch streams (served in TS) enforce the same policy via SubjectAccessReview                                                                                                                                    |
| ServiceAccount tokens (`TokenRequest`)                         | ✅ Real upstream JWT signing/validation (`pkg/apiserver/serviceaccounttoken.go`) -- `kubectl create token`, kubelet's own projected-volume tokens, and any client-go `CreateToken` caller all work and are governed by the same RBACAuthorizer as any other identity. Pod-bound tokens only (no Secret/Node binding); no OIDC discovery document backing the issuer |
| `kubectl auth can-i` (`SelfSubjectAccessReview`)               | ✅ Real per-verb/per-resource evaluation from the same authorizer that gates live traffic                                                                                                                                                                                                                                                                           |
| Admission webhooks                                             | ❌ Not implemented                                                                                                                                                                                                                                                                                                                                                  |
| OpenAPI schema (`kubectl apply` client-side validation)        | ✅ Served as Static Assets, verified with a real `kubectl apply` (no `--validate=false`) against a running dev stack                                                                                                                                                                                                                                                |

## Node backends

Two ways to get Pods actually running, mix-and-match per cluster:

| Backend                                                                | Setup                                                               | Image                                                              | Networking                                                                 |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------ | -------------------------------------------------------------------------- |
| **BYO VM** (`cmd/agent`)                                               | Run the real, unmodified k3s agent yourself (EC2 or any Linux host) | Any OCI image, pulled by containerd like any other Kubernetes node | Full — flannel/kube-proxy, `kubectl logs`/`exec` via the VPC kubelet proxy |
| **Containers node backend** (Pod-on-Containers, virtual-kubelet-style) | included in `npm run deploy` (workers/k8flare), no VM to run        | **Allowlisted only** — see below                                   | Limited — see below                                                        |

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
- **No routable Pod IP; `kubectl logs`/metrics work, `exec`/`attach` don't
  yet.** These Pods run `hostNetwork: true` (injected at admission,
  `pkg/apiserver/computeclass.go`) because the microVM sandbox has no
  working netfilter, so there is no separate Pod IP distinct from the
  node's own. `kubectl logs` (+ `-f`) and the kubelet's read-only
  stats/metrics endpoints are bridged through the consolidated Worker →
  NodeVM DO → `containerFetch` on `cmd/agent`'s plain-HTTP kubelet proxy
  (port 10256), TokenReview-authenticated like a real cluster
  (`docs/platform-verification.md` S16). `pods/proxy`/`services/proxy`
  ingress and a Pod's own outbound `ClusterIP` traffic are bridged the same
  way (`pkg/vkubeproxy`'s node-local TUN + userspace TCP forwarder,
  `workers/k8flare/src/nodes/podproxy.ts`; task #13) — the node-side
  intercept and the Worker-side Service/EndpointSlice resolution are both
  verified against real primitives, but a live end-to-end run against a
  deployed NodeVM is still open (`docs/platform-verification.md` S20: local
  Docker on an ARM Mac cannot boot this node image at all). `exec`/`attach`
  (secure kubelet port 10250) are not implemented for this backend yet.
- **Pod scheduling/deletion latency is ~10s, not sub-second.** `workers/nodes`
  discovers Pods bound to its node by listing them on its own Lease-renewal
  alarm (every ~10s) rather than via a push/watch — see `virtualnode.ts`'s
  header comment for why, and `docs/cost-model.md`'s "Phase 7 (nodes)
  implementation" section for the cost reasoning. Acceptable given
  Containers' own cold start is already 1–3s+.
- **`restartPolicy` is supported** (`Always`/`OnFailure`/`Never`), evaluated
  the same ~10s tick: a container that exited is restarted, left stopped, or
  marked `Succeeded`/`Failed` accordingly.

### The Containers node backend and `npm run deploy`

The node backend ships inside the single Worker deploy (`npm run deploy`
builds the node agent image input and deploys `workers/k8flare`, whose
`containers[]` section covers the NodeVM classes). Its operator surface
lives under the deployed Worker's `/nodes/*` routes (token-gated), e.g.:

```bash
# Health poke (wakes the scheduler DO's reconcile loop if it was parked)
curl -H "Authorization: Bearer $K3S_TOKEN" https://k8flare.<your-subdomain>.workers.dev/nodes/healthz
```

## Volumes (R2 PV/PVC)

k8flare provisions `PersistentVolumeClaim`s dynamically against R2, backed
by [Temporary Access Credentials](https://developers.cloudflare.com/r2/api/s3/temporary-credentials/)
minted locally (no Cloudflare API call — see `docs/cost-model.md`'s Phase 8
section). This is **S3 API access via environment variables, not a real
POSIX-mounted filesystem** — the right fit for an app that already speaks
S3 (most object-storage-aware apps do), not for one that expects a plain
file path.

### Using it (`workers/nodes` Pods)

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-data
spec:
  accessModes: ["ReadWriteMany"] # R2 has no real single-writer exclusivity to enforce
  resources:
    requests:
      storage: 1Gi # accepted, but not enforced as a hard quota in v1
  # storageClassName omitted -- defaults to "r2", the only class this
  # project dynamically provisions
---
apiVersion: v1
kind: Pod
metadata:
  name: my-app
spec:
  containers:
    - name: app
      image: k8flare/demo:latest # workers/nodes' v1 allowlist -- see Node backends above
      volumeMounts:
        - name: data
          mountPath: /data # accepted for schema completeness; nothing is actually mounted at this path in v1, see below
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: my-data
```

Creating the PVC above synchronously provisions and binds a `PersistentVolume`
in the same request (`kubectl get pvc` shows `Bound` immediately, no polling
needed). When `workers/nodes` starts `my-app`'s container, it mints a
credential scoped to exactly this PVC's R2 key prefix and injects it as:

| Env var                                                         | Contents                                                                                                        |
| --------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`/`AWS_SESSION_TOKEN` | Standard AWS credential trio — any S3 SDK/CLI picks these up automatically from its default credential chain    |
| `R2_ENDPOINT`                                                   | `https://<account-id>.r2.cloudflarestorage.com`                                                                 |
| `R2_BUCKET`                                                     | This cluster's shared R2 bucket (one bucket for every PVC, isolated by prefix — see `spikes/s6-r2/RESEARCH.md`) |
| `R2_PREFIX`                                                     | This PVC's own key prefix (`pvc-<uid>/`) — write/read keys under this prefix, not the bucket root               |

Any S3-compatible SDK (boto3, aws-sdk-\*, aws4fetch, `aws-cli`) works
immediately with these five variables and no other configuration.

**Credentials expire and are refreshed, not renewed in place.** A minted
credential is short-lived (1 hour by default). `workers/nodes` re-mints a
fresh one every time it (re)starts a Pod's container for any reason, and —
for `restartPolicy: Always` Pods only — proactively cycles the container
once its credential is past expiry, so long-running Pods keep working
without manual intervention. This means a brief container restart roughly
once per hour for a Pod that runs that long; `restartPolicy: OnFailure`/
`Never` Pods are not proactively cycled (forcing a restart would violate
their own semantics) and will start seeing `403`s from R2 if they outlive
the credential's TTL. See `workers/nodes/src/virtualnode.ts`'s "Credential
refresh" doc comment for the full design and what's left as future work (an
in-image credential-refresh sidecar, which would need a base image built to
cooperate with it — out of scope for v1's single demo image).

**Not yet implemented**: a real POSIX-mounted filesystem at
`volumeMounts[].mountPath` (Cloudflare Containers officially supports
FUSE-mounting an R2 bucket, but this can't be verified in local `wrangler
dev` — see `spikes/s6-r2/RESEARCH.md` — so it's offered as a future opt-in
once verified against a real deployment, not in v1), and deleting the R2
objects under a PVC's prefix when it's deleted (the `PersistentVolume`
object itself is cleaned up; the underlying data is not — see
`pkg/apiserver/pvcbind.go`'s package doc comment).

### Using it (BYO VM nodes)

A BYO VM node (`cmd/agent`, a real unmodified k3s agent) is a normal
Kubernetes node with a real kubelet and container runtime — it can run any
existing CSI driver that mounts an S3-compatible bucket as a filesystem,
independent of anything `workers/nodes` does. This isn't wired up by
k8flare itself (a CSI driver is a separate DaemonSet/controller a cluster
operator installs), but any of the common S3-backed CSI drivers work
against R2's S3-compatible endpoint (`https://<account-id>.r2.cloudflarestorage.com`,
`region: auto`) the same as against real AWS S3:

- [`s3fs-fuse`](https://github.com/s3fs-fuse/s3fs-fuse) (or the `csi-s3`/
  `ctrox/csi-s3` CSI wrapper around it) — the most common choice, POSIX
  semantics via FUSE.
- [`goofys`](https://github.com/kahing/goofys) — faster than `s3fs` for
  some workloads, looser POSIX compliance.
- Cloudflare's own `tigrisfs`-based FUSE example
  (`developers.cloudflare.com/containers/examples/r2-fuse-mount/`), if
  running the mount directly rather than via a CSI driver.

This path is independent of k8flare's own per-PVC credential minting (that
endpoint is service-binding-only — reachable from another Worker in this
project, not from a BYO VM host, by design: it has no separate
authentication of its own, trusting the service-binding boundary the same
way every other `/internal/*` route in this project does). Configure the
CSI driver the normal way instead: create your own
[R2 API token](https://developers.cloudflare.com/r2/api/tokens/) via the
Cloudflare dashboard (Object Read & Write, scoped to one bucket), point the
driver's endpoint at `https://<account-id>.r2.cloudflarestorage.com`
(`region: auto`), and store the resulting Access Key ID/Secret Access Key
as a Kubernetes `Secret` the CSI driver reads — this gives a BYO VM Pod a
real mounted filesystem today, which `workers/nodes` Pods don't have in v1.

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

5. **Cluster DNS** — ~~CoreDNS as a Deployment~~ superseded 2026-07-07: a
   node-local DNS shim + DNS-over-HTTPS synthesis endpoint instead (no
   Containers/CoreDNS dependency, see the [DNS row](#core-resources)
   above and `docs/general-purpose-k8s-plan.md`'s Phase 4). ClusterDNS
   is already advertised to kubelets.
6. **API & auth parity** — ~~OpenAPI discovery (no more `--validate=false`)~~
   done (see [`docs/k8s-version-bump.md`](docs/k8s-version-bump.md)),
   ~~PriorityClass~~ n/a here (registered but not consumed, see the
   Scheduling table), ~~TokenRequest/TokenReview~~ done, ~~RBAC~~ done.
   **Update:** `rbac.authorization.k8s.io/v1` object registration, a
   `SelfSubjectAccessReview` handler, the actual
   authorizer-in-front-of-the-stores step (the real upstream
   `RBACAuthorizer` + bootstrap policy, enforcing on every API path and
   the TS watch path), and real ServiceAccount `TokenRequest` tokens as
   a non-admin identity source are all done as of 2026-07-07 — see
   [Auth & admission](#auth--admission) above. This item is closed.

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
- **Cross-cloud node networking: use k3s's own `wireguard-native` backend,
  not Cloudflare Mesh.** Evaluated and **not adopted** (Phase 9,
  `spikes/p9-mesh/RESEARCH.md`): flannel's `host-gw` backend needs true L2
  adjacency, which Mesh (a relayed-through-a-PoP L3 network, no direct
  peer-to-peer path, real throughput/UDP-loss tradeoffs, no Kubernetes/CNI
  integration precedent) can't provide either, and Mesh's one hypothesized
  advantage over k3s's own alternative — NAT traversal — couldn't be
  verified (it requires Cloudflare dashboard/Zero Trust access outside this
  evaluation's scope). For nodes that don't share a subnet, use k3s's
  built-in `--flannel-backend=wireguard-native` plus `--node-external-ip`
  instead — live-verified end-to-end through this project's own control
  plane in Phase 9 (`cmd/agent --node-external-ip` and the apiserver's
  `FlannelExternalIP` config field). See
  [`docs/cloudflare-mesh-networking.md`](docs/cloudflare-mesh-networking.md)
  for the full evaluation and history.
- **`k8f` CLI** — standalone OAuth 2.0 PKCE login (independent of
  Cloudflare's `cf` CLI): log in, provision, get a kubeconfig in one
  command.
- **Cloudflare Access integration** — connectivity and identity for the API,
  mapped onto RBAC once Track A lands it.
- **Worker / Durable Object / DynamicWorker / Facets as Kubernetes
  resources** — manage real Cloudflare primitives with `kubectl`.

## Multi-cluster (management API)

One deployment serves many clusters. The zero-config **default** cluster
lives at the bare URL and authenticates with `K3S_TOKEN`, exactly as before
— everything in this README keeps working with no provisioning step.
Additional clusters are provisioned through the admin-only management API
and live under a `/c/<id>` path prefix (kubectl and client-go fully support
path-prefixed server URLs):

```bash
# Create a cluster; the response carries the id, its first bearer token,
# and a ready-to-use kubeconfig (server: https://<host>/c/dev1)
curl -X POST https://<host>/clusters \
  -H "Authorization: Bearer $ADMIN_TOKEN" -d '{"id": "dev1"}'

curl https://<host>/clusters                          # list
curl https://<host>/clusters/dev1/kubeconfig          # kubeconfig YAML
curl -X POST https://<host>/clusters/dev1/tokens      # mint a 2nd token (rotation)
curl -X DELETE https://<host>/clusters/dev1/tokens/<tokenId>  # revoke (refuses the last one)
curl -X DELETE https://<host>/clusters/dev1           # teardown (async, idempotent)
```

Each provisioned cluster gets its own Durable Object tree (storage, watch
fan-out, controllers, node scheduler), its own rotatable token vault, and —
for R2-backed volumes — its own `clusters/<name>/` object-key prefix. Tokens
never cross clusters: a request for `/c/dev1` is authenticated against
dev1's vault at the door, and the default cluster only ever accepts
`K3S_TOKEN`.

The management API itself authenticates with either (or both) of:

- `ADMIN_TOKENS` — comma-separated rotatable admin secrets
  (`npx wrangler secret put ADMIN_TOKENS`), presented as `Authorization:
Bearer <token>`.
- `ACCESS_TEAM_DOMAIN` + `ACCESS_AUD` — Cloudflare Access JWT verification
  (signature against the team's JWKS, issuer, audience, expiry).

With neither configured the API falls back to the dev-only admin token
`k8flare-dev-admin-token` — the same posture as `K3S_TOKEN`'s dev fallback.
**Production deployments must set `ADMIN_TOKENS` or the Access variables.**

## Quick Start

### From Release

```bash
# Download and extract the Workers bundle (the single workers/k8flare
# deploy unit, prebuilt WASM chunks included)
gh release download -R k8flare/k8flare -p 'k8flare-workers-*.tar.gz'
tar xzf k8flare-workers-*.tar.gz
cd k8flare-workers
pnpm install

# Set your cluster token (one Worker, one secret)
npx wrangler secret put K3S_TOKEN --config workers/k8flare/wrangler.jsonc

# Deploy the single Worker
npm run deploy
```

### From Source

```bash
git clone https://github.com/k8flare/k8flare.git
cd k8flare
pnpm install  # plain `npm install` fails here -- package.json uses pnpm workspace:* deps

# Build Go WASM chunks (needs a local Go toolchain + binaryen's wasm-opt;
# skip this and use the release tarball above if you'd rather not install one)
npm run build:wasm

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
  --server https://your-k8flare.workers.dev \
  --token YOUR_K3S_TOKEN &

./k8flare-scheduler-linux-arm64 \
  --server https://your-k8flare.workers.dev \
  --token YOUR_K3S_TOKEN &

./k8flare-controller-manager-linux-arm64 \
  --server https://your-k8flare.workers.dev \
  --token YOUR_K3S_TOKEN &
```

See [scripts/ec2-user-data.sh](scripts/ec2-user-data.sh) for automated EC2 bootstrap (currently automates the agent only).

Nodes are expected to share one VPC/subnet by default (flannel's `host-gw`
backend). If your agents span clouds or accounts that don't share a subnet,
pass `--node-external-ip YOUR_NODE_PUBLIC_IP` to `k8flare-agent` on every
node — this is k3s's own built-in
[multicloud networking](https://docs.k3s.io/networking/distributed-multicloud)
support (`wireguard-native` flannel backend), evaluated and recommended over
Cloudflare Mesh for this purpose; see
[`docs/cloudflare-mesh-networking.md`](docs/cloudflare-mesh-networking.md).

## Configuration

### Worker Environment Variables

| Variable                | Required                                                                                            | Description                                                                                                                                       |
| ----------------------- | --------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `K3S_TOKEN`             | Yes                                                                                                 | Cluster authentication token. Generate with `openssl rand -hex 32`                                                                                |
| `R2_ACCOUNT_ID`         | Only for [R2 PV/PVC](#volumes-r2-pvpvc)                                                             | Your Cloudflare account ID                                                                                                                        |
| `R2_ACCESS_KEY_ID`      | Only for [R2 PV/PVC](#volumes-r2-pvpvc)                                                             | Access Key ID of a parent [R2 API token](https://developers.cloudflare.com/r2/api/tokens/) (Object Read & Write, scoped to `R2_BUCKET` below)     |
| `R2_SECRET_ACCESS_KEY`  | Only for [R2 PV/PVC](#volumes-r2-pvpvc)                                                             | Secret Access Key of the same token — set via `wrangler secret put`, never `vars`                                                                 |
| `R2_BUCKET`             | Only for [R2 PV/PVC](#volumes-r2-pvpvc)                                                             | The one shared R2 bucket every PVC provisions into (isolated per-PVC by key prefix, not by bucket — see [Volumes (R2 PV/PVC)](#volumes-r2-pvpvc)) |
| `CLOUDFLARE_API_TOKEN`  | Only for [per-Pod Mesh membership](#optional-per-pod-cloudflare-mesh-membership-containers-backend) | A Zero Trust/Tunnel-scoped API token, used to mint one Mesh connector per Pod on the Containers backend                                           |
| `CLOUDFLARE_ACCOUNT_ID` | Only for [per-Pod Mesh membership](#optional-per-pod-cloudflare-mesh-membership-containers-backend) | Your Cloudflare account ID (same value as `R2_ACCOUNT_ID` if both features are used)                                                              |

All four `R2_*` variables are set on `workers/k8flare` (the single Worker
that signs credentials). Without them, PVCs still bind and mint
syntactically valid credentials from fixed dev placeholder values — see
`docs/cost-model.md`'s Phase 8 section — that simply won't authenticate
against real R2 until real values are set.

### Optional: Cloudflare Mesh (for kubelet proxy)

To enable `kubectl logs` and `kubectl exec`, join each BYO VM node to
Cloudflare Mesh (`spikes/s17-mesh-nodevm/FINDINGS.md`) — this is the
recommended path, replacing the Tunnel+VPC Service setup below
(user decision 2026-07-07). The `MESH` binding (`network_id:
"cf1:network"`, an account-wide Mesh network) is already declared in
`workers/k8flare/wrangler.jsonc`; nothing to add there.

Per node, mint a connector via the Cloudflare API — no dashboard step
needed:

```bash
# Create the mesh node and fetch its connector token
curl -X POST "https://api.cloudflare.com/client/v4/accounts/$ACCOUNT_ID/warp_connector" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" -H "Content-Type: application/json" \
  -d '{"name": "my-node"}'
# -> note the returned "id", then:
curl "https://api.cloudflare.com/client/v4/accounts/$ACCOUNT_ID/warp_connector/$ID/token" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN"
# -> the "result" field is the connector token
```

Install `cloudflare-warp` on the node image (Ubuntu/Debian/RHEL —
Alpine/musl has no build, see the S17 findings) and pass the token to
`cmd/agent`:

```bash
./k8flare-agent --server=... --token=... --mesh-connector-token=<TOKEN>
```

`cmd/agent` joins Mesh, discovers its Mesh IP, and advertises it as
the Node's `ExternalIP` automatically — the gateway then reaches this
node's kubelet directly at that IP through the `MESH` binding, no
per-node Tunnel/VPC Service resource required.

### Optional: per-Pod Cloudflare Mesh membership (Containers backend)

A different, automated use of the same Mesh mechanism
(`spikes/s17-mesh-nodevm/FINDINGS.md`'s per-Pod-Mesh entry): each Pod
scheduled onto the Pod-on-Containers backend
(`k8flare.com/compute: containers`) gets its OWN Mesh IP, decoupled from
the underlying NodeVM's own address — no manual per-node connector
minting, no dashboard step, no sidecar container. Set `CLOUDFLARE_API_TOKEN`
(Zero Trust/Tunnel-scoped) and `CLOUDFLARE_ACCOUNT_ID` on
`workers/k8flare`; `cf-containers-scheduler` mints and tears down one
`warp_connector` per Pod automatically as Pods are scheduled/deleted.
Absent either variable, Pods on this backend boot exactly as before this
feature existed (no Mesh membership, no error). **Known cap**: every
Mesh connector counts against the account's 50-node limit — with this
feature on, that limit bounds concurrent Pods on this backend, not just
BYO VM nodes.

### Legacy: Cloudflare Tunnel + VPC Service

The original setup, still supported as a fallback
(`workers/k8flare/src/gateway/proxy/target.ts` tries `MESH` first,
then `KUBELET_VPC`):

```bash
./scripts/setup-tunnel.sh
```

Then add the VPC binding to `workers/k8flare/wrangler.jsonc`:

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
