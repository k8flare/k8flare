# k8flare

A Kubernetes control plane that runs on Cloudflare — and scales to zero.

The apiserver, controller-manager, scheduler, and garbage collector are the
**real upstream Kubernetes binaries** (k8s v1.36, k3s-flavored), compiled to
WASM and executed on demand as Worker Loader dynamic workers. Cluster state
lives in Durable Objects. When nobody talks to the cluster, nothing runs:
no processes, no polling alarms, no resident WebSockets — idle cost
approaches storage cost alone. A write wakes the control plane in
milliseconds. No other hosted Kubernetes offers a scale-to-zero control
plane.

## Architecture

```
kubectl / kubelet (BYO VM: unmodified k3s embed, cmd/agent)
   │ HTTPS + token
   ▼
workers/k8flare  — the ONE deployed Worker (routing, auth, watch streaming,
   │               kubelet proxy)
   ├─► apiserver   (Go WASM, dynamic worker; real generic registry over DO storage)
   ├─► kcm         (real kube-controller-manager: 6 workload controllers)
   ├─► gc          (real garbagecollector)
   ├─► sched       (real kube-scheduler)
   ├─  Cluster DO (kine-style revision log + per-namespace facets)
   ├─  WatchHub DO (watch fan-out over hibernating WebSockets)
   └─  Static Assets (WASM chunks, OpenAPI/discovery documents)
```

Key design points:

- **Reuse the real thing.** Control-plane semantics come from upstream
  packages, not re-implementations: `genericregistry.Store` +
  `storage.Interface` over Durable Objects, upstream RBAC authorizer, real
  JWT ServiceAccount tokens, upstream defaulters, real controllers. The
  hand-written surface is inventoried in
  [docs/custom-code-inventory.md](docs/custom-code-inventory.md).
- **Event-armed everything.** Controllers are woken by writes (a bounded
  "poke pump"), safety-net alarms self-disarm when idle, watches use
  hibernation. The cost rules are codified in
  [docs/cost-model.md](docs/cost-model.md).
- **Conformance CI is the definition of done.** Official Kubernetes
  e2e tests run against a real `wrangler dev` stack with a real k3s agent
  (kubelet + containerd): `.github/workflows/e2e-conformance.yml`
  (currently manual-dispatch only).

## What works today

- Full CRUD + watch for the ~38 core/apps/batch/networking/etc. resource
  types in `pkg/apiserver/apidef/table.go`, served by the upstream generic
  registry; real kubectl works end to end (tables, OpenAPI validation,
  apply, delete cascades via the real GC).
- Deployments/ReplicaSets/Jobs/CronJobs/DaemonSets reconciled by the real
  kube-controller-manager; Pods bound by the real kube-scheduler.
- BYO-VM nodes: `cmd/agent` embeds the unmodified k3s agent (kubelet,
  containerd, flannel, kube-proxy) and joins over HTTPS + token, optionally
  over Cloudflare Mesh
  ([docs/cloudflare-mesh-networking.md](docs/cloudflare-mesh-networking.md)).
- Namespace lifecycle admission, graceful deletion (Orphan/Foreground via
  the real GC), ServiceAccount tokens, RBAC, cluster DNS, ClusterIP/PodCIDR
  allocation, Endpoints/EndpointSlice reconciliation.

Not there yet: dynamic PV provisioning (PVCs stay Pending; a standard CSI
approach is under consideration), multi-tenant hosting UX
([docs/multi-tenancy-and-hosting.md](docs/multi-tenancy-and-hosting.md)),
and Pod-on-Containers NodeVMs are implemented but not currently deployed
(cost measurement pending).

## Getting started (development)

Prereqs: Go 1.26+, Node 24+/pnpm, binaryen (`wasm-opt`), and Docker only if
you want containers locally.

```sh
pnpm install
make wasm        # build apiserver/kcm/gc/sched WASM chunks (~2 min first time)
make dev         # wrangler dev on :8787
```

Talk to it with a bearer token (dev fallback: `k8flare-dev-token`):

```sh
curl -H 'Authorization: Bearer k8flare-dev-token' localhost:8787/api/v1/namespaces
```

Real kubectl needs TLS; `go test`-driven clients don't. Run the test
suites:

```sh
make test        # integration suite: real client-go against wrangler dev
make test-kcm    # control-plane smoke with the real KCM/GC/sched dynamic workers
```

## Joining a node (BYO VM)

Build and run the agent on a Linux VM:

```sh
make nodes-agent   # or: GOOS=linux go build ./cmd/agent
k8flare-agent --server https://<your-worker>.workers.dev --token <cluster token>
```

See `cmd/agent/main.go` for Mesh networking, labels/taints, and the other
flags.

## Deploying

`wrangler deploy -c workers/k8flare/wrangler.jsonc` — set `K3S_TOKEN` (and
`ADMIN_TOKENS` for the management API) as Worker secrets; a deployment with
no secrets is a dev posture. The `containers` section provisions NodeVM
container apps, which bill by wall clock — omit it unless you are using
Pod-on-Containers.

## Documentation

| Doc | What's in it |
|---|---|
| [docs/custom-code-inventory.md](docs/custom-code-inventory.md) | Hand-written vs upstream code, generation pipeline |
| [docs/platform-verification.md](docs/platform-verification.md) | Every platform spike + measured finding (S1–S25) |
| [docs/cost-model.md](docs/cost-model.md) | Idle/active cost per component, cost invariants |
| [docs/control-plane-architecture.md](docs/control-plane-architecture.md) | Controllers ↔ Cloudflare primitives mapping |
| [docs/general-purpose-k8s-plan.md](docs/general-purpose-k8s-plan.md) | Conformance expansion plan |
| [docs/k8s-version-bump.md](docs/k8s-version-bump.md) | How to bump the pinned Kubernetes version |
