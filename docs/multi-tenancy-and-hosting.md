# Multi-tenancy, scale, and hosted k8flare.com

Design doc, July 2026. Target architecture for turning today's single
hardcoded cluster into: many isolated clusters per deployment, namespace-level
isolation and storage scale-out inside each cluster, and eventually a hosted,
billable product at `k8flare.com`.

## Where we start

Today the entire cluster is **one** SQLite-backed Durable Object, addressed
from four call sites (`main.go:37`'s `IdFromName` — capitalized, the
generated binding's exported-method spelling — plus three lowercase
`idFromName` calls: `packages/k8s/src/watch.ts:164`,
`packages/dynamic-worker/src/helpers.ts:5`, `packages/crd/src/storage.ts:11`),
all with the same literal name `"default"`. Storage is one generic
kine-compatible table, but application data is **not** all under one prefix:
Kubernetes objects live under `/registry/...`, the cluster CA under
`/ca/{client,server}-ca.{crt,key}` (`certmanager.go:43-46`), node password
hashes under `/nodepasswords/<nodeName>` (`nodepassword.go:25`), and the
PodCIDR counter at `/registry/_internal/podcidr-counter`
(`scheduler.ts:80`). Auth is one static token (`K3S_TOKEN`), but it does not
map to a single fixed identity — depending on transport it resolves to
`system:masters` (plain bearer, or basic auth as any username other than
`node`), to `k3s:agent`/`system:nodes` (basic auth as `node`, the agent join
path), or to whatever identity a fronting TLS proxy asserts via
`X-Remote-User`/`X-Remote-Group` (`auth.go:40-94`) — one shared secret, three
possible outcomes. Nothing in any key or code path has a cluster or tenant
dimension yet. That makes the migration tractable: the blast radius of
"which cluster am I?" is small and enumerable — but "one prefix, one
identity" is a simplification to correct before design work leans on it.

## Verified Cloudflare platform facts (July 2026)

