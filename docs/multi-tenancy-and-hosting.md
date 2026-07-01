# Multi-tenancy, scale, and hosted k8flare.com

Design doc, July 2026. Target architecture for turning today's single
hardcoded cluster into: many isolated clusters per deployment, namespace-level
isolation and storage scale-out inside each cluster, and eventually a hosted,
billable product at `k8flare.com`.

## Where we start

Today the entire cluster is **one** SQLite-backed Durable Object, addressed as
`idFromName("default")` in exactly four places (`main.go:37`,
`packages/k8s/src/watch.ts:164`, `packages/dynamic-worker/src/helpers.ts:5`,
`packages/crd/src/storage.ts:11`). All state — objects, the cluster CA, node
password hashes, the PodCIDR counter — lives under a single `/registry` prefix
in one kine-compatible table. Auth is one static token (`K3S_TOKEN`) that maps
to `system:masters`. Nothing in any key or code path has a cluster or tenant
dimension yet. That makes the migration tractable: the blast radius of
"which cluster am I?" is small and enumerable.

## Verified Cloudflare platform facts (July 2026)

The design below leans on these verified numbers. Sources:
[DO limits](https://developers.cloudflare.com/durable-objects/platform/limits/),
[DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/),
[facets blog post](https://blog.cloudflare.com/durable-object-facets-dynamic-workers/),
[facets docs](https://developers.cloudflare.com/dynamic-workers/usage/durable-object-facets/),
[Containers limits](https://developers.cloudflare.com/containers/platform-details/limits/).

| Primitive             | Status                                                | Numbers that matter                                                                                                                                                                                                                                                                                      |
| --------------------- | ----------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| SQLite Durable Object | GA                                                    | **10 GB storage per DO**, ~**1,000 req/s** soft ceiling per DO (200–500 for complex ops), single-threaded, 2 MB max value/row, 32,768 WebSockets per DO, 30-day point-in-time recovery, unlimited instances per namespace                                                                                |
| DO Facets             | **Open beta** (Dynamic Workers, Agents Week Apr 2026) | Named child DOs of a parent; each facet has its **own isolated SQLite database** and independent lifecycle (`get`/`abort`/`delete`), but **all facets share the parent's 10 GB limit** and are **reachable only through the parent DO**, so all facet traffic funnels through the parent's single thread |
| Workers for Platforms | GA, $25/mo                                            | Unlimited tenant Workers per dispatch namespace, per-tenant CPU/subrequest limits                                                                                                                                                                                                                        |
| Cloudflare Containers | GA (Apr 2026)                                         | Max instance **4 vCPU / 12 GiB / 20 GB disk**; each container is paired 1:1 with a Durable Object; scale-to-zero (sleep after timeout); built-in autoscaling not shipped at GA                                                                                                                           |
| Read replicas for DOs | Not available                                         | Only D1 has read replication; scaling DO reads means adding DOs yourself                                                                                                                                                                                                                                 |

Two facet findings are decision-critical and worth repeating: facets buy
**isolation and lifecycle**, not throughput or storage headroom. Anything that
needs its own 10 GB or its own thread must be a **top-level** DO. (Whether
colocated facets execute truly in parallel is undocumented — flagged
unverified; the design below does not depend on it.)

## The constraint that shapes everything: resourceVersion ordering

Kubernetes clients — every informer, `kubectl get -A`, and the real
kube-scheduler this project just migrated to — depend on `resourceVersion`
being a **monotonic total order per resource type across all namespaces**. A
reflector lists pods in all namespaces at revision X, then opens a watch
"from X"; that only means something if one authority ordered all pod writes.
Independent per-namespace revision counters break cross-namespace watch
resumption, which breaks the scheduler.

This is not a Cloudflare limitation — etcd itself is a single-writer system
(all writes serialize through the raft leader), and the upstream apiserver
scales _reads_ with its watch cache, not writes. The correct shape, there and
here, is:

> **One ordering point per cluster for writes; scale out reads and watch
> fan-out horizontally.**

The k8s API contract permits one useful relaxation: resourceVersions are
never comparable **across types** (aggregated apiservers with separate etcds
are the existing proof). So per-_type_ ordering shards are legal if a single
cluster's write rate ever demands them — see "Scale model" below.

## Target architecture

Chosen target (decided July 2026): build the namespace-DO tier from the
start, rather than keeping namespaced data inside the cluster DO.

```
kubectl / kubelet / kube-scheduler / controllers
        │  Host: <cluster>.k8flare.com resolves the cluster
        ▼
   Worker (stateless, runs everywhere)
        │
        ├── watch ──────────► WatchHub DO × N     one WS upstream, ≤32k client
        │                                         sockets each; add hubs to scale
        ▼  writes, ordering
   Cluster DO  (one per cluster)
        │   • assigns the global revision (kine log)
        │   • stores cluster-scoped objects (Node, Namespace, PV, RBAC…)
        │   • broadcasts watch events (the ordering source)
        │   • hosts controller alarms (they are writers — locality matters)
        │   ├─ facet: ca-vault        CA keys + node-password hashes
        │   └─ facet: events-log     high-churn Event isolation (beta-gated)
        │
        │  forwards {key, rev, value} after commit
        ▼
   Namespace DO  (one per namespace, top-level: own 10 GB, own thread)
        │   • idempotently applies forwarded writes, keyed by revision
        │   • serves namespace-scoped GET/LIST from its own thread
        │   • namespace deletion = deleteAll() on this DO
        └─ facets: churny per-type isolation only if measured (leases,
           endpointslices) — not one facet per resource type by default
```

The original three-level intuition — cluster DO, namespace DO, facets inside
— survives with two amendments forced by the facts above:

1. **Namespace DOs are followers, not authorities.** They apply writes the
   cluster DO already ordered. This keeps resourceVersion semantics intact
   while still giving each namespace its own 10 GB and its own read thread.
2. **Facets are used sparingly.** Since facets share their parent's thread
   and storage budget, "one facet per resource type" adds moving parts
   without adding capacity. Facets are reserved for churn isolation (Events)
   and for compartmentalizing sensitive material (CA vault), each gated on
   the beta maturing.

### Write path

1. Worker resolves the cluster, authenticates, and sends the write to the
   **cluster DO**.
2. The cluster DO assigns the next revision, appends to its kine log,
   broadcasts the watch event, and acks the client. Durability at this point
   is Cloudflare's storage relay (writes are quorum-replicated before commit
   returns).
3. It then forwards `{key, revision, value, prevRevision}` to the owning
   namespace DO, which applies idempotently (revision is the idempotency
   key). Forwarding is asynchronous with a retry queue drained by the cluster
   DO's alarm; a lagging or briefly unavailable namespace DO delays _reads of
   that namespace_, never correctness of ordering.
4. Once a namespace DO acks a range, the cluster DO may **trim the value
   blobs** from its own log (keeping `{key, revision, tombstone}` for watch
   replay and ordering). This is what makes storage scale: the cluster DO
   stays small; bulk data lives in namespace DOs, 10 GB each.

Recovery: a namespace DO is rebuilt by replaying the log; both DO classes
have 30-day PITR underneath as the backstop.

### Read path

- Namespaced GET/LIST → the **namespace DO**, served at its applied revision.
  Requests that demand `resourceVersion >= X` wait for the applied revision
  to catch up (bounded) or fall through to the cluster DO for the
  un-forwarded tail.
- Cluster-scoped and all-namespaces LIST → cluster DO picks the snapshot
  revision; the Worker fans out to namespace DOs and merges. All-namespaces
  LISTs are rare on hot paths (informers do them once at startup).

### Watch path

Watch events are broadcast where they are ordered — the cluster DO — exactly
as `broadcastEvent` works today. **WatchHub DOs** subscribe once upstream and
re-broadcast to up to 32k client sockets each; the hub preserves upstream
order, so adding hubs multiplies watcher capacity without touching semantics.
Replay of historical revisions reads the cluster DO log (keys + revisions)
and fetches trimmed values from namespace DOs.

### Scale model

Per cluster:

| Axis                         | Ceiling                      | Notes                                                                                                                                                        |
| ---------------------------- | ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Writes                       | ~200–1,000 req/s             | The ordering thread. Node Lease renewals are 1/10s/node → 100 nodes ≈ 10 writes/s; hundreds of nodes and thousands of pods fit before this is the constraint |
| Watchers                     | 32k × number of hubs         | Effectively unbounded; hubs are cheap                                                                                                                        |
| Namespaced storage           | 10 GB × number of namespaces | After value-trimming lands                                                                                                                                   |
| Cluster-scoped storage + log | 10 GB                        | etcd's own default quota is 2 GB (8 GB recommended max) — one cluster DO already exceeds a stock etcd budget                                                 |
| Watch fan-out latency        | one extra DO hop via hubs    | Direct cluster-DO sockets remain fine below ~thousands of watchers                                                                                           |

If a single cluster's _write_ rate ever exceeds the ordering thread, the
escape hatch is per-type ordering shards (a type-owner DO for the churniest
types — Events first, then Leases), which the k8s API contract explicitly
tolerates. Designed, not scheduled.

The real horizontal axis is **cluster count**: every cluster is an
independent DO tree with an independent budget, so the platform scales by
adding clusters, not by growing one. DO instances per namespace are
unlimited.

## Isolation model

| Boundary                             | Mechanism                                                                                                                                                                                                                                                                          | Strength                                                                                                                                               |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Cluster ↔ cluster                    | Separate DO trees, separate CA (already stored in-DO, so it shards for free), separate token(s), separate 10 GB/thread budgets, separate PITR timelines, optional [jurisdiction](https://developers.cloudflare.com/durable-objects/reference/data-location/) (e.g. EU) per cluster | Hard — nothing shared but Worker code                                                                                                                  |
| Namespace ↔ namespace (same cluster) | Separate namespace DOs (storage, read thread, deletion blast radius); Events/CA material in separate facets                                                                                                                                                                        | Storage-level today; **API-level isolation additionally needs RBAC** (see the general-purpose plan) before namespaces can be sold as tenant boundaries |
| In-cluster workload ↔ control plane  | Nodes are outside Cloudflare entirely (BYO VMs) or Containers with their own isolation                                                                                                                                                                                             | Unchanged                                                                                                                                              |

A concrete bug this structure retires: namespace cascading deletion today
sweeps only Go-registered stores and misses the TypeScript-side CRDs
(`namespacedelete.go:22-25`). When a namespace is a DO, deletion is
`deleteAll()` on that DO — nothing to forget.

## Hosted k8flare.com

### Routing

Wildcard DNS `*.k8flare.com` plus a wildcard Worker route on the zone; the
Worker maps `Host` → cluster ID (Workers KV as the routing cache, management
plane as the source of truth) and everything downstream is already
per-cluster. The supervisor already advertises `https://` + `r.Host`
(`supervisor.go:153`), so agents follow the right hostname automatically.
`*.workers.dev` and self-hosted single-cluster mode keep working — cluster
resolution falls back to `"default"` when no management plane is configured,
which preserves the zero-config OSS path. Customer-owned domains
(`k8s.acme.com`) would use Cloudflare for SaaS custom hostnames later;
wildcard custom hostnames are Enterprise-only, so that is not an early
feature.

### Management plane

A small control-plane-of-control-planes, deliberately boring: one management
DO (or D1) holding accounts, clusters, tokens, plans, and usage rollups.
Provisioning is `POST /clusters` → create the record → bootstrap the cluster
DO (namespaces, CA, minted per-cluster token) → return a kubeconfig. The
`k8f` CLI (`k8f login` via OAuth PKCE, `k8f cluster create`) and a web
console are thin clients of this API. Today's `sync.Once` bootstrap and the
global `K3S_TOKEN` env var both move into per-cluster records; that is the
bulk of phase 0.

### Nodes

- **BYO agents** — today's flow, unchanged: run `k8flare-agent` anywhere,
  pointed at the per-cluster URL with the per-cluster token. Cloudflare
  Mesh/Workers VPC covers private-network reachability.
- **Managed nodes on Cloudflare Containers** — each managed node is a
  Container instance (paired 1:1 with a per-node DO, scale-to-zero) running
  the same agent image. Ceiling per node is 4 vCPU / 12 GiB / 20 GB, so
  managed pools target small/bursty workloads; big nodes stay BYO. This is
  the first node backend the platform itself can bill for.

### Metering and billing

Cloudflare has no turnkey per-tenant metering product (verified July 2026),
so metering is ours: Analytics Engine events keyed by cluster ID (API
requests, watch messages, storage samples, managed-node minutes) rolled up
into the management plane, billed through Stripe. The DO-per-cluster and
DO-per-namespace structure makes attribution trivial — a cluster's usage _is_
its DO tree's usage.

Rough unit economics (verify by measurement before pricing anything): an
idle, hibernated cluster costs approximately its storage — a 50 MB cluster is
~$0.01/month. A small active cluster (3 nodes: Lease renewals + status
patches ≈ 0.9M requests/month ≈ $0.14, plus rows and active duration) lands
in single-digit dollars per month of infrastructure cost. The headline
product property falls out of the platform: **a control plane that costs
~nothing while idle and wakes in milliseconds** — no other hosted Kubernetes
offers scale-to-zero control planes. The biggest cost unknown is WebSocket
message + duration billing under sustained informer watches; measure that
first.

### Security posture, in order

1. Per-cluster static token (phase 0 — replaces the global `K3S_TOKEN`).
2. ServiceAccount TokenRequest + RBAC (shared prerequisites with the
   general-purpose plan; RBAC is what upgrades namespaces from storage
   isolation to sellable tenant isolation).
3. Human/agent identity via Cloudflare Access & Managed OAuth (GA) mapped
   onto RBAC identities.
4. Per-cluster audit logs via Workers Logs/Logpush; evaluate envelope
   encryption for Secrets at rest.

## Delivery phases

Each phase keeps the live cluster working and the conformance CI green.

| Phase | Work                                                                                                                                  | Acceptance                                                                                                       |
| ----- | ------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| 0     | Cluster resolution (Host → cluster ID at the 4 call sites), per-cluster token, per-cluster bootstrap; `"default"` fallback intact     | Two clusters on one deployment, both pass the conformance baseline; token of one rejected by the other           |
| 1     | Revision authority refactor inside the cluster DO: log append + broadcast decoupled from "where values live" (no behavior change yet) | Conformance green, zero client-visible change                                                                    |
| 2     | Namespace DOs: forward/apply, namespaced reads served by them, value trimming in the cluster log                                      | Aggregate storage across namespaces exceeds 10 GB in a demo; namespace delete = `deleteAll()`; conformance green |
| 3     | WatchHub DOs                                                                                                                          | Benchmark: ≥10× today's concurrent watchers with ordering preserved                                              |
| 4     | Facets (beta-gated): `ca-vault`, Event churn isolation                                                                                | Facet loss/reset drills; no cross-facet reads from dynamic code                                                  |
| 5     | Management plane + wildcard `k8flare.com` routing + `k8f cluster create`                                                              | A cluster provisioned end-to-end in one command                                                                  |
| 6     | Metering → Stripe billing, plans/quotas; managed Container node pools                                                                 | Private beta with real invoices                                                                                  |

## Open questions

- Facet execution parallelism is undocumented (tracked as unverified); the
  design deliberately never depends on it.
- WebSocket message + duration billing coefficients under real informer load
  — measure before publishing prices.
- Whether the management plane wants D1 (relational, read-replicated) or a
  DO (transactional, simpler) — decide in phase 5.
- Per-type ordering shards: define the trigger metric (sustained cluster-DO
  write saturation) before building anything.
- DO jurisdictions constrain placement; how that composes with
  latency-sensitive multi-region customers is unexplored.
