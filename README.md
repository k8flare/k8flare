# k8flare

A Kubernetes control plane that runs on Cloudflare — and scales to zero.

The control plane is built from the **real upstream Kubernetes packages**
(k8s v1.36, k3s-flavored) — the actual controller-manager/scheduler/GC
controller code and the upstream generic registry — compiled to WASM and
executed on demand as Worker Loader dynamic workers. Cluster state
lives in Durable Objects. When nobody talks to the cluster, nothing runs:
no processes, no polling alarms, no resident WebSockets — idle cost
approaches storage cost alone. A write wakes the control plane in
milliseconds.

**Status: pre-production.** Single maintainer, no tagged releases yet,
APIs and storage layout may change. See
[docs/adopter-quickstart.md](docs/adopter-quickstart.md) for what running
it actually requires and what is not there yet.

## Architecture

```
kubectl / kubelet (BYO VM: unmodified k3s embed, cmd/agent)
   │ HTTPS + token
   ▼
packages/k8flare-worker  — the ONE deployed Worker (routing, auth, watch streaming,
   │               kubelet proxy)
   ├─► apiserver   (Go WASM, dynamic worker; real generic registry over DO storage)
   ├─► kcm         (real kube-controller-manager: 7 workload controllers plus
   │                endpoint, endpointslice, nodeipam, nodelifecycle, tainteviction)
   ├─► gc          (real garbagecollector)
   ├─► sched       (real kube-scheduler)
   ├─► clusterop   (cluster operator: reconciles k8flare.com/v1alpha1 Clusters;
   │                runs only on the management "default" cluster)
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
- **Upstream conformance is the definition of done.** Official Kubernetes
  e2e tests run against a real `wrangler dev` stack with a real k3s agent
  (kubelet + containerd). Since 2026-09-12 that gate runs **locally, not in
  CI**: a maintainer runs the three control-plane variants by hand and the
  focus sets are lifted from `.github/workflows/e2e-conformance.yml`,
  which is kept as their reference and no longer executes. The recipe is in
  [docs/development.md](docs/development.md#running-upstream-conformance-locally);
  why CI is off is in [CONTRIBUTING.md](CONTRIBUTING.md#ci-there-isnt-any-since-2026-09-12).

## What works today

- Full CRUD + watch for the 41 core/apps/batch/networking/etc. resource
  types in `pkg/apiserver/apidef/table.go`, served by the upstream generic
  registry; real kubectl works end to end (tables, OpenAPI validation,
  apply, delete cascades via the real GC).
- Deployments/ReplicaSets/Jobs/CronJobs/DaemonSets reconciled by the real
  kube-controller-manager; Pods bound by the real kube-scheduler.
- Services get Endpoints and EndpointSlices, Nodes get a `spec.podCIDR`, and
  Nodes whose Lease goes stale are marked `Ready=Unknown` and tainted -- all
  from the real upstream controllers in the same kcm dynamic worker.
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

Prereqs: Go 1.26+, Node 24+/pnpm, binaryen (`wasm-opt`). Docker is only
needed if you want to run Pod-on-Containers NodeVMs locally — `make dev`
passes `--enable-containers=false` so the rest works without it.

```sh
pnpm install
make wasm        # builds the five WASM chunks
make dev         # wrangler dev --local (default port 8787)
```

`make wasm` on a cold machine also downloads the k3s-flavored Kubernetes
tree (a few GB) and regenerates the `.build/` module mirrors every Go
command here depends on, so budget ~5 minutes the first time. Once the
mirrors exist a no-op rebuild is a few seconds. Do not run `go mod
download` by hand before that: the mirrors are `replace` targets that do
not exist yet, so it fails until `make` has created them.

`make dev` runs wrangler with `--local`: the `MESH` VPC binding in
`wrangler.jsonc` has no local emulation, so a plain `wrangler dev` would
open a real Cloudflare proxy session at startup — failing without
credentials, and quietly using your real account with them.

Chunk sizes are gated at build time against the Worker Loader's 64MiB cap.
The tightest chunk is apiserver: 48,784,426 bytes of the 67,108,864-byte
cap in a local `make wasm` build on 2026-09-13, about 18MB of headroom. Adding a dependency
to it can fail the build outright; `make wasm` prints the remaining
headroom for every chunk, and that is the number to trust over this one.

Talk to it with a bearer token (dev fallback: `k8flare-dev-token`):

```sh
curl -H 'Authorization: Bearer k8flare-dev-token' localhost:8787/api/v1/namespaces
```

Real kubectl needs TLS; `go test`-driven clients don't. Run the test
suites:

```sh
make test        # all four lanes (~8 min)
make test-unit   # seconds: Go/WASM runtime + TypeScript units, no wrangler
make test-kcm    # control-plane smoke with the real KCM/GC/sched dynamic workers
```

`test-unit` runs in-process. The other three (`test-apiserver`,
`test-kcm`, `test-clusterop`) each start their own `wrangler dev` and drive
it with real client-go, with and without the real controllers.

## Joining a node (BYO VM)

Build and run the agent on a Linux VM:

```sh
make nodes-agent   # builds packages/k8flare-worker/images/node/k8flare-agent (linux/amd64)
./packages/k8flare-worker/images/node/k8flare-agent --server https://<your-worker>.workers.dev --token <cluster token>
```

See `cmd/agent/main.go` for Mesh networking, labels/taints, and the other
flags.

## Deploying

Before your first deploy, change the values in
`packages/k8flare-worker/wrangler.jsonc` that are specific to a
deployment:

| Value | Why |
|---|---|
| `vars.GATEWAY_URL` | The public URL nodes dial and the origin baked into minted kubeconfigs. Leave it pointing elsewhere and your nodes join someone else's control plane. |
| `name` | The Worker name claimed in your account. |
| `containers[].authorized_keys` | Empty by default. Add your own SSH key only if you want to debug NodeVMs. |

Then:

```sh
wrangler deploy -c packages/k8flare-worker/wrangler.jsonc
```

Set `K3S_TOKEN` (the default cluster's root token, the only Worker
secret; it also authenticates the management API). Additional per-cluster
tokens are minted through the admin API; **with no secret and no minted
tokens the cluster accepts the publicly documented dev token
`k8flare-dev-token`** — never leave a public deployment in that state.
The `containers` section provisions NodeVM container apps, which bill by
wall clock — omit it unless you are using Pod-on-Containers.

## Contributing

Topic branches, English commits, and the local gates (`make check`,
`make vet`, `make test`) are described in
[CONTRIBUTING.md](CONTRIBUTING.md); day-to-day local-dev traps are in
[docs/development.md](docs/development.md). Security issues go through
[SECURITY.md](SECURITY.md), not the issue tracker — and read its "current
posture" section before exposing a deployment.

## Documentation

| Doc | What's in it |
|---|---|
| [CONTRIBUTING.md](CONTRIBUTING.md) | Branches, commit rules, the local gates, and why the gate is local rather than CI |
| [SECURITY.md](SECURITY.md) | Reporting a vulnerability, and the honest current auth posture |
| [docs/adopter-quickstart.md](docs/adopter-quickstart.md) | **Start here if you are evaluating it**: required Cloudflare entitlements, what to change before deploying, security posture, cost, backup/exit |
| [docs/development.md](docs/development.md) | Local dev: required `wrangler dev` flags, DO state, test lanes, the 64MiB cap |
| [docs/admin-guide.md](docs/admin-guide.md) | Operator guide: deploy, secrets, cluster issuance, cost ops (Japanese) |
| [docs/user-guide.md](docs/user-guide.md) | Cluster user guide: kubeconfig, what works, quirks (Japanese) |
| [docs/custom-code-inventory.md](docs/custom-code-inventory.md) | Hand-written vs upstream code, generation pipeline |
| [docs/known-issues.md](docs/known-issues.md) | **What is broken or unproven today.** Read before deciding to run this |
| [docs/platform-verification.md](docs/platform-verification.md) | Every platform spike + measured finding (S1–S67). Read its current-state summary first |
| [docs/cost-model.md](docs/cost-model.md) | Idle/active cost per component, cost invariants |
| [docs/control-plane-architecture.md](docs/control-plane-architecture.md) | Controllers ↔ Cloudflare primitives mapping |
| [docs/general-purpose-k8s-plan.md](docs/general-purpose-k8s-plan.md) | Conformance expansion plan |
| [docs/k8s-version-bump.md](docs/k8s-version-bump.md) | How to bump the pinned Kubernetes version |
| [docs/cluster-api-design.md](docs/cluster-api-design.md) | Cluster resource (`k8flare.com/v1alpha1`) + cluster-operator: design and implementation record (Japanese) |