The design below leans on these verified numbers. Sources:
[DO limits](https://developers.cloudflare.com/durable-objects/platform/limits/),
[DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/),
[facets blog post](https://blog.cloudflare.com/durable-object-facets-dynamic-workers/),
[facets docs](https://developers.cloudflare.com/dynamic-workers/usage/durable-object-facets/),
[Containers limits](https://developers.cloudflare.com/containers/platform-details/limits/).

| Primitive             | Status                                                | Numbers that matter                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| --------------------- | ----------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| SQLite Durable Object | GA                                                    | **10 GB storage per DO**, ~**1,000 req/s** soft ceiling per DO (200–500 for complex ops), single-threaded, 2 MB max value/row, 32,768 WebSockets per DO, 30-day point-in-time recovery, unlimited instances per namespace                                                                                                                                                                                                                                              |
| DO Facets             | **Open beta** (Dynamic Workers, Agents Week Apr 2026) | Named child DOs of a parent; each facet has its **own isolated SQLite database** (confirmed) and independent lifecycle (`get`/`abort`/`delete`); reachable **only through the parent DO** (confirmed — no direct Worker binding), so all facet traffic funnels through the parent's single thread. Whether the 10 GB limit is shared across parent+facets or independent per facet is **not explicitly documented anywhere** — treated here as shared (see note below) |
| Workers for Platforms | GA, $25/mo                                            | Unlimited tenant Workers per dispatch namespace, per-tenant CPU/subrequest limits                                                                                                                                                                                                                                                                                                                                                                                      |
| Cloudflare Containers | GA (Apr 2026)                                         | Max instance **4 vCPU / 12 GiB / 20 GB disk**; each container is paired 1:1 with a Durable Object; scale-to-zero (sleep after timeout); built-in autoscaling not shipped at GA                                                                                                                                                                                                                                                                                         |
| Read replicas for DOs | Not available                                         | Only D1 has read replication; scaling DO reads means adding DOs yourself                                                                                                                                                                                                                                                                                                                                                                                               |

Two facet findings are decision-critical. First, facets are reachable only
through the parent DO (confirmed, no direct binding), so they buy **isolation
and lifecycle**, not extra throughput — anything that needs its own thread
must be a **top-level** DO.

Second, whether facets buy their own storage headroom is genuinely
undocumented — checked the facets usage guide, the DO limits page, the
announcement blog post, and the Dynamic Workers pricing page directly; none
states whether the 10 GB SQLite limit is per-facet or shared with the parent.
The strongest signal available is the blog post's own framing: "each instance
of `AppRunner` is **one Durable Object** composed of _two_ SQLite databases,"
and "one Durable Object can have any number of facets (subject to storage
limits)" (singular limits, not per-facet limits) — both describe parent+facets
as one billing/limit unit rather than independent ones. That framing is a
reasonable basis for a conservative planning assumption, not a documented
fact. **This design treats the 10 GB as shared** (the conservative
assumption: if wrong, we get more headroom for free; if we assumed
independent 10 GB and were wrong, namespace DOs would be under-provisioned
budget in the design). (Whether colocated facets execute truly in parallel is
separately undocumented — flagged unverified; the design below does not
depend on it either way.)

**Empirical check against a real deployment (July 2026, KOOFFICE account).**
Deployed a throwaway supervisor-DO Worker implementing the documented facets
pattern and exercised it directly. Findings:

- A facet's `class` **must** come from the Dynamic Workers loader
  (`env.LOADER.get(...).getDurableObjectClass(...)`) — passing a plain,
  statically-imported `DurableObject` subclass to `ctx.facets.get(name, () =>
({ class: LocalClass }))` fails at runtime with `TypeError: Incorrect type
for the 'class' field on 'StartupOptions': the provided value is not of
type 'DurableObjectClass or LoopbackDurableObjectNamespace or
LoopbackColoLocalActorNamespace'`. This isn't stated as a hard requirement
  anywhere in the docs (every example just happens to use the loader) —
  confirmed here to actually be one. Practically: our internal-CRD-as-facet
  design must go through the same `worker_loaders`/`LOADER` machinery
  `packages/dynamic-worker` already uses for user-supplied code, even for our
  own first-party facet classes.
- Storage isolation holds under real, nontrivial data, not just toy values:
  wrote independent keys into two sibling facets and the supervisor, then
  confirmed cross-reads return nothing (each only sees its own writes).
  `abort()` preserves a facet's data on next `get()`; `delete()` genuinely
  destroys it (subsequent `get()` returns a fresh, empty facet). Neither
  operation on one facet affects sibling facets or the supervisor.
- **`PRAGMA` statements are rejected** inside a Durable Object's
  `ctx.storage.sql.exec()` with `Error: not authorized: SQLITE_AUTH` —
  Cloudflare deliberately blocks this introspection path. There is
  consequently **no way for application code to directly query a facet's or
  a DO's actual on-disk SQLite size**; the wrangler CLI has no equivalent
  command either. This is a real operational gap for the design above: a
  cluster or namespace DO cannot self-report "how close to 10 GB am I"
  without maintaining its own byte-accounting (e.g. summing value lengths on
  write) rather than asking SQLite directly.
- Wrote ~1.01 GB into one facet (1,010 × 1 MB rows, confirmed via `SELECT
COUNT(*)`) with no error, no slowdown, and no smaller-than-10-GB checkpoint
  encountered. Concurrently wrote 100 MB into the supervisor and created a
  brand-new sibling facet — both succeeded immediately and remained correctly
  isolated from the 1 GB facet's data throughout. This confirms coexistence
  and isolation hold under real data volume, but **1 GB is too small to
  distinguish shared-vs-independent 10 GB** — under either model, 1 GB
  (or even 1 GB + 100 MB combined) is nowhere near either a 10 GB shared
  ceiling or a 10 GB-per-facet independent one. Definitively resolving that
  question needs a test near the actual boundary (one side filled to ~9 GB+,
  then checked whether the other side is constrained), which was not run.

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

> **Honest correction (Phase 4, 2026-07-02):** the "Namespace DO" tier
> described below -- a top-level, own-thread DO per namespace that
> follows the cluster DO's ordering -- was **not** what got built.
> Phase 4 implemented namespace data as **facets of the cluster DO**
> (`ctx.facets.get("ns/<namespace>", ...)`) instead, per the project's
> July 2026 decision to facet-split state "as much as possible" (see the
> v2 rewrite plan). This section is left as-written (not deleted) per
> the project's correction convention; the paragraphs immediately below
> explain what changed and why, then the original design continues
> unedited for the historical record.
>
> **What's different in practice:**
>
> - Namespace data lives in a facet of the _same_ Cluster DO, not a
>   separate top-level DO. A facet has its own SQLite database (S1,
>   confirmed empirically) but is reachable **only through its parent's
>   thread** (S1/S2) -- so "own thread" below is inaccurate for facets;
>   only "own storage" holds.
> - The **write path is simpler than described below**, not more
>   complex: because a facet is reached synchronously, in-process, from
>   the same request that assigned the revision, "forward {key, rev,
>   value} after commit" is not a separate async step with its own retry
>   queue -- the parent inserts its own envelope row (assigning the
>   revision) and then `await`s one `fetch()` call to the facet, within
>   the same request. There is no cross-DO availability gap to design
>   around the way there would be for a genuinely independent top-level
>   Namespace DO.
> - **Namespace deletion does NOT call `ctx.facets.delete()`**, contrary
>   to "namespace deletion = deleteAll() on this DO" below. Reproduced
>   empirically: repeatedly deleting and recreating a facet under the
>   _same name_ (a realistic scenario -- CI suites and iterative
>   development both reuse namespace names) works for the first few
>   cycles, then every following create through that facet name
>   permanently returns a false "already exists", deterministically at
>   the 4th cycle, unaffected by adding delays between requests -- an
>   undocumented platform limitation, not a bug in this codebase (S1's
>   spike only ever exercised a single delete-then-recreate). Since
>   every namespaced object is already individually tombstoned by the
>   cascade delete before the Namespace object itself is removed,
>   correctness doesn't depend on the facet-level delete -- skipping it
>   only forgoes a storage-GC nicety. See `workers/storage/src/index.ts`
>   (`handleDelete`) for the full writeup and the fix this leaves open
>   (facet names suffixed with the Namespace's own UID, so a reused
>   _name_ never reuses a facet _name_).
> - **WatchHub does not hold "one WS upstream" to the cluster DO.**
>   Reproduced empirically: a Durable Object cannot call
>   `ctx.acceptWebSocket()` on a WebSocket obtained via `resp.webSocket`
>   from calling `fetch()` on a _different_ DO -- hibernatable accept is
>   only for a WebSocketPair half the same invocation just created, not
>   for relaying a socket obtained from another DO's response. Instead,
>   the cluster DO **pushes** each event to WatchHub via a plain `fetch()`
>   POST immediately after committing it, which WatchHub fans out to its
>   client sockets. This is a net simplification (no upstream
>   connect/reconnect bookkeeping) and, unlike a held-open upstream
>   socket, keeps both DOs hibernation-eligible between events.
> - The `ca-vault`/`events-log` facets described below as children of
>   the (non-existent) separate flow are, in the implementation, facets
>   of the cluster DO directly, exactly as drawn in the diagram below --
>   that part of the diagram is accurate.
> - **Known open item, resolved 2026-07-03** (was open when this note was
>   first written): a minimal Go `net/http` client -- and therefore
>   client-go, kubectl, and every real Kubernetes controller -- did not
>   receive any bytes from a long-lived watch response against local
>   `wrangler dev`, while curl read the identical bytes immediately (see
>   `docs/cost-model.md`'s Phase 4 actuals and the WatchHub redesign
>   commit for the full repro). Root cause turned out to be a local
>   `wrangler dev`-only gzip-buffering interaction with Go's default
>   `Accept-Encoding: gzip`, fixed by an explicit `Content-Encoding:
identity` response header, and confirmed **absent in production**
>   (a real client-go `watch.Interface` receives every event promptly
>   both locally after the fix and against a real deployed Worker) --
>   see `docs/platform-verification.md`'s Correction log ("Go net/http
>   streaming block against local wrangler dev was a client/dev-stack
>   Accept-Encoding interaction, not a platform-wide bug") for the full
>   writeup.
>
> Everything below this note describes the **originally planned**
> Namespace-DO design; treat "Namespace DO" as "namespace facet of the
> Cluster DO" when reading it, and the "Write path"/"Read path" sections'
> async-forwarding descriptions as superseded by the synchronous,
> in-process version above.

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
~$0.01/month. A small active cluster (3 nodes, each sending a Lease renewal +
status heartbeat every ~10s ≈ 3 × 8,640/day × 30 ≈ 780,000 requests/month)
costs **~$0/month in requests standalone** — that volume sits under the
Workers Paid plan's 1,000,000 requests/month included allowance, so it isn't
until rows/duration or additional traffic are added that this cluster shows
up as a line item at all. The catch: that allowance is pooled **per account,
across every cluster on it**, not per cluster — so it covers roughly one
active cluster before additional clusters start accruing the standard
$0.15/million marginal rate. Model pricing on marginal cost per cluster
above the shared allowance, not on a flat per-cluster number. The headline
product property still falls out of the platform: **a control plane that
costs ~nothing while idle and wakes in milliseconds** — no other hosted
Kubernetes offers scale-to-zero control planes. The biggest cost unknown is
WebSocket
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

> **Correction (2026-07-02):** this table's phase numbers are this
> document's own original delivery sequence, not the v2 rewrite plan's
> phase numbers (a separate, later plan that supersedes the delivery
> order above). What the v2 plan calls "Phase 4 — storage v2" delivered
> this table's Phase 2 (namespace data isolation -- as facets, not
> top-level DOs, see the "Target architecture" correction above), Phase
> 3 (WatchHub, redesigned as push-based fan-out), and part of Phase 4
> (the `ca-vault`/`events-log` facets, plus `ns/<namespace>`) all
> together, rather than in this sequence. "Namespace delete =
> `deleteAll()`" in Phase 2's acceptance criteria did not ship (see the
> correction above); the rest of each row's acceptance criteria was met
> or superseded as described inline.

