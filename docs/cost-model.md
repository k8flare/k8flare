# Cost model

k8flare's concept is a serverless control plane whose idle cost
approaches storage cost alone (CLAUDE.md). Following cost invariant #5 —
new features and components get a cost estimate before they're
implemented — this document records per-component idle and active unit
costs.

- **Estimate** and **actual** are tracked in separate columns. Unmeasured
  items are marked `TBD` — numbers are never invented.
- Even when an estimate is superseded by a measurement, the estimate
  itself is kept, not deleted (honest correction convention, CLAUDE.md
  inviolable rule #4).
- Sources (commit hash, official doc name, changelog date) are always
  cited.

## Billing primitives

| Primitive                       | Billing axis                                                                                                                                                                                                                                                  | Known numbers                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | Source                                                                                                                                                                                                                               |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Workers                         | CPU time actually consumed only. Waiting on I/O is free. No wall-clock limit on an HTTP streaming response (CPU time limit of 5 min/invocation applies)                                                                                                       | Unit rate itself not recorded in this document (TBD)                                                                                                                                                                                                                                                                                                                                                                                                                            | v2 rewrite plan's research findings (primary source: Cloudflare Workers pricing docs, URL not recorded — to be added when referenced)                                                                                                |
| Durable Objects                 | requests / duration (GB-s) / rows read-written (SQLite) / alarm invocation count                                                                                                                                                                              | 10GB storage/DO, ~1,000 req/s soft ceiling (200–500 for complex ops), single-threaded, 2MB max value/row, 32,768 WebSockets/DO, 30-day PITR. $ rates not recorded in this document (TBD)                                                                                                                                                                                                                                                                                        | [DO limits](https://developers.cloudflare.com/durable-objects/platform/limits/), [DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/) (cited via docs/multi-tenancy-and-hosting.md)                     |
| Cloudflare Containers           | Wall-clock (uptime) based vCPU-second and GiB-second billing                                                                                                                                                                                                  | vCPU cost $0.00002/vCPU-sec, memory cost $0.0000025/GiB-sec. Paid-plan included allowance: 375 vCPU-min/month, 25 GiB-hours/month, free. Instance ceiling 4vCPU/12GiB/20GB disk, paired 1:1 with a DO, scale-to-zero                                                                                                                                                                                                                                                            | v2 rewrite plan's estimate (Phase 5 route B), [Containers limits](https://developers.cloudflare.com/containers/platform-details/limits/) (cited via docs/multi-tenancy-and-hosting.md)                                               |
| Worker Loader (Dynamic Workers) | Billed per unique load                                                                                                                                                                                                                                        | $0.002/unique/day                                                                                                                                                                                                                                                                                                                                                                                                                                                               | CLAUDE.md cost invariant #5, v2 rewrite plan                                                                                                                                                                                         |
| R2                              | Storage (GB-month, tiered Standard/Infrequent Access) + Class A ops (writes/lists/multipart) + Class B ops (reads) + data retrieval (Infrequent Access only). Egress and deletes (`DeleteObject`/`DeleteObjects`/`DeleteBucket`) are always free and uncapped | Standard: $0.015/GB-month storage, $4.50/M Class A requests, $0.36/M Class B requests, free tier 10GB-month + 1M Class A + 10M Class B/month. Infrequent Access: $0.01/GB-month storage (30-day minimum), $9.00/M Class A, $0.90/M Class B, $0.01/GB retrieval, no free tier. Minting a Temporary Access Credential is a control-plane call, not a Class A/B op — free via local JWT signing, or drawn from the shared 1,200 req/5min account budget if minted via the REST API | [R2 pricing](https://developers.cloudflare.com/r2/pricing/) (page dateModified 2026-05-28), [R2 limits](https://developers.cloudflare.com/r2/platform/limits/) (page dateModified 2026-06-08) — cited via `spikes/s6-r2/RESEARCH.md` |
| Workers KV                      | Read/write counts + storage (assumed, details unconfirmed)                                                                                                                                                                                                    | TBD (not yet used; planned for use as `CLUSTER_ROUTES` when hosted multi-tenancy work starts, to be added before implementation — corrected from an earlier "Phase 2" estimate, which shipped without it: Phase 2 was a functionally-equivalent repo split, and `CLUSTER_ROUTES` has no single-tenant use)                                                                                                                                                                      | —                                                                                                                                                                                                                                    |

## Per-component estimates

| Component                    | Idle monthly cost (target: ~0)                                                                                                                                                                                                    | Active unit cost                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | Estimate                                   | Actual                                                                                                                                                                                                                                                                                                  |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| gateway (TS Worker)          | ~0 (stateless, no DO)                                                                                                                                                                                                             | Workers request billing (actual CPU time)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | TBD                                        | Not yet done (not implemented, Phase 2)                                                                                                                                                                                                                                                                 |
| apiserver (Go WASM)          | ~0                                                                                                                                                                                                                                | Workers request billing (actual CPU time). Whether the WASM startup tax applies depends on S5's outcome (isolate singleton-ization, see `docs/platform-verification.md` S5)                                                                                                                                                                                                                                                                                                                                                                                   | TBD (pending S5)                           | Not yet done (not implemented, Phase 2)                                                                                                                                                                                                                                                                 |
| storage: Cluster DO          | Target: storage cost only (alarm parked)                                                                                                                                                                                          | DO requests / duration GB-s / rows read-written + alarm invocation count (event-armed only). A namespaced key op now costs 1 parent request + 1 facet request (GET/list) or 1 parent write + 1 facet `/apply` write (PUT/DELETE) -- roughly double the DO-request count of the pre-facet single-table design, in exchange for per-namespace storage headroom. Loader cost is a flat $0.002/day regardless of namespace count: one generic facet class, one content-hash loader key, reused via `ctx.facets.get(name, ...)` for every ns/events/ca-vault facet | TBD                                        | See Actual: idle alarm parking measured, Phase 4                                                                                                                                                                                                                                                        |
| storage: WatchHub DO         | ~0 (while hibernating)                                                                                                                                                                                                            | DO requests: 1 `/push` per write that touches a watched key (from Cluster, awaited synchronously) + 1 `/replay` per new client connection (to Cluster) + N WebSocket sends per push (N = matching connected clients). No upstream connection to hold open (see design note below), so nothing keeps WatchHub resident between events -- nothing added to WatchHub's own idle cost beyond the DO-request cost of whichever push/replay call last touched it                                                                                                    | TBD                                        | See Actual below, Phase 4                                                                                                                                                                                                                                                                               |
| runtime (TS Worker + LOADER) | ~0 (when cron hasn't fired)                                                                                                                                                                                                       | Workers request billing + Worker Loader $0.002/unique/day                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | TBD                                        | Not yet done (not implemented, Phase 2)                                                                                                                                                                                                                                                                 |
| controllers                  | See "controllers execution path: two-route estimate" below                                                                                                                                                                        | Same                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | Same                                       | Execution shape verified working (Phase 5, local `wrangler dev`); blocked on deployed WASM size (~19MiB gzip for kube-controller-manager, limit 10MiB) before $ can be measured. kube-scheduler is host-process/BYO-VM-only (does not compile for GOOS=js). See "Phase 5 implementation actuals" below. |
| nodes (Pod-on-Containers)    | Target ~0 plus one mandatory, unconditional cost: VirtualNode DO's Lease-renewal alarm fires every ~10s for as long as the virtual node is registered, whether or not any Pod exists (see "Phase 7 (nodes) implementation" below) | Containers vCPU/GiB-second billing. **Each running Pod container implies a DO alarm firing at least every ≤3 minutes** while it's up — confirmed by reading the `@cloudflare/containers` self-monitoring source (`spikes/s3-containers/FINDINGS.md`); it re-arms on that cadence while running and calls `deleteAlarm()` once stopped, so it parks when idle (event-armed rule satisfied, but not zero-alarm while any Pod is running). **The Pod's own running cost is the user's workload cost** (cost invariant #6, not counted as control-plane cost)     | See "Phase 7 (nodes) implementation" below | Implemented (this phase); $ not yet measured (no `wrangler deploy` performed)                                                                                                                                                                                                                           |
| R2 PV/PVC                    | Target: storage cost only                                                                                                                                                                                                         | R2 operation billing                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | TBD                                        | Not yet done (not implemented, Phase 8)                                                                                                                                                                                                                                                                 |

## Phase 4 (storage v2) actuals

Written after implementation, per cost invariant #5's "estimate before
implementing, update from measurement" cycle -- the estimate row above
was written first, this section records what was actually verified
against real `wrangler dev` (2026-07-02).

**Alarm parking (cost invariant #1/#3) -- verified working end to end**:
using a temporary debug endpoint exposing `ctx.storage.getAlarm()`
(removed before committing), the full cycle was observed directly:

1. Fresh cluster, no Nodes/Services: `alarm: null` (never armed at all
   -- `initialize()` only arms if `hasPendingSafetyNetWork()` is already
   true, e.g. a DO waking from eviction with live state).
2. Create a Node: alarm arms (debounced ~1s pull-in via
   `wakeSchedulerSoon`), fires once (PodCIDR allocated, confirmed), and
   _re-arms_ ~60s later because the Node is still live.
3. Delete the Node, then wait past the next scheduled fire: the alarm
   fires (no-op, nothing to allocate), evaluates
   `hasPendingSafetyNetWork()` (now false), and does **not** re-arm:
   `alarm: null` again. Confirms the idle cluster genuinely stops
   costing alarm invocations, not just "polls less often."
4. Single write (a Service needing a ClusterIP) from that parked state:
   alarm immediately re-arms. Confirms resume-on-write.

This is the strongest evidence available short of a production billing
statement: an idle cluster (no Nodes, no Services) incurs zero DO alarm
invocations after its initial settle, and a cluster with live
infrastructure still bounds its safety-net cost to once per
`SAFETY_NET_INTERVAL_MS` (60s) regardless of write volume (writes that
need prompt attention pull the _existing_ alarm closer via
`wakeSchedulerSoon` rather than scheduling additional ones).

**Facet routing overhead**: every namespaced key operation (all of
Pod/Service/ConfigMap/Secret/etc. -- the bulk of real workload data)
now involves at least one additional DO-to-DO `fetch()` call from
Cluster to the owning `ns/<namespace>` facet (or `events-log`/
`ca-vault`), versus zero extra calls in the pre-facet single-table
design. All-namespaces LIST/replay fans out to every namespace's facet
concurrently (`Promise.all`) rather than serially, so wall-clock cost
scales with the slowest facet, not the sum -- consistent with S1's
finding that facet dispatch isn't serialized. This trade (more DO
requests, less storage pressure on the parent) is the intended one:
see docs/multi-tenancy-and-hosting.md's honest-correction note for why
per-namespace storage headroom was worth this.

**WatchHub redesign cost implications**: the original design (WatchHub
holds one upstream WebSocket to Cluster) turned out to not be a
supported platform pattern at all (see the redesign commit and
watchhub.ts's module comment) -- Cluster instead pushes each event to
WatchHub via a plain `fetch()` POST, awaited as part of the write path.
This is a _better_ cost shape than the original design, not just a
workaround: no upstream connection means nothing keeps WatchHub
DO-resident between events (the original design's upstream socket would
have been held via `.accept()`, which -- per the redesign commit's
finding -- cannot be hibernation-managed for a socket obtained from
another DO's response, meaning WatchHub would have stayed non-hibernated
for as long as any client was connected, a wall-clock-shaped cost this
redesign avoids entirely).

**Verified working, cost mechanism not yet measurable in $**: the
above confirms the _shape_ of billable events (request counts, when
alarms fire) is correct. Actual $ costs require either Cloudflare's
published per-request DO pricing (not yet looked up into this doc) or a
production account with real traffic -- both out of scope for this
phase (no `wrangler deploy`).

**Known gap this phase leaves for production verification**: a
minimal Go `net/http` client (and therefore client-go, and therefore
kubectl and every real Kubernetes controller) does not receive any
bytes from a long-lived streaming watch response against local
`wrangler dev`, while curl reads the identical bytes immediately. See
the WatchHub redesign commit message for the full repro. If this
reproduces against production Cloudflare too, it is cost-relevant as
well as functionally blocking: a watch connection that a real client
can't consume can't be measured for its actual duration/request cost
either. This must be re-verified against a real deployment before
watch can be considered done, not just facet storage.

## controllers execution path: two-route estimate

scheduler/KCM are run in order to use the real upstream controllers —
under either execution path, wall-clock-billed residency would violate
the concept (CLAUDE.md).

> **Update, 2026-07-02**: per user decision (commit `62c9c43`; full
> detail in `docs/platform-verification.md`'s S8 Correction log),
> Containers is no longer an available fallback for controllers — some
> form of route A (WASM-resident) is mandatory. Route B below is kept
> as-written for the historical record of what was originally being
> compared; it is not a live option going forward. If plain
> stream-residency (S8 (a)–(d)) turns out to be infeasible, the fallback
> is an alternative **WASM-only** design (DO-hosted event-driven
> execution, WebSocket-hibernation re-entry, wake-on-write) whose cost
> profile still needs to be estimated here once a specific alternative is
> chosen.

Which technique within route A is adopted is decided by the results of
S8 in `docs/platform-verification.md`.

### Route A: WASM-resident (preferred, if S8 succeeds)

`workers/controllers` (Go WASM) stays running on the syumai fork as a
response stream that's never closed, kept alive by internal calls from
the Cluster DO. Since informers spend most of their time waiting on I/O,
actual CPU consumption should be small — the design hypothesis is that
**billing is proportional to actual reconcile work, not wall-clock
time**. The very notion of starting/stopping goes away, so
demand-start/idle-stop orchestration, idle-timeout tuning, and thrash
mitigation aren't needed — expected to make the implementation simpler
than route B.

**Estimate**: since time an informer spends waiting on I/O is free,
low-to-medium frequency usage is naturally cheaper than route B
(Containers), and even under constant load, billing only accrues for
actual reconcile computation — so this shouldn't compare unfavorably
against a VPS. This is a design hypothesis, not yet measured.

**Actual**: not yet done. `docs/platform-verification.md` S8 (b) will
measure CPU-ms/hour; switch to route B if it diverges from expectations.

### Phase 5 implementation actuals (2026-07-03): execution shape confirmed, blocked on deployed size, not cost

Full detail and reproduction numbers: `docs/platform-verification.md`'s
S8 section, "Phase 5 implementation" entry. Summary for this document's
purpose: the DO-hosted + WaitUntil-resident execution shape itself is
**confirmed working end-to-end against real `wrangler dev`** — real
kube-controller-manager controllers (nodeipam, nodelifecycle,
taint-eviction-controller, endpoint, endpointslice, plus this repo's
original five workload controllers), instantiated once per DO instance,
reconciling real objects through a real `gateway`→`apiserver`→`Cluster DO`
chain over a Cloudflare service binding. This validates route A's cost
_shape_ (informers idle on I/O, billing should track actual reconcile
work) qualitatively, but **the compiled `app.wasm` for all ten
controllers together is ~19MiB gzip — Cloudflare Workers' 10MiB gzip
limit blocks deployment before any cost measurement against production
billing can even be attempted** (S8(b)'s CPU-ms/hour methodology, proven
against a minimal toy binary, could not be re-run against the real
controllers binary this phase — there is nothing to `wrangler deploy` yet).
kube-scheduler is a separate, harder blocker: it does not compile for
GOOS=js/wasm at all (see platform-verification.md), so it remains
host-process/BYO-VM-only regardless of the size question — no route A
cost applies to it.

**Actual (size, not $, measured this phase — see platform-verification.md
for the full breakdown)**: `k8s.io/client-go`'s generated typed
clientset+informers alone cost ~8.87MiB gzip before any controller logic;
one controller pushes past 15MiB; all ten reach ~19MiB. This is an
artifact of using real upstream controller code (every constructor needs
a full `clientset.Interface`), not of controller count.

### Phase 5 follow-up (2026-07-03): PodCIDR/Endpoints/Node lifecycle restored

Restored as synchronous apiserver reconciles; actual cost measured.

The three hand-written TS reconcilers deleted the same day as the
size-blocker findings above (`endpoints.ts`, `nodelifecycle.ts`,
`scheduler.ts`'s PodCIDR half) on the premise that the WASM-resident KCM
above would replace them turned out to need restoring once that premise
was confirmed wrong: none of the ten controllers deployed above, so
these three pieces of functionality were unavailable in any non-BYO-VM
deployment. Restored using the same pattern Phase 3 already established
for ClusterIP allocation (`pkg/apiserver/clusterip.go`): call the real
upstream **algorithm**, not the whole informer-based controller,
synchronously from apiserver's Go WASM binary — zero new Workers, zero new
Containers, zero new alarm polling beyond what already existed.

| Feature                                                | Mechanism                                                                                                                  | Real upstream reuse                                                                                                                                                                             | Measured incremental gzip cost                                          |
| ------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Node PodCIDR allocation                                | Synchronous, apiserver's Node-create path (`pkg/apiserver/nodecidr.go`)                                                    | `k8s.io/kubernetes/pkg/controller/nodeipam/ipam/cidrset.CidrSet` (leaf package, no client-go)                                                                                                   | +11KB (isolated); +35KB (real feature, incl. CAS persistence glue)      |
| Endpoints/EndpointSlice                                | Synchronous, on every Service/Pod write incl. the `/status` subresource (`pkg/apiserver/endpoints.go`)                     | `k8s.io/endpointslice/util` (`IsPodReady`/`ShouldSetHostname`, separate lightweight package); `labels.SelectorFromValidatedSet`; small hand-ported functions where the real ones are unexported | +34KB (isolated util import); +97KB (real feature)                      |
| Node lifecycle (Lease staleness → Unknown+taint+evict) | Cluster DO's existing event-armed safety-net alarm pings a new internal apiserver route (`pkg/apiserver/nodelifecycle.go`) | Real grace-period defaults (`nodelifecycle/config/v1alpha1`), real taint ops (`pkg/util/taints`), real toleration matching (`corev1.Toleration.ToleratesTaint`)                                 | +53KB (full feature, incl. the two lightweight upstream packages above) |
| **All three combined**                                 | —                                                                                                                          | —                                                                                                                                                                                               | **+150,104 bytes (146.6KB), 7,699,133 → 7,849,237 bytes gzip**          |

**What was _not_ reusable, and why (the negative-space finding this phase
adds)**: `k8s.io/endpointslice`'s root package exports exactly the one
function needed (`FindPort`) but is the same package as
`reconciler.go`'s `NewReconciler`, which requires a full
`k8s.io/client-go/kubernetes.Interface` — Go compiles a package as a
whole, so importing it costs the same ~2.6MiB `client-go` tax the whole-
controller approach above already ran into, confirmed by actually adding
the import and rebuilding (7,699,133 → 10,469,635 bytes). Similarly,
`node_lifecycle_controller.go`'s taint templates live in the same package
as the full, client-go-entangled `nodelifecycle` controller. Both were
hand-ported (not reimplemented from scratch — copied with exact upstream
citations) rather than imported. This is a real, load-bearing distinction
this phase learned the hard way: Go's package-is-the-compilation-unit
property means "is this function exported" is not sufficient to know
whether importing it is cheap — the whole package's import graph is what
counts, and a single heavy sibling file (a `clientset.Interface`-typed
constructor, in every case found so far) can make an otherwise-tiny,
genuinely pure function unreachable without paying for the whole package.

Cumulative apiserver size after this phase: **7,849,237 bytes gzip, 78.8%
of the 9.5MiB CI budget** — ~2.1MiB of headroom remains. No new alarm
polling was introduced: Node writes now also call the existing
`armSafetyNetSoon` (1s debounce), and the existing 60s safety-net tick
now also covers "is there a live Node" in addition to "is there a live
Service", but the alarm chain still parks (cost invariants #1/#3) once
neither exists — verified against real `wrangler dev`, not asserted (see
the commit implementing this for the exact repro: Node create → taint
applied within ~2s via a real end-to-end DO-alarm→service-binding→
apiserver round trip, not a direct call).

### Route B: Containers (demand-start/idle-stop) — superseded (kept for the record, user decision 2026-07-02)

Estimate assuming 1vCPU+1GiB, within the Containers Paid included
allowance (375 vCPU-min/month, 25 GiB-hours/month, free) — this is the
v2 rewrite plan's estimate, not a measurement:

| Usage pattern                                                                   | Assumption                   | Approx. monthly cost                                                              |
| ------------------------------------------------------------------------------- | ---------------------------- | --------------------------------------------------------------------------------- |
| Low frequency (solo development, 20 starts/day × 30s)                           | 10 min/day = 300 min/month   | **$0**, within the free allowance                                                 |
| Medium frequency (small team, 50 starts/day × 60s)                              | 50 min/day = 1,500 min/month | **~$1.35/month**, billed for the overage past the free allowance                  |
| Constantly busy (minute-scale CronJobs, frequent HPA adjustments — never idles) | Effectively 24-hour uptime   | **~$58/month** in vCPU+memory — more than an equivalent-spec VPS (from ~$5/month) |

For the "constantly busy" case, the README will honestly state that
running k3s directly on a VPS is cheaper, and recommend BYO VM (hosting
`cmd/scheduler` / `cmd/controller-manager` as the existing unmodified
binaries) as the deployment target for that usage pattern — a safety net
that requires zero extra implementation. In the medium-frequency range,
watch out for "thrashing" (an idle-timeout that's too short causes
frequent start/stop cycles, each triggering an informer relist that
worsens both cost and recovery latency). The idle-timeout isn't
hard-coded to a fixed value — its initial value will be chosen based on
`docs/platform-verification.md` S3/S7 measurements (relist's actual cost
and duration), with the reasoning recorded here.

**Actual**: not yet done (only implemented and measured if route B is
adopted).

### Rejected decision: moving apiserver to Containers

We considered the proposal that, if controllers can run on Containers,
moving apiserver there too would make tracking upstream versions
easier — and rejected it based on the following estimate (see
`docs/platform-verification.md` S7 for the detailed verification items
and re-verification plan):

- Latency: a Workers isolate warms up in under 5ms vs. Cloudflare
  Containers cold-starting typically in 1–3 seconds, 3–15 seconds in
  real-world use.
- Cost: keeping one cluster's 1vCPU+1GiB instance warm 24 hours a day
  works out to vCPU cost $0.00002/vCPU-sec × 2,592,000 sec/month ≈ $52,
  plus memory cost $0.0000025/GiB-sec × 2,592,000 sec/month ≈ $6.5, for
  **~$58/cluster/month**. Fatal under a hosted-product model with many
  idle clusters.
- Because apiserver is on the hot path (every kubectl command goes
  through it), this cold-start delay directly hits the user
  experience — unlike scheduler/KCM (asynchronous reconcilers), which
  differ in this respect.

**Conclusion**: apiserver stays on WASM/Worker (lowest latency wins).
This is a provisional decision until S7 produces project-specific
measurements.

## Phase 7 (nodes) implementation

`workers/nodes` (virtual-kubelet-style Pod-on-Containers backend) is
implemented this phase as: one `VirtualNode` Durable Object (registers
`cf-containers-<pool>`, renews its Lease, reconciles Pods) plus three
`PodContainer{Small,Medium,Large}` Durable Object classes (Cloudflare
Containers-backed, one per `docs/cost-model.md`'s and `spikes/s3-containers/
FINDINGS.md`'s "image x size-tier" allowlist unit -- see
`workers/nodes/src/images.ts`).

**Two independent, unconditional alarm sources, priced separately**:

1. **VirtualNode's Lease-renewal alarm: fires every ~10s for as long as the
   virtual node is registered, regardless of Pod count.** This is a
   deliberate, documented exception to "nothing runs while idle" (cost
   invariant #1) -- justified the same way a real BYO-VM node's kubelet
   heartbeat is not treated as a cost-invariant violation: it is the
   Kubernetes NodeLease protocol itself (a real kubelet's default is also a
   ~10s renew interval, see `workers/nodes/src/client.ts`'s
   `NODE_LEASE_RENEW_INTERVAL_MS` doc comment), not a k8flare-invented poll.
   Pod reconciliation (list Pods bound to this node, start/stop
   `PodContainer` instances) rides this same already-mandatory tick rather
   than adding a second one -- see virtualnode.ts's header comment for why a
   push from Cluster DO (the lower-latency alternative) was not built: it
   would require editing `workers/storage`'s existing `handlePut`/
   `handleDelete` (mirroring `needsControllersPing`), which is out of scope
   for this task. Net effect: with zero Pods, each 10s tick costs 3 Worker
   requests to `workers/apiserver` (`getNode` existence check,
   `getLease`+`renewLease`, `listPodsForNode`) and no Container activity —
   **≈259,200 apiserver requests/month per registered virtual node**,
   whether or not it ever runs a Pod. This is the "target ~0 plus one
   mandatory cost" row in the table above; not literally zero, but bounded
   and independent of workload. $ rate for Workers requests is not yet
   recorded in this document (TBD, see the Billing primitives table above).
2. **Each running Pod's `PodContainer` DO alarm: fires at least every ≤3
   minutes while that Pod is up** (the `@cloudflare/containers` library's
   own self-monitoring, not something this code added — see
   `spikes/s3-containers/FINDINGS.md` item 3). Parks (`deleteAlarm()`) once
   the container is stopped. This is Pod workload cost (cost invariant #6),
   not control-plane cost — a Pod that runs for an hour costs at most ~20 of
   these alarm firings, on top of whatever vCPU/GiB-second billing the
   container itself accrues while running.

**Estimate, not yet measured**: no `wrangler deploy` of `workers/nodes` has
been performed this phase (implementation + local `wrangler dev` +
Docker-based smoke verification only, per this task's verification gate) — the
259,200 requests/month figure above is a request-_count_ estimate from the
code's own design, not a production measurement. Following cost invariant #5,
this row should be updated with real `wrangler tail`/billing numbers the first
time `workers/nodes` is actually deployed.

**Image/size allowlist (v1)**: one demo image (`workers/nodes/images/demo`,
distroless-based per `spikes/s3-containers/FINDINGS.md`'s CA-bundle finding)
across three Cloudflare Containers named instance types -- `lite` (small,
1/16 vCPU / 256 MiB), `basic` (medium, 1/4 vCPU / 1 GiB), `standard-1` (large,
1/2 vCPU / 4 GiB) -- chosen because both a Pod's container image and its
instance size are deploy-time-fixed on Cloudflare Containers (no runtime
override API for either, confirmed in `spikes/s3-containers/FINDINGS.md`), so
a real allowlist is necessarily a fixed set of (image, size) pairs, not a
single configurable Container class. A Pod's `resources.requests` (summed
across its containers) is rounded up to the first tier whose capacity fits
both CPU and memory; a Pod that fits none of the three, uses more than one
container, or names an image outside the allowlist is rejected (`status.phase
= Failed`) rather than silently mis-scheduled.

## Idle-cluster verification checklist

A checklist for mechanically confirming cost invariant #1, "nothing is
allowed to run while idle."

- [x] Controllers Worker's actual consumed CPU time is effectively zero
      while idle -- **automated in CI as of Phase 6**
      (`.github/workflows/cost-gate.yml`), but only as a local proxy: a
      sustained idle window (30s, zero requests) after settling produces
      zero new wrangler-dev-logged activity for `workers/controllers`.
      This is not the same measurement as S8(b)'s production
      `wrangler tail` CPU-ms (below), which remains the manual/production
      procedure -- there is no production deployment to measure against
      in CI, and `workers/controllers` in its current ~19MiB-gzip shape
      can't be deployed at all (S8 section) to produce one. Route
      B/Containers is not applicable: superseded, see "controllers
      execution path" below.
- [x] No DO alarm is scheduled (parked state — only re-armed on waiting
      events, with no fixed-interval polling left running) -- **verified
      2026-07-02 against real `wrangler dev`** for the Cluster DO's
      safety-net alarm (manual check, "Phase 4 (storage v2) actuals"
      above), and **automated in CI as of Phase 6**
      (`.github/workflows/cost-gate.yml`) for both the Cluster and
      Controllers DOs: idle (no Nodes/Services) parks to no scheduled
      alarm and stays there; a single write resumes it; deleting the
      last live Node/Service lets the next scheduled fire park it again
      rather than re-arming forever. The CI job reads Miniflare's
      on-disk alarm store directly rather than adding a debug endpoint --
      see `docs/platform-verification.md`'s Phase 6 correction log entry
      for why this is possible locally despite S1's item 10 originally
      saying otherwise (that finding turned out to be facet-specific,
      not true of top-level DOs).
- [ ] WebSocket connections are hibernating (WatchHub DO) -- client-facing
      sockets use `ctx.acceptWebSocket` as designed, but end-to-end watch
      consumption by a real Kubernetes client (client-go/kubectl) could
      not be verified this phase (see the known gap above); the
      `ctx.waitUntil` keep-alive this line originally referred to no
      longer exists in gateway's watch relay in the same shape after
      Phase 4 -- worth re-auditing once the client-go gap is resolved.
      Out of Phase 6's cost-gate scope (not exercised by that job).
- [x] No billable DO operations (alarm firing, facet access, etc.) occur
      over a sustained period -- alarm firing is verified directly
      (above). Facet access isn't measured directly, but is covered by
      construction: a Durable Object only ever executes in response to
      `fetch()`/`alarm()`/a WebSocket message, so "no alarm scheduled"
      plus "zero incoming requests over the idle window" (both asserted
      by `.github/workflows/cost-gate.yml`) together leave no code path
      that could reach a facet either.

As of Phase 6 (2026-07-03): alarm-parking and the "nothing runs while
idle" invariant are both enforced automatically in CI on every PR that
touches the relevant paths (`.github/workflows/cost-gate.yml`), not just
verified once by hand. WebSocket hibernation remains the one open item,
blocked on the client-go transport gap documented above and in the
WatchHub redesign commit -- unrelated to what Phase 6 added.