### Implementation record (2026-07-06): path-prefix multi-cluster shipped

Phase 0 of the table above, plus the `POST /clusters` slice of Phase 5's
management plane, shipped in `packages/k8flare-worker/src/clusters/` — with one
deliberate deviation: cluster resolution is a **`/c/<id>` path prefix**
(the Rancher `/k8s/clusters/<id>` precedent), not the Host-based wildcard
routing described under "Routing" above. Path prefixes work today on
`*.workers.dev` and any single hostname with zero DNS/zone setup; Host
routing remains the plan for hosted `k8flare.com` and layers on top (map
Host → id, then reuse this exact resolution seam). The decisions, for
reference from code comments:

- **Decision A — tokens are plaintext in the cluster's own `ca-vault`
  facet** (one kine value at `/ca/cluster-tokens`, multiple
  concurrently-valid tokens = rotation). The vault already holds the CA
  private keys, the NodeVM path injects the real token into microVMs, and
  the kubeconfig endpoint re-serves it — hashing would add no real
  exposure barrier here while breaking both. Verifiers cache per isolate
  (60s TTL), so revocation propagates within the TTL; emergency
  revocation is cluster deletion.
- **Decision B — registry DO with uid indirection.** One tiny
  `ClusterRegistry` DO (`idFromName("registry")`) holds only
  `{id → uid, state}` and the list; DO instances are named `<id>@<uid>`
  so a recreated cluster id never reuses a DO or facet name (the facet
  name-reuse wedge), and auth never funnels through this single-threaded
  DO. No alarms, no WebSockets: idle = storage only.
- **Decision C — the derived-env seam.** The public router derives a
  per-request `Env` whose DO namespaces transparently retarget
  `idFromName("default")` → this cluster's tree
  (`clusters/clusterenv.ts`), so every downstream module stays
  single-cluster-shaped. Inside DOs the wrapper doesn't exist — sibling
  DOs of the same cluster share the DO's own instance name.
- **Decision D — provisioned clusters authenticate at the door** against
  their vault; the verified presented token is threaded into the derived
  env's `K3S_TOKEN` so downstream defense-in-depth checks re-validate
  trivially. The default cluster keeps its exact pre-multi-cluster
  semantics (env token, enforced downstream), preserving the zero-config
  OSS path and the Go/CI dev-token dependency.
- **Teardown** is admin-`DELETE`, async and idempotent: mark `deleting`
  (resolution 404s) → destroy Scheduler first (Containers are wall-clock
  billed) → Controllers → WatchHub → Cluster (facets + `deleteAll`) →
  registry record. Not covered in v1: R2 objects under
  `clusters/<doName>/` (needs SigV4 the Worker doesn't carry; R2 deletes
  are free, so a later cleanup pass loses nothing — see
  `docs/cost-model.md`).

Verified 2026-07-06 against real `wrangler dev`: two clusters on one
deployment with disjoint data and watch streams; each cluster's token
rejected by the other (Phase 0's acceptance shape, minus conformance CI
which runs against the default cluster unchanged); rotation (two live
tokens), revocation, refusal to delete the last token; teardown then
recreation of the same id yields a fresh empty cluster and kills the old
tokens. `go test ./pkg/apiserver/...` green against the same build.

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
