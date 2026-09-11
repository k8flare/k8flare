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

| Component                    | Idle monthly cost (target: ~0)                                                                                                                                                                                                    | Active unit cost                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | Estimate                                               | Actual                                                                                                                                                                                                                                                                                         |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| gateway (TS Worker)          | ~0 (stateless, no DO)                                                                                                                                                                                                             | Workers request billing (actual CPU time)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | TBD                                                    | Not yet done (not implemented, Phase 2)                                                                                                                                                                                                                                                        |
| apiserver (Go WASM)          | ~0                                                                                                                                                                                                                                | Workers request billing (actual CPU time). Whether the WASM startup tax applies depends on S5's outcome (isolate singleton-ization, see `docs/platform-verification.md` S5)                                                                                                                                                                                                                                                                                                                                                                                   | TBD (pending S5)                                       | Not yet done (not implemented, Phase 2)                                                                                                                                                                                                                                                        |
| storage: Cluster DO          | Target: storage cost only (alarm parked)                                                                                                                                                                                          | DO requests / duration GB-s / rows read-written + alarm invocation count (event-armed only). A namespaced key op now costs 1 parent request + 1 facet request (GET/list) or 1 parent write + 1 facet `/apply` write (PUT/DELETE) -- roughly double the DO-request count of the pre-facet single-table design, in exchange for per-namespace storage headroom. Loader cost is a flat $0.002/day regardless of namespace count: one generic facet class, one content-hash loader key, reused via `ctx.facets.get(name, ...)` for every ns/events/ca-vault facet | TBD                                                    | See Actual: idle alarm parking measured, Phase 4                                                                                                                                                                                                                                               |
| storage: WatchHub DO         | ~0 (while hibernating)                                                                                                                                                                                                            | DO requests: 1 `/push` per write that touches a watched key (from Cluster, awaited synchronously) + 1 `/replay` per new client connection (to Cluster) + N WebSocket sends per push (N = matching connected clients). No upstream connection to hold open (see design note below), so nothing keeps WatchHub resident between events -- nothing added to WatchHub's own idle cost beyond the DO-request cost of whichever push/replay call last touched it                                                                                                    | TBD                                                    | See Actual below, Phase 4                                                                                                                                                                                                                                                                      |
| runtime (TS Worker + LOADER) | ~0 (when cron hasn't fired)                                                                                                                                                                                                       | Workers request billing + Worker Loader $0.002/unique/day                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | TBD                                                    | Not yet done (not implemented, Phase 2)                                                                                                                                                                                                                                                        |
| controllers                  | See "controllers execution path: two-route estimate" below                                                                                                                                                                        | Same + Worker Loader $0.002/unique/day (KCM runs as a Loader-loaded dynamic worker, supplied from Static Assets)                                                                                                                                                                                                                                                                                                                                                                                                                                              | Same                                                   | Execution shape verified working (Phase 5 + S14 ASSETS+LOADER deploy path, local `wrangler dev`); the 10MiB-gzip deploy blocker is resolved (see "ASSETS+LOADER deploy path" below), production $ still unmeasured. kube-scheduler is host-process/BYO-VM-only (does not compile for GOOS=js). |
| nodes (Pod-on-Containers)    | Target ~0 plus one mandatory, unconditional cost: VirtualNode DO's Lease-renewal alarm fires every ~10s for as long as the virtual node is registered, whether or not any Pod exists (see "Phase 7 (nodes) implementation" below) | Containers vCPU/GiB-second billing. **Each running Pod container implies a DO alarm firing at least every ≤3 minutes** while it's up — confirmed by reading the `@cloudflare/containers` self-monitoring source (`spikes/s3-containers/FINDINGS.md`); it re-arms on that cadence while running and calls `deleteAlarm()` once stopped, so it parks when idle (event-armed rule satisfied, but not zero-alarm while any Pod is running). **The Pod's own running cost is the user's workload cost** (cost invariant #6, not counted as control-plane cost)     | See "Phase 7 (nodes) implementation" below             | Implemented (this phase); $ not yet measured (no `wrangler deploy` performed)                                                                                                                                                                                                                  |
| R2 PV/PVC                    | Target: storage cost only (achieved -- see "Phase 8 (R2 PV/PVC backend) implementation" below: nothing control-plane-side runs, alarms, or costs anything while no PVC is ever created)                                           | R2 storage + Class A/B ops (the Pod's own reads/writes, cost invariant #6 -- user workload cost, not control-plane) + 1 Worker-to-Worker request per container (re)start to mint a credential (CPU-time-billed only, no R2 charge -- local JWT signing)                                                                                                                                                                                                                                                                                                       | See "Phase 8 (R2 PV/PVC backend) implementation" below | Bind/mint mechanism implemented and verified end-to-end against real wrangler dev + Docker (this phase); no real R2 bucket/account billing measured yet (see below for why)                                                                                                                    |

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
limit (historical: removed 2026-09, see platform-verification.md S35 —
the ASSETS+LOADER channel is still required, on the 64 MiB raw limit)
blocks deployment before any cost measurement against production
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

### ASSETS+LOADER deploy path (2026-07-05): the size blocker above is resolved; cost shape updated

The "~19MiB gzip vs 10MiB deploy cap" blocker in the previous subsection
no longer applies: the KCM WASM is no longer part of the Worker script
bundle at all. It ships as Static Assets (three ≤24MiB chunks + manifest)
and is assembled + run through the Worker Loader at request time
(`workers/controllers/src/index.ts`, `scripts/build-controllers-wasm.sh`;
verification: `docs/platform-verification.md` S14). The binding
constraint is now the Loader's hard 64MiB raw cap, satisfied at 59.6MiB
after `-s -w` + `wasm-opt -Oz`.

Cost model for this shape (all still local-verified only; production
unmeasured):

- **Idle cluster**: unchanged, ~0 — no pokes ⇒ the dynamic worker is
  never dispatched, the DO safety-net alarm self-parks when no live
  Nodes exist, and Static Assets storage is free-tier-class.
- **Worker Loader**: $0.002/unique loaded worker/day while the cluster is
  active (one unique id per KCM binary hash). Isolate idle-eviction
  cadence in production is unverified — each re-load re-fetches ~60MB
  from ASSETS and re-runs informer initial sync (requests against
  gateway/apiserver/Cluster DO), so eviction frequency, not the $0.002
  itself, is the number to measure first in production.
- **Pump windows**: each poke (relevant write / safety-net alarm) opens a
  bounded 25s `waitUntil` window in the dynamic worker. I/O waits inside
  the window are not CPU-billed; billing tracks actual reconcile CPU,
  preserving route A's cost hypothesis. Event-armed only — no poke, no
  window.
- **Deploy-time**: `wasm-opt -Oz` adds ~2 min to `build:wasm:controllers`
  (build cost, not runtime cost); enforced as a hard gate because an
  over-64MiB binary fails at Loader time, not deploy time.

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
| Node PodCIDR allocation _(superseded 2026-09-09, see the note below)_  | Synchronous, apiserver's Node-create path (`pkg/apiserver/nodecidr.go`)                                                    | `k8s.io/kubernetes/pkg/controller/nodeipam/ipam/cidrset.CidrSet` (leaf package, no client-go)                                                                                                   | +11KB (isolated); +35KB (real feature, incl. CAS persistence glue)      |
| Endpoints/EndpointSlice _(superseded 2026-09-09, see the note below)_  | Synchronous, on every Service/Pod write incl. the `/status` subresource (`pkg/apiserver/endpoints.go`)                     | `k8s.io/endpointslice/util` (`IsPodReady`/`ShouldSetHostname`, separate lightweight package); `labels.SelectorFromValidatedSet`; small hand-ported functions where the real ones are unexported | +34KB (isolated util import); +97KB (real feature)                      |
| Node lifecycle (Lease staleness → Unknown+taint+evict) _(superseded 2026-09-09, see the note below)_ | Cluster DO's existing event-armed safety-net alarm pings a new internal apiserver route (`pkg/apiserver/nodelifecycle.go`) | Real grace-period defaults (`nodelifecycle/config/v1alpha1`), real taint ops (`pkg/util/taints`), real toleration matching (`corev1.Toleration.ToleratesTaint`)                                 | +53KB (full feature, incl. the two lightweight upstream packages above) |
| **All three combined** _(all superseded 2026-09-09; kept per rule 4, deleted from the code)_ | —                                                                                                                          | —                                                                                                                                                                                               | **+150,104 bytes (146.6KB), 7,699,133 → 7,849,237 bytes gzip**          |

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

**Superseded (2026-09-09)**: all three rows in the table above were
deleted and handed back to the real kube-controller-manager controllers
in the kcm dynamic worker (`endpoint`/`endpointslice`/`nodeipam`/
`nodelifecycle`/`tainteviction`), which cost +1.87MiB on the kcm chunk and
left it 21.8MiB under the Loader cap — the size argument that forced the
synchronous apiserver-side versions no longer holds for kcm (it still
holds for the apiserver chunk, which has 2.57MiB of headroom). Numbers,
what was verified, and what was NOT (production 128MiB isolate memory) are
in docs/platform-verification.md's S28.

**Alarm accounting (corrected the same day)**: this note first claimed the
accounting above was "unchanged". It is not, and the first attempt at it
was a cost regression. What holds now: while at least one Node is live,
each 60s safety-net tick costs one Cluster DO alarm plus **one kcm pump**
— the tick sends the Controllers DO a dedicated alarm-origin poke
(`/safety-net/node-lifecycle`) that only ensures and pumps the kcm
component, with `armWarmup:false`. It deliberately does NOT reuse the
write-path poke (`pingControllers`), which arms a 3-minute warmup window,
resets the unconverged backoff, and re-arms the Controllers DO's own 60s
alarm: doing that on every tick revives exactly the "an idle BYO node kept
a pointless 60s chain alive" regression `controllers/index.ts`'s `alarm()`
comment records (cost invariants #1/#3). So the steady state with Nodes is
one alarm + one pump per minute and **no Controllers DO alarm chain**, and
everything parks once the last Node is gone. Measured against real
`wrangler dev` rather than asserted (60.9s between consecutive pokes with
one Node, zero pokes in the 150s after deleting it) — S28 has the numbers.

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
   for this task. Net effect: with zero Pods, each 10s tick costs 4 Worker
   requests to `workers/apiserver` (`getNode` existence check, `getLease`,
   `renewLease` or `createLease`, `listPodsForNode`) and no Container
   activity — 8,640 alarm firings/day (259,200/month) × 4 requests/tick ≈
   **1,036,800 apiserver requests/month per registered virtual node**,
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
~1,036,800 requests/month figure above is a request-_count_ estimate from the
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

## Phase 8 (R2 PV/PVC backend) implementation

**REMOVED 2026-07-08:** this entire custom R2-backed dynamic provisioner
(`pkg/apiserver/r2.go`, `r2handlers.go`, `pvcbind.go`) was deleted --
user decision to stop hand-rolling a synthetic-CSI-named, synchronous
bind mechanism and instead rebuild PV/PVC provisioning later using a
real CSI driver (k8s's standard mechanism), once CRD support exists.
`StorageClass`/`PersistentVolume`/`PersistentVolumeClaim` remain generic
CRUD resources in `apidef.Table`; no StorageClass is bootstrapped or
marked default, so PVCs stay `Pending` exactly like a real cluster with
no provisioner configured. The section below is kept as a historical
record of the removed implementation and its cost figures -- see git
history for the actual code.

`StorageClass`/`PersistentVolume`/`PersistentVolumeClaim` are added to
`apidef.Table` (both `corev1`/`storagev1`, already-served groups, so no new
mux route or Scheme registration was needed -- adding the table rows was
the entire wiring, confirming the Phase 3 table-driven design's whole
point). A PVC naming (or defaulting to) the "r2" `StorageClass` gets a
`PersistentVolume` created and bound **synchronously, in the same request**
(`pkg/apiserver/pvcbind.go`), the same "allocate before the first write"
shape as `AssignClusterIP`/`AssignPodCIDR`, adapted for a two-object
relationship -- see that file's package doc comment for exactly how.

**No new alarm, no new DO, no new always-on process.** PVC bind runs
synchronously inside the same HTTP request that creates the PVC (a
`ResourceStore.Create`/`Update` pair against the already-existing Cluster
DO, billed under that DO's existing request/duration/rows-read-written
model -- introducing zero new billing primitives). Nothing runs, and no
alarm fires, for a cluster that never creates a PVC: this satisfies cost
invariant #1 by construction, not by a separate idle-check (there is
nothing to idle-check -- Phase 8 added no periodic process at all).

**Credential minting is a pure local HMAC computation, not an R2
operation.** `pkg/apiserver/r2.go`'s `MintCredential` builds and HS256-signs
a JWT locally (`crypto/hmac`+`crypto/sha256`, Go stdlib only -- no new
dependency, keeping the WASM gzip size impact to +1,459 bytes for the whole
PV/PVC/StorageClass/R2-signing feature: 7,849,237 -> 7,850,696 bytes,
measured directly). This makes no network call to Cloudflare at all, so it
is neither a billed R2 Class A/B operation nor a draw against the shared
1,200 req/5min R2 REST management-API budget (`spikes/s6-r2/RESEARCH.md`) --
exactly the property that recommendation was chosen for. The one added
per-container-(re)start cost is a single Worker-to-Worker service-binding
request (`workers/nodes` -> `workers/apiserver`'s
`/internal/mint-r2-credentials`), billed as ordinary Workers CPU time (a
few HMAC/JSON operations), not a new pricing primitive.

**Credential-refresh mitigation adds no new alarm either.** The
proactive-restart-before-expiry mechanism (`workers/nodes/src/
virtualnode.ts`, see its "Credential refresh" doc comment) rides
`VirtualNode`'s existing ~10s Lease-renewal-driven reconcile tick (already
priced under "Phase 7 (nodes) implementation" above) -- it does not arm a
second alarm source. Its only cost is the Container restart cycle itself
(a `stop()`+`ensureRunning()` pair) once per credential TTL window (1 hour
by default) for a long-running `restartPolicy: Always` Pod that mounts a
PVC -- Pod workload cost (invariant #6), the same bucket a Pod's normal
vCPU/GiB-second billing already falls into, not control-plane cost.

**R2 pricing** (Standard tier, from the Billing primitives table above,
sourced from `spikes/s6-r2/RESEARCH.md`): $0.015/GB-month storage, $4.50/M
Class A ops (writes/lists), $0.36/M Class B ops (reads), egress and all
deletes free and uncapped, 10GB-month + 1M Class A + 10M Class B/month free
tier. These are the Pod's own S3 API calls against its PVC's prefix --
genuinely new cost surface Phase 8 introduces, and squarely the user's Pod
workload cost (invariant #6), not counted against the control plane's
idle-~0 target.

**What was and wasn't verified against a real account.** Per this task's
verification gate and CLAUDE.md rule 2 ("actually run it"), the bind ->
mint -> inject mechanism was verified end-to-end against real `wrangler
dev` + real Docker (not a mock): a PVC was created and bound, a Pod
mounting it was scheduled, and `docker inspect` on the actual running
container confirmed correctly-scoped `AWS_ACCESS_KEY_ID`/
`AWS_SECRET_ACCESS_KEY`/`AWS_SESSION_TOKEN`/`R2_ENDPOINT`/`R2_BUCKET`/
`R2_PREFIX` env vars, and the real minted JWT was decoded and its
`paths.prefixPaths` confirmed to match exactly that PVC's prefix. The JWT
signing implementation itself was additionally cross-checked against an
independent Node.js reference implementation and a from-scratch Python HMAC
verification (`pkg/apiserver/r2_test.go`). **Not verified**: an actual
authenticated S3 call against production R2 using a minted credential.
Doing so needs a parent R2 API token (an Access Key ID / Secret Access Key
pair), and this task found no way to create one that stays within this
project's established "use `wrangler`'s existing OAuth session, never
extract or fabricate account credentials from its local config" boundary --
R2 API tokens are dashboard- or Cloudflare-REST-API-created
(`developers.cloudflare.com/r2/api/tokens/`), and no `wrangler` subcommand
exposes that operation. This is the same class of residual item
`spikes/s6-r2/RESEARCH.md` already flagged (its residual item 5): "whether
this OAuth session ... can actually create a bucket / R2 API token /
temporary credential -- only listing was verified." Recorded here rather
than silently skipped (CLAUDE.md rule 4). **Manual completion path for
whoever has dashboard access** (~5 minutes): Cloudflare dashboard -> R2
object storage -> Manage API Tokens -> create a token scoped to one bucket,
Object Read & Write permission; set the resulting Access Key ID/Secret
Access Key/bucket name/account ID as `R2_ACCOUNT_ID`/`R2_ACCESS_KEY_ID`/
`R2_SECRET_ACCESS_KEY`/`R2_BUCKET` via `wrangler secret put` (or `.dev.vars`
for local testing) on `workers/apiserver`; then repeat this section's PVC
create -> Pod create -> `docker inspect`-decoded-JWT flow and additionally
issue one real signed S3 request (`aws4fetch`/`boto3`/`aws-cli` all work,
per R2's docs) using the minted triple against
`https://<account>.r2.cloudflarestorage.com`; delete the test bucket
afterward.

## Single-Worker consolidation + multi-cluster (estimate, 2026-07-06 — pre-implementation per invariant #5)

The 6→1 Worker consolidation (S19 spike gates in
`docs/platform-verification.md`) and path-prefix multi-cluster change
costs as follows. Worker count itself is not billed, so consolidation
is cost-neutral by default; the deltas are:

| Item                                                                                  | Delta                                                                | Estimate                                                                                                                                                                           |
| ------------------------------------------------------------------------------------- | -------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| apiserver as Loader dynamic worker                                                    | +1 unique loader id per ACTIVE cluster-day                           | $0.002/cluster/active-day; $0 idle (never loaded). Rebuild day = 2 ids briefly.                                                                                                    |
| KCM/sched/gc per-cluster loader ids (multi-cluster)                                   | ids become `kcm:<doName>@sha` (and `gc:<doName>@sha`)                | $0.002 × loaded binaries × active clusters/day (today: kcm + gc = 2 global ids; sched still unshipped, see below). Idle clusters load nothing.                                     |
| OpenAPI assets served via `env.ASSETS.fetch` from the Worker (run_worker_first: true) | request + CPU on discovery paths that previously bypassed the Worker | Negligible: kubectl discovery bursts only (~30 reqs/invocation), still asset-store-backed; no isolate-size cost.                                                                   |
| apiserver dynamic-worker warm dispatch                                                | per-request `LOADER.get` (factory skipped) + entrypoint hop          | S19 measured warm 12–24 ms wall (dev), CPU-billed portion unchanged — the per-request Go instantiation is the same work today's `worker.mjs` does.                                 |
| Static assets storage                                                                 | kcm (~60MB) + apiserver (~43MB) chunks + openapi                     | assets are free/flat (no per-GB assets fee at current pricing); no change to R2/DO storage.                                                                                        |
| Registry DO (multi-cluster)                                                           | 1 tiny sqlite DO, no alarm, no WS                                    | Idle = storage only (KBs). Reads on cluster-resolution cache misses only.                                                                                                          |
| Per-cluster token vault reads                                                         | +1 facet read per verifier isolate per ~60s TTL on cache miss        | Bounded by isolate count × TTL; rows-read pennies at any realistic scale.                                                                                                          |
| Poke paths (storage→controllers/nodes)                                                | service-binding hop → direct DO binding call                         | Slightly FEWER billed requests (one Worker invocation removed per poke).                                                                                                           |
| Cluster teardown leaves R2 objects (multi-cluster v1)                                 | objects under `clusters/<doName>/` survive `DELETE /clusters/<id>`   | R2 storage $0.015/GB-mo until a cleanup pass exists (S3 SigV4 deletion isn't wired into the Worker). R2 delete ops are free, so deferring loses nothing beyond the storage itself. |

Idle invariants unchanged: no new alarms, no fixed-interval polling, no
non-hibernating sockets; an idle cluster still costs ~storage only.
Numbers to replace with measurements after implementation: production
loader eviction cadence (cold frequency — S14 open item), actual
per-request CPU-ms of the loader-path apiserver vs today, Registry DO
rows/month under real cluster-resolution traffic.

## Cloudflare Mesh for kubelet proxy (actual, 2026-07-07)

Replaces Cloudflare Tunnel + Workers VPC Service for the `kubectl
logs`/`kubectl exec` path (`spikes/s17-mesh-nodevm/FINDINGS.md`,
`docs/cloudflare-mesh-networking.md`'s 2026-07-07 addendum). No
additional Cloudflare-side billing surface found in Workers VPC's own
pricing docs beyond what a `vpc_networks` binding already costs
(request/CPU time on the Worker side, same as any other fetch — this
binding type has no documented per-request premium as of 2026-07,
unlike Containers). The `cloudflare-warp` client on each BYO VM node is
free software running on infrastructure the cluster already needed
(the node itself); no new billed Cloudflare resource per node (a
`warp_connector` tunnel resource has no listed price, unlike a
Containers instance). Numbers to replace with measurements: actual
Worker CPU-ms per kubelet-proxy request via `MESH.fetch()` vs. the
prior `KUBELET_VPC.fetch()` (Tunnel-relayed) path, and whether Workers
VPC exits its public beta with a different pricing model.

## Cluster DNS (actual, 2026-07-07)

`pkg/dnsshim` (cmd/agent) + `pkg/apiserver`'s `/dns-query` DoH endpoint
(docs/general-purpose-k8s-plan.md's Phase 4). No new resident process on
the Cloudflare side, no Containers, no CoreDNS Deployment Pod — the
Worker-side half is just another apiserver request, same billing shape
as every other API call (Worker CPU-time-only, zero cost while idle).
The node-local shim runs on infrastructure the cluster already needed
(the BYO VM's own `cmd/agent` process) rather than adding a new billed
component, so this feature has **no incremental Cloudflare cost** at
any traffic level. Idle invariant #1 holds trivially (nothing new to be
idle).

## Demand-start Containers control plane (estimate, 2026-07-07 — pre-implementation per invariant #5, GATED on this estimate per user decision)

Containers GA (2026-04-13 changelog) changed the billing split this
document's older sections assume: **CPU is now billed on active usage
only** ($0.000020/vCPU-s, 10ms granularity); **memory and disk stay
provisioned-wall-clock while the instance is awake** ($0.0000025/GiB-s,
$0.00000007/GB-s); **a sleeping container is billed nothing**. Workers
Paid includes 375 vCPU-min + 25 GiB-h + 200 GB-h per month.
CLAUDE.md's cost invariant #2/#7 wording ("Containers are wall-clock
billed") predates this and should be read as "memory/disk are
wall-clock while awake" — corrected here, not silently.

Proposal being estimated (task #23): run the real linux kube-scheduler

- kube-controller-manager binaries in ONE demand-start container per
  active cluster, replacing the 64MiB-capped KCM WASM and the BYO-VM
  scheduler requirement, reusing the existing poke/park machinery to
  start the container on work and `stop()` it on drain.

| Scenario ("basic" 1 GiB / 0.25 vCPU / 4 GB disk instance)       | Memory (wall-clock while awake)        | CPU (active only)                          | Monthly $/active cluster                                 |
| --------------------------------------------------------------- | -------------------------------------- | ------------------------------------------ | -------------------------------------------------------- |
| Awake ~1 h/day (event-armed start/stop, typical active cluster) | 108,000 GiB-s ≈ allowance +$0.05       | ~1,350 vCPU-s, inside allowance            | **~$0.05–0.30**                                          |
| Awake ~6 h/day (busy cluster)                                   | 648,000 GiB-s ≈ $1.40 beyond allowance | still pennies (CPU is idle-waiting mostly) | **~$1.5**                                                |
| Always-on (informers pin it awake — the failure mode)           | 2.63M GiB-s ≈ $6.4 + disk $0.7         | ~$0.3                                      | **~$7.4 — VIOLATES invariant #1/#2, must not ship this** |

The decisive design constraint: the scheduler/KCM hold long-lived
watches, which keep the container awake indefinitely if left alone —
the event-armed pump-window discipline the WASM KCM already follows
must map onto container start/stop (start on poke, drain, `stop()`;
`onActivityExpired` ask-before-sleep as the safety net). Idle cluster
= container stopped = $0, preserving invariant #1. Hot paths stay off
Containers (invariant #7 intact: scheduler/KCM are async reconcilers).
Numbers to replace with measurements: actual awake-fraction under the
poke cadence, actual active-CPU per reconcile burst, cold-start delay
(typical 1–3 s) added to first Pod schedule after idle.

## Virtual kube-proxy + pods/proxy bridge (estimate, 2026-07-07 — pre-implementation per invariant #5, task #13)

Pod-on-Containers Pods run `hostNetwork: true` (`pkg/apiserver/computeclass.go`,
shipped 2026-07-06) because the microVM sandbox has no working netfilter, so
the real k3s kube-proxy embedded in `cmd/agent` cannot program NAT rules for
these nodes even though it's enabled cluster-wide
(`DisableKubeProxy: false`). Two request-driven bridges close the resulting
gaps, neither of which adds a resident process or a poll loop:

1. **`pods/proxy` / `services/proxy`-style ingress** (Worker-side only): the
   gateway already routes `/api/v1/namespaces/{ns}/pods/{name}/proxy/...`
   into `nodes/index.ts`'s `handleNodes` (`gateway/index.ts`); this task
   replaces its `501` stub with a real resolve-and-`containerFetch` path
   (Pod → tracked NodeVM → its container port). **Cost: zero new
   components.** This is exactly the same Worker request + DO `fetch()` +
   Containers `containerFetch()` shape the already-measured kubelet
   logs/metrics bridge uses (see "Phase 7 (nodes) implementation" below) —
   billed as ordinary Worker/DO request CPU time, no new alarm, no new
   Container instance (it reuses the Pod's own already-running NodeVM).
2. **ClusterIP traffic from inside a Pod-on-Containers Pod** (`10.43.0.0/16`,
   `ServiceCIDR`): a new node-image-only component,
   `pkg/vkubeproxy` (linked into `cmd/agent`, gated behind a CLI flag only
   passed on the Containers node image's entrypoint — BYO VM nodes keep
   using the real kube-proxy, unaffected). It owns a TUN device inside the
   microVM's own network namespace (shared with the Pod under
   `hostNetwork`), terminates TCP connections addressed to `ServiceCIDR`
   with an embedded userspace TCP/IP stack
   (`gvisor.dev/gvisor/pkg/tcpip`, the same "real library, don't
   reimplement TCP" call this project already made for
   kube-scheduler/kube-controller-manager — rule #3), and forwards each
   HTTP request over an ordinary outbound HTTPS call to the new
   `/nodes/vkubeproxy` Worker endpoint (Service+EndpointSlice resolution,
   same `containerFetch` backend path as bridge 1 above).

   **Cost: no new Cloudflare-billed component at all.** This process runs
   _inside_ the Containers instance the Pod's own NodeVM already pays for
   (cost invariant #6: that instance's vCPU/GiB-second billing is the
   user's workload cost regardless of whether this forwarder exists) — it
   adds a small amount of CPU time to that already-running instance per
   connection, and it is entirely demand-driven (a TCP SYN triggers work;
   no idle-timer, no poll). On the Worker side, each proxied HTTP request
   is exactly one more `containerFetch`-shaped request/DO-fetch, same unit
   cost as bridge 1. Idle Pods (no ClusterIP traffic) cost nothing beyond
   what their NodeVM already costs today.

   **What this does NOT do**: no new DO, no new alarm, no persistent
   WebSocket from the node (each proxied request is a plain outbound
   HTTPS call — no hibernating connection to keep warm, so cost invariant
   #4 doesn't even apply here). v1 scope is HTTP/1.x request-response only
   (matches the design's explicit deferral of raw TCP passthrough to a
   later v2); TokenReview-gated secure kubelet (10250, `exec`/`attach`) and
   raw-TCP Services are both out of scope for this pass, same as bridge 1.

Numbers to replace with measurements: incremental container CPU-ms per
proxied HTTP request (netstack TCP handling + one outbound fetch),
`containerFetch` request count added per Service call. Not yet measured —
see the "what's verified" note this task's implementation commit records
in `docs/general-purpose-k8s-plan.md` and `docs/platform-verification.md`
for what could and couldn't be exercised end-to-end on this pass (ARM Mac
local Docker cannot boot this node image at all, a pre-existing limitation
recorded in S16 — ​not specific to this feature).

**Update (2026-07-07, docs/platform-verification.md S20)**: bridge 1 and
bridge 2's mechanism are both implemented and functionally proven against
real primitives (a real `/dev/net/tun` device + gVisor netstack in a
privileged Linux container for bridge 2's intercept; a real `wrangler dev`

- real Service/EndpointSlice objects for the Worker-side resolution both
  bridges share) — see S20 for what exactly was and wasn't exercised. This
  confirms the cost shape reasoned about above (no new DO, no new alarm, no
  persistent connection) is what actually got built, not just what was
  planned. Per-request CPU-ms numbers are still not measured — that requires
  a real deployed NodeVM, which this pass could not produce (same ARM
  Mac/Rosetta local-Docker limitation as before; `wrangler deploy` is outside
  this session's constraints).

### Scheduler-as-Loader-dynamic-worker: cost shape if it ever fits (2026-07-07)

Recorded alongside `docs/platform-verification.md`'s S8
kube-scheduler-wasm-fork entry (2026-07-07): a session narrowed
`informers.SharedInformerFactory` from 19 to 6 groups, closing the wasm
binary from 102.8MB to 101.1MB opt -- still ~34MB over the Worker
Loader's 64MiB cap, so **this remains a size question, not yet a cost
question** (the Containers-based estimate above, "basic instance", still
applies as the only currently-fitting execution shape). If a future
session closes the remaining gap (see that doc entry's scoped follow-up),
the cost shape would be **identical to KCM's already-measured Loader
dynamic-worker profile** (`controllers` row, cost table top of this
file): Worker Loader's flat $0.002/unique-code/day (one more unique
`sched.wasm` hash alongside `kcm.wasm`/`apiserver.wasm`'s existing
per-hash charges) plus ordinary Workers CPU-time billing for the
resident scheduling loop, pumped by the same event-armed
poke/`waitUntil`-window discipline already governing KCM (invariant #3)
-- no new cost primitive, since a Loader-loaded WASM binary is billed
identically regardless of which control-plane component it runs. This
would be strictly cheaper than the Containers alternative above (no
wall-clock container awake-time, no cold-start delay after idle,
CPU-time-only billing per invariant #2) if and when it fits -- the
entire remaining blocker is the 64MiB cap, not a cost-model concern.

## Per-Pod Cloudflare Mesh membership (estimate + partial actual, 2026-07-08 -- pre-implementation per invariant #5, spikes/s17-mesh-nodevm/FINDINGS.md)

Gives each Pod-on-Containers Pod its own Cloudflare Mesh IP
(`100.96.0.0/12`), independent of whatever IP the underlying NodeVM has,
by having `cmd/agent` join Mesh (already-shipped `pkg/meshconnector`,
reused unmodified) as part of its own boot sequence, with the connector
token minted FRESH per Pod at schedule time (`nodes/meshconnector.ts`,
new) instead of a human pre-provisioning one long-lived connector per
BYO VM. No sidecar container, no shared-network-namespace Pod sandbox
work needed: `hostNetwork: true` already merges every process on this
backend's one-Pod-per-microVM into one network namespace, and real
kubelet source (`k3s-io/kubernetes@v1.36.2-k3s1`'s
`pkg/kubelet/kubelet_pods.go`'s `getHostIPsAnyWay`, read directly to
confirm this, not assumed) inherits hostNetwork Pods' `status.PodIP`
from the Node's own InternalIP -- which k3s's `--node-ip` (not
`--node-external-ip`, the flag the already-shipped BYO-VM Mesh feature
uses for a different purpose) feeds. See FINDINGS.md's dated entry for
the full design/verification writeup this summarizes.

**New cost surfaces, none of which add a new billed Cloudflare
component (matches the "Cluster DNS" and "Virtual kube-proxy" sections
above's shape: reuse of infrastructure the Pod already pays for):**

1. **`warp-svc` running inside the same already-wall-clock-billed
   Containers instance.** Adds real vCPU/memory to what each NodeVM's
   size tier must fit, on top of the k3s agent (kubelet+containerd) and
   the Pod's own container this instance already runs. Not yet measured
   against a live NodeVM (this session could not deploy one -- same
   ARM-Mac/no-real-Containers-runtime gap already on record for the
   virtual-kube-proxy section above); measure per-tier headroom before
   enabling this by default on the `small` tier (63 milliCPU / 256MiB,
   images.ts) in particular, since that is the tightest fit already.
   **New, surprising finding this pass**: the `cloudflare-warp` Debian
   package pulls in a large GUI dependency tree (GTK3/WebKit/GStreamer,
   `--no-install-recommends` notwithstanding) that is very likely dead
   weight for a headless connector-only use -- confirmed by actually
   building the switched-to-`debian:bookworm-slim` node image
   (`packages/k8flare-worker/images/node/Dockerfile`) locally: the image grew to
   **1.48GB** (vs. the prior Alpine base's much smaller footprint,
   effectively all statically-linked Go + a bare k3s binary). This is a
   one-time-per-deploy image pull cost, not a per-Pod cost, but it likely
   lengthens the FIRST NodeVM cold start after any node-image deploy
   (Containers pulls/caches the image once, not per instance) -- not
   measured this pass; worth a follow-up to see whether a slimmer
   packaging (e.g. hand-extracting just the `warp-svc`/`warp-cli`
   binaries and their actual runtime library deps, skipping `apt`
   entirely) is worth the added maintenance.
2. **Cloudflare API calls to mint/delete `warp_connector` resources**
   (`nodes/meshconnector.ts`'s `createMeshConnector`/`deleteMeshConnector`,
   one POST+GET pair per Pod schedule, one DELETE per Pod teardown).
   Checked this pass, not assumed (invariant #5): Cloudflare's
   [account-limits docs](https://developers.cloudflare.com/cloudflare-one/account-limits/)
   and [Mesh docs](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/)
   list no per-connector price and no API-call price for
   create/list/token/delete -- consistent with the existing "Cloudflare
   Mesh for kubelet proxy" section's already-recorded finding for the
   BYO-VM path. The calls themselves cost only the calling Worker's own
   ordinary request/CPU-time billing (`cf-containers-scheduler`'s
   `reconcile()`, already running regardless). No Cloudflare-documented
   rate limit specific to `warp_connector` was found either; the
   account-wide general API rate limit (not warp_connector-specific)
   would be the only throttling concern at very high Pod
   churn -- not expected to matter at this project's current scale, not
   measured against real sustained throughput.
3. **The 50-Mesh-nodes/account cap now bounds concurrent PODS on this
   backend, not concurrent NodeVMs (FINDINGS.md, honestly recorded, not
   new but re-scoped by this change).** Before this feature, Mesh wasn't
   used per-Pod at all, so the cap was irrelevant to this backend. After:
   the 51st simultaneously-scheduled Pod on the Containers backend would
   fail to get a connector -- `createMeshConnector` degrades gracefully
   (the Pod still boots, just without Mesh membership, matching
   `pkg/vkubeproxy`/`pkg/dnsshim`'s existing "log and continue" posture
   for their own optional enhancements), so this is a **quality
   degradation under the cap, not a hard scheduling failure** -- but it
   is a real, load-bearing capacity ceiling for any deployment that wants
   more than 50 concurrent Pods on this backend all Mesh-connected,
   worth surfacing to an operator (e.g. a metric/log line) rather than
   only discovering it silently, a follow-up not yet built.

**Not yet measured / verified against real infrastructure this pass**
(no `CLOUDFLARE_API_TOKEN` was available in this sandbox to exercise
`nodes/meshconnector.ts` against the real KOOFFICE account, only a
`wrangler`-OAuth session whose reuse outside `wrangler`'s own CLI
surface this session declined, same reservation FINDINGS.md's own
2026-07-07 correction already recorded): a real Pod's `status.podIP`
actually becoming a live Mesh IP end-to-end. `cmd/agent`'s
`--mesh-ip-as-node-ip` wiring and the whole node-image boot sequence
(warp-svc start, readiness wait, `meshconnector.Run`) WERE verified for
real in a privileged Docker container built from the actual switched
Dockerfile, using the actual entrypoint.sh and a real cross-compiled
`k8flare-agent` binary -- with an intentionally-invalid connector token,
which fails fast and cleanly (`Error: Failed to parse WARP Connector
token`, propagated through `log.Fatalf` in well under a second, not a
hang), proving the wiring reaches the real `warp-cli` call correctly.
See FINDINGS.md for the full transcript-backed writeup.

**Update (2026-07-08): the Worker-side API calls ARE now verified for
real**, closing part of the gap above. With a real `CLOUDFLARE_API_TOKEN`
made available for this one verification pass, `nodes/meshconnector.ts`'s
exact three calls were run directly against the real KOOFFICE account
(`POST /accounts/{id}/warp_connector`, `GET .../token`,
`DELETE /accounts/{id}/warp_connector/{id}`), matching gate 2's original
proof method: create succeeded (`"success":true`, real connector id
returned), token fetch succeeded (`"success":true`), delete succeeded
(`deleted_at` set), and a follow-up list call confirmed the connector no
longer exists -- no leaked capacity against the 50-node cap. **Still not
verified**: this test only proves the Worker's own API client code path;
it did not exercise `cf-containers-scheduler`'s actual `reconcile()`
call site, and it still did not touch a real running NodeVM, so "a real
Pod's `status.podIP` becoming a live Mesh IP end-to-end" remains open,
same as recorded above.

**Update (2026-07-08, later same day): the remaining gap IS now closed --
by deploying to real Cloudflare Containers and finding a real,
platform-level blocker, not a code bug.** Per user authorization to
deploy for this one verification pass (cleaning up every real resource
created afterward -- see `spikes/s17-mesh-nodevm/FINDINGS.md`'s matching
entry for the full transcript-backed writeup): real Pods were scheduled
end-to-end against the live `k8flare` Worker. Two real, reproducible
concurrency bugs were found and fixed in `cf-containers-scheduler` along
the way (both leaked a Mesh connector against the 50-node cap under
overlapping `reconcile()` invocations -- see the code comments in
`packages/k8flare-worker/src/nodes/scheduler.ts` for the exact mechanism), and
`cmd/agent`'s hard `log.Fatalf` on Mesh-join failure was made non-fatal
(matching this repo's existing "log and continue" posture for optional
enhancements) after it was found to leave a Pod stuck in `Pending`
forever with no Node ever registering, any time the Mesh connection
itself failed.

**With those three fixes in place, Pods now schedule successfully and
degrade gracefully -- but the Mesh connection itself never succeeds on
this backend.** Real diagnostic output pulled from a live container
instance (`warp-cli status`, `warp-cli settings`, `ip addr show
CloudflareWARP`) shows: the account's Zero Trust policy locks WARP to
`WARP tunnel protocol: MASQUE` (QUIC/HTTP-3-based, not classic
WireGuard-over-UDP); the `CloudflareWARP` interface is **never created at
all** ("Device does not exist"); `warp-cli status` reports "Disconnected,
Reason: Manual Disconnection" after the connect attempt; and warp-svc's
own internal watchdog logs "Watchdog reports hung daemon
(watchdog_name=main loop)". Meanwhile a plain `curl https://
www.cloudflare.com` from inside the same container succeeds
(`http_code=200`) -- ordinary HTTPS/TCP egress works fine. This matches
Cloudflare's own Containers outbound-traffic documentation
(`developers.cloudflare.com/containers/platform-details/outbound-traffic/`),
which describes outbound handling in terms of HTTP/HTTPS on ports 80/443
plus DNS, with no mention of UDP/QUIC egress at all. **Conclusion: this
is very likely a platform-level networking constraint (Cloudflare
Containers' egress model does not carry the QUIC/UDP-based MASQUE
tunnel WARP requires), not a bug in this repo's code.** Recorded as the
likely explanation rather than an absolutely certain one, since no
Cloudflare support channel was consulted to confirm it directly --
whoever revisits this should treat "file a question with Cloudflare
about Containers + WARP/MASQUE egress" as the next step before assuming
it can never work.

**Net cost-model conclusion**: per-Pod Mesh membership for the Containers
backend cannot currently be delivered as designed. The three shipped
fixes are real, independent improvements (no connector leaks even under
repeated `FailedScheduling` retries, verified live; Pods no longer get
stuck forever when Mesh fails) and are kept regardless of whether Mesh
itself ever works here, since they also apply to any future retry of
this feature. No new billed component exists today because the feature
does not functionally work yet -- Pods on this backend get a plain
container-internal IP (e.g. `10.0.0.1`), identical in cost shape to
before this feature was ever built.

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

## apiserver Loader-cap headroom: split investigated, not implemented (2026-07-07)

Task #23 (see `docs/platform-verification.md`'s S20 section for the full
investigation) considered splitting `cmd/apiserver-wasm` into multiple
Loader dynamic workers by GroupVersion to relieve the Loader's 64MiB
cap (apiserver was at 2.21MiB headroom). **No split was implemented**
-- measurement showed the per-GroupVersion type surface is not the
dominant cost (removing 10 of 12 GroupVersions saved only 2.48MiB),
while the actually-dominant costs (a ~13MiB `strategicpatch` PATCH
dependency, RBAC authorization, ServiceAccount token authentication)
would have to be duplicated into every split binary anyway, since they
gate every request/every resource today. **Cost-model consequence:
none** -- `packages/k8flare-worker` still loads exactly one apiserver Loader
dynamic worker id per active cluster-day, same as the "Single-Worker
consolidation" estimate above; this investigation did not add a second
one.

The headroom problem was instead relieved by dropping 5 unused
upstream defaulters packages from `cmd/k8flare-gen/defaulters.go`
(zero cost-model impact -- pure binary-size change, no new Loader id,
no new request path): apiserver headroom went from 2.21MiB to 4.05MiB.

## PriorityClass (scheduling.k8s.io/v1) admission (actual, 2026-07-08)

Phase 5 quick win (`docs/general-purpose-k8s-plan.md`): registered
`scheduling.k8s.io/v1`'s `PriorityClass` in `apidef.Table` and added
`pkg/apiserver/priority.go`, which resolves a Pod's
`spec.priorityClassName` to a real `spec.priority` synchronously in the
same `HandleResource` POST path LimitRange defaulting and compute-class
routing already use (`handler.go`) -- no new Worker, no new Durable
Object, no new alarm, no new Loader id: this is pure added Go code inside
the existing apiserver Loader dynamic worker, billed under the same
per-request CPU time it already accrues.

**Measured apiserver WASM size delta** (`bash scripts/build-wasm-chunks.sh`,
raw `.wasm` bytes against the 64MiB Loader cap, not gzip -- this repo
moved off the gzip/9.5MiB-CI-budget convention the earlier phases above
used once the S19 single-Worker + ASSETS+LOADER path landed): baseline
measured directly on this worktree's `feat/v2-rearchitecture` tip before
this change (confirmed by reverting via `git stash`, rebuilding, and
restoring -- not assumed from an older doc's number) was **62,862,779
bytes**; with `PriorityClass` registered plus `priority.go`'s admission
logic it is **62,999,035 bytes**, a delta of **+136,256 bytes (+133.06
KiB, +0.217%)**. Headroom under the 64MiB (67,108,864 byte) cap: 4,013KiB
(was 4,146KiB before this change). KCM (`cmd/kcm-wasm`) is unaffected --
**65,041,921 bytes, unchanged** -- since it doesn't import `pkg/apiserver`
(no workload controller in this project's `--controllers` set consumes
`PriorityClass`, matching upstream: none of ReplicaSet/Deployment/
DaemonSet/StatefulSet/Job/CronJob/Endpoints/EndpointSlice read it).

No `pkg/leanclient` change was needed: the real `kube-scheduler`
(`cmd/scheduler`, BYO VM/host process) only ever reads a Pod's already-
resolved `spec.priority`, never `PriorityClass` objects directly (the
`schedulingv1listers.PriorityClassLister` upstream's own priority
admission plugin needs is apiserver-side machinery, and this apiserver's
`ResolvePodPriority` reads its own in-process `ResourceStore` directly,
no client library involved), and no enabled `pkg/controller/*` reconciler
constructs a PriorityClass informer either -- confirmed by grep against
`pkg/leanclient/leanclient.go`'s hand-curated 13-type list, unchanged by
this task.

**Idle-cost invariant**: unaffected by construction -- `ResolvePodPriority`
only runs synchronously inside an existing Pod-create request, the same
"runs only when there's a write to do" shape every other admission step
in this path (LimitRange, compute-class routing) already has. No alarm,
no polling, nothing added for a cluster that never creates a Pod or a
PriorityClass.

## Phase 9 (real garbagecollector controller) implementation (actual, 2026-07-09)

Replaces `pkg/apiserver/gc.go`'s `CascadeDeleteDependents` (a hand-rolled,
synchronous-in-request, namespace-scoped-only ownerReferences sweep) with
the real, unmodified upstream `k8s.io/kubernetes/pkg/controller/
garbagecollector`, hosted as a THIRD Loader-loaded dynamic worker
(`gc:<doName>@sha`) inside the Controllers DO, alongside kcm and (still
unshipped) sched -- see `pkg/controllers/gc`'s doc comment for the full
design and `docs/general-purpose-k8s-plan.md`'s "ownerReferences GC"
entry for the investigation this was deferred from.

**Why a third isolate, not folded into kcm's binary.** The garbage
collector's dependency-graph builder needs an informer/watch per
resource type across the _entire_ `apidef.Table`, not the handful of
types the six workload controllers already watch. Measured, not
assumed: this code briefly lived directly in `pkg/controllers` (sharing
kcm's package) and added ~4.15MB to the committed KCM binary (64,994,641
-> 69,145,178 bytes) despite `RunControllerManager` never calling
`RunGarbageCollector` -- Go's GOOS=js/wasm dead-code elimination does not
prune an entire unrelated controller's reachable code out of a shared
package. Splitting `pkg/controllers/gc` (and the REST-config builder,
`pkg/controllers/restconfig`) into their own packages restored kcm to
64,990,970 bytes and left gc at 65,205,198 bytes -- both comfortably
under the Loader's 67,108,864-byte (64MiB) cap (1859KiB / 2068KiB
headroom respectively).

**New lean-overlay surface.** `k8s.io/controller-manager/pkg/
informerfactory.InformerFactory`'s `ForResource` return type is fixed to
`k8s.io/client-go/informers.GenericInformer` -- importing that type
pulled in the _existing_ `pkg/clientgo-lean-overlays/informers/
factory.go` overlay (built earlier for kube-scheduler's wide,
non-leanwidth `kubernetes.Interface`), which doesn't compile under
`-tags leanwidth`'s narrowed `kubernetes.Interface`. Fixed with a new
`factory_leanwidth.go` sibling (`//go:build leanwidth`) declaring just
the bare `GenericInformer`/`SharedInformerFactory` types this build
needs, none of the real `SharedInformerFactory`'s Apps()/Core()/etc.
accessors (which is what dragged in the wide-Interface-only informer
subpackages). Also new: `pkg/leanclient.MetadataClient` (a
`metadata.Interface` implementation reusing this repo's existing
DoRaw+json verb/watch generics with `T = metav1.PartialObjectMetadata`)
and `apidef.NewRESTMapper` (a static `meta.ResettableRESTMapper` built
once from `apidef.Table` at call time -- no discovery round trip, since
this project's resource set is fixed at build time; `Reset()` is a
permanent no-op for the same reason).

**Cost delta**: +1 Loader unique-load id per active cluster/day
($0.002/cluster/active-day, same primitive as kcm's own line in the
Single-Worker consolidation table above) -- idle clusters load nothing,
same as kcm/apiserver. No new alarm: gc is poked the same
`fetch()`-driven + safety-net-alarm-driven way kcm/sched already are
(`packages/k8flare-worker/src/controllers/index.ts`'s `COMPONENTS` array), no
independent polling loop of its own.

**User-visible behavior change, not just an implementation swap.** The
old mechanism ran cascade delete synchronously inside the same HTTP
request that deleted the owner (`kubectl delete deployment` didn't
return until its ReplicaSets/Pods were gone too). The real
garbagecollector runs asynchronously in its own dynamic-worker isolate:
`kubectl delete deployment` now returns as soon as the Deployment itself
is gone, with dependents cascading afterward on that isolate's own pump
window -- eventually consistent, matching real upstream Kubernetes
exactly, not the artificially-synchronous guarantee the old bespoke
mechanism gave for free by construction. This was an explicit,
user-approved tradeoff (not discovered after the fact), made because
matching real Kubernetes semantics was judged more valuable than the
synchronous convenience -- see git history for the decision point.

## Phase 10 (real kube-scheduler as a dynamic worker) implementation (actual, 2026-07-10)

The real, unmodified upstream kube-scheduler ships as the FOURTH dynamic
worker (`sched:<doName>@sha`, `-tags schedwidth`): 45.2MB opt against
the 64MiB Loader cap, down from 101.1MB via the three severings recorded
in `docs/platform-verification.md` S21. The same cri-client severing
also shrank kcm 65.0->41.8MB and gc 65.2->41.2MB as a side effect.

**Cost delta**: +1 Loader unique-load id per active cluster/day
($0.002/cluster/active-day, same primitive as kcm/gc/apiserver's
existing lines). Same poke/event-armed execution shape as kcm/gc -- no
new alarm, no polling; idle clusters load nothing. Replaces nothing yet
(the TS binder in packages/k8flare-worker/src/nodes/scheduler.ts still runs for
Pod-on-Containers nodes; retiring it in favor of this scheduler is task
#2's design work), so for now the sched DW adds capability (real
scheduling semantics for BYO-VM-node clusters without a host scheduler)
rather than replacing spend.

## Task #2 (Pod-on-Containers Provisioner, real-scheduler handoff) implementation (actual, 2026-07-11)

Replaces the task #2 target design's original assumption (a new
resident Go dynamic worker, poke-driven like kcm/gc/sched) with a
**per-request, admission-time** implementation instead --
`pkg/apiserver/computeclass.go`'s `AssignContainersNode`, called from
the existing pod-create path (`handler.go`) that already runs
`MutatePodForComputeClass`. The chosen shape costs strictly less than
either a new resident DW or the TS binder it replaces:

- **No new resident dynamic worker.** Size-tier resolution (real
  `k8s.io/apimachinery/pkg/api/resource.Quantity` math, replacing
  `nodes/images.ts`'s hand-rolled parsing) and dedicated-Node-name
  minting (`k8s.io/apiserver/pkg/storage/names.SimpleNameGenerator`,
  the same generator `store.go` already links for `generateName`) are
  both pure, side-effect-free functions of the Pod object already being
  admitted -- there is no reactive/polling shape to justify a poke-armed
  isolate, alarm, or warmup window (cost invariants #1-3: this is the
  cheap side of the choice the design doc's open question asked for).
  Runs inside the SAME per-request apiserver WASM instantiation every
  Pod create already pays for; the only marginal cost is a few more Go
  function calls per Pod-on-Containers Pod create, not a new billing
  primitive.
- **`nodes/scheduler.ts` (the DO, kept under its old class name --
  renaming needs a wrangler.jsonc DO migration entry, not justified by
  this change) shrinks, it doesn't grow**: the Binding-subresource call
  and Scheduled/FailedScheduling event synthesis it used to do are both
  deleted outright (the real scheduler, `pkg/controllers/sched`, now
  does both natively -- see the event-broadcaster-sink fix, commit
  `088c7fe`, which was a *precondition* for this handoff: without it,
  the standard events `kubectl describe pod` shows would have silently
  vanished). What remains is the same event-armed poke/safety-net-alarm
  shape the DO already had (unchanged wake model), just with less work
  per poke.
- **Net effect on the sched DW's existing cost line above**: it no
  longer "adds capability... rather than replacing spend" -- Pod-on-
  Containers Pods are now bound by the real scheduler too, so this
  closes that open item. No new Loader unique-load id beyond what
  Phase 10 already counted (sched was already loaded on every relevant
  Pod write via `pingControllers`; it simply does more useful work per
  load now).

**Rejected alternative (per this task's own design brief, explicitly
left open pending a cost-invariant check)**: a resident Go "Provisioner"
dynamic worker on the kcm/gc/sched poke pattern. Rejected because it
would add a 5th ~64MiB-capped binary, its own warmup window and
safety-net alarm, and a Loader unique-load id per active cluster/day --
recurring cost with no reactive workload to justify it, since every
decision it would have made is a pure function of the Pod already in
hand at admission time. This is the kind of call cost invariant #8 asks
for before adding a new poke/alarm-shaped component: it doesn't survive
the check here.

**Unverified**: the Docker-dependent half of this design (a NodeVM's
real embedded kubelet self-registering the exact dedicated Node name
`AssignContainersNode` pinned, and that registration's own
`node.kubernetes.io/not-ready` self-taint correctly gating the real
scheduler until Ready -- see the finding recorded against task #2's
design phase, 2026-07-10, that Ready-condition-alone does NOT gate the
real scheduler) has not been exercised end-to-end in this sandbox (no
Docker). `smoke-nodes.yml` was rewritten for the new flow but its own
CI run is the first real check of that half.

**Verified in CI (2026-07-11, run 29139610706)**: the full
Pod-on-Containers flow -- admission provisioning, per-Pod NodeVM boot,
kubelet self-registration of the pinned Node name, real-scheduler Bind,
workload Running, restartPolicy semantics, Pod delete -> container stop
+ Node reap -- passes smoke-nodes.yml end to end (Docker-backed local
emulation; the workflow header records the seven-iteration bring-up and
the CI-only privileged/tmpfs shims real Firecracker microVMs won't
need).

## Task #1 (watch selector matching -> real apimachinery WASM) implementation (actual, 2026-07-12)

The watch fan-out's label/field selector parsing+matching moved from a
hand-rolled TS subset (label-selector.ts, deleted) to the real
apimachinery parsers, compiled to a 4.58MB (-Oz; ~1.3MB gzip) wasm
module BUNDLED into the gateway script (`make wasm-selector`;
pkg/selectormatch). Cost shape: zero new requests/DOs/alarms/Loader
ids -- one ~26ms Go-runtime instantiation per gateway isolate at first
selector use, ~0ms per synchronous match call afterwards (execution
model verified in docs/platform-verification.md S22). Bundle impact:
+1.3MB gzip against the Worker script's 10MiB deploy budget.
(2026-09-10: that gzip budget no longer exists, and the module moved
behind a dynamic import so it is no longer compiled at isolate startup
-- only a watch that actually carries a selector pays for it. Upload
size is unchanged; see platform-verification.md S35.)
Functional gains measured live: set-based operators (`in`, `!key`)
now work, and invalid selectors 400 at watch open like upstream
(previously silently mis-applied).

## Idle-behavior measurement (2026-07-26/27, production, wrangler tail)

Method: `wrangler tail k8flare --format json` against the deployed Worker
with an EMPTY cluster (no namespaces beyond bootstrap, no workloads, no
nodes), two ~25-minute windows.

- **Before** commit c1eebe6: 286 events / 24.6 min, including **94
  Controllers-DO alarm firings**, each reloading the kcm/sched/gc dynamic
  workers — the safety-net alarm was self-perpetuating through the warmup
  window (violated cost invariants #1/#3; root cause and fix in that
  commit).
- **After**: 392 events all within the first 2.7 minutes (residual warmup
  drain + 7 alarm firings running the cheap convergence probe), then
  **zero invocations and zero alarms for the remaining 21+ minutes** —
  the control plane is fully parked while idle. Dollar figures still TBD
  (billing-grade numbers need a longer window via the dashboard/GraphQL),
  but the *activity* half of scale-to-zero is now measured, not assumed.

## cluster-operator (estimate, 2026-07-27 — pre-implementation per invariant #5)

The fourth resident dynamic worker (`pkg/controllers/clusterop`), hosted by
the Controllers DO of the **management ("default") cluster only** — tenant
clusters hold no Cluster objects, so `loadComponent` returns null for them
and they pay nothing for it at all.

| Axis                        | Cost                                                                                                                                                                                                                                                                                                    |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Idle                        | **0.** Same event-armed poke pump as kcm/gc/sched: it runs only inside a pump window opened by a write under `/registry/clusters/` (storage's `pingControllers`) or by the safety-net alarm, which parks when no Cluster is unconverged. Its watch is the apiserver's, not a wall-clock connection.       |
| Loader                      | +$0.002/unique/day, and only on days the management cluster actually loads it (a day with no Cluster write loads nothing). One id, not per-tenant — `clusterop:default@<sha>#<tokenTag>`.                                                                                                                |
| Per reconcile               | DO reads/writes only: 1 Cluster read + at most 1 status write + 1 Secret read/write (`k8flare-system/cluster-<name>`) + 1 registry DO write + 1 token-vault facet write. Single-digit DO ops per Cluster create; a converged Cluster costs **zero** (the observedGeneration/phase guard skips it).        |
| Per teardown                | The existing cascade's cost, unchanged (4 DO `/admin/destroy` calls + 1 registry delete) — it moves from the admin API to the operator, it is not new work.                                                                                                                                              |
| Alarm                       | No new alarm. It reuses the Controllers DO's existing safety net, whose convergence probe gains **one list call** (`/apis/k8flare.com/v1alpha1/clusters`) per tick, on the management cluster only, and only while a tick is already happening.                                                          |

Risk to watch (the reason the guard exists): the operator writes Cluster
status and Secrets, and Cluster writes poke the pump. Status is written
only on a real diff and skipped entirely once `observedGeneration ==
generation && phase == Ready`, and Secrets are deliberately **not** in
`CONTROLLER_RELEVANT_PREFIXES` — without both, a single provisioned
cluster would reconcile forever and the alarm would never park.

Actual: not yet measured (no `wrangler deploy` performed for this change).

## no-op 更新の書き込みストーム (actual: 行数レート / modeled: 月額換算, 2026-07-30)

`docs/platform-verification.md` の S26 訂正の裏付け計測。「Deployment を
作ってからノードを join する」という平凡な順序で、rows-written 課金と
kine の行数が**無制限に伸びていた**。

計測条件: `wrangler dev --local`、ノード 0 台のクラスタに 2 replica の
Deployment を 1 個だけ作成し、以後は誰も触らない。

| | 修正前 | 修正後 |
|---|---|---|
| kine 行(120 秒あたり) | 約 2,260 | **0** |
| kine 総行数 | 7 分で 8,400、増加継続 | 159 で定常 |
| Deployment resourceVersion | 約 20/秒で増加 | 固定 |
| Controllers アラーム間隔 | 15 秒固定 | 15→30→60→120→240 秒 |

書かれていた値は前リビジョンと 1 バイトも違わなかった(GET を 2 回叩いて
resourceVersion 以外の全フィールドが一致することを確認済み)。原因は
`KineStorage.GuaranteedUpdate` に upstream etcd3 store の no-op 抑止
(`bytes.Equal(data, origState.data)`)が無かったこと。

換算すると、修正前は Deployment 1 個あたり月あたり約 4,900 万行の
rows-written(1,130 行/60 秒 × 2,592,000 秒 = 48,816,000)。**行数レートは
実測だが、この月額換算は実測ではなくモデル値**である — 課金された実クラスタは
まだ存在せず、`wrangler dev --local` の sqlite から本番 DO の課金軸へ
外挿している(docs/adopter-quickstart.md の「What it costs」と同じ但し書き)。上の表の「rows read-written」が DO の課金軸である以上、
これはストレージ代だけのアイドルとは程遠い。同じ形の回帰は 2 段で捕まえる: `make test` レーンの
`pkg/apiserver/upstreamstorage_test.go` の 3 本(KineStorage を直接叩く
単体テスト、全 PR で走る)と、`cost-gate.yml` の
"Verify an unconvergeable workload does not write forever" ステップ
(ノードなしクラスタに Deployment を作り、全シャード合計の kine 行数の
定常増加を 60 秒あたり 50 行未満に縛る)。

## CronJob の wake-up アラーム (actual + estimate, 2026-07-30)

**手順違反の記録(不変条件 #5)**: この見積もりは実装の**後**に書いた。#5 は
「実装前にコストを見積もって書く」と定めており、順序を守れなかった。内容は
実測に基づくが、順序の逸脱としてここに残す。

S27(`docs/platform-verification.md`)の修正で、CronJob が存在するクラスタは
アイドルでも「次の発火時刻」にアラームを 1 回張るようになった。

アイドル時の課金対象:

| CronJob なし | 変化なし。アラームはパークする(実測: 全 alarm 0) |
|---|---|
| CronJob あり | **発火 1 回につき** DO アラーム 1 回 + KCM 等の dynamic worker ロード 1 回 + apiserver への list 数回 |

固定間隔ポーリングではない(不変条件 #3): アラームは CronJob が存在する
から張られ、特定の時刻に 1 回だけセットされ、最後の CronJob を削除すると
またパークする。

頻度はユーザーのスケジュール次第で、上限はユーザーが決める:

| schedule | 月あたりの発火 |
|---|---|
| `0 3 * * *`(日次) | 30 |
| `0 * * * *`(毎時) | 720 |
| `*/5 * * * *` | 8,640 |
| `* * * * *` | 43,200 |

発火 1 回のコストは「コールドな制御プレーンを起こして 1 サイクル回す」分
— dynamic worker のロードは Workers の CPU 時間課金で、I/O 待ちは無課金。
Pod 本体はユーザーのコスト(不変条件 #6)だが、**この起床は制御プレーンの
コスト**なので上表に含めた。`* * * * *` を置けばアイドルとは呼べない
水準になる、というのが正直なところ。

なお `overdue`(発火時刻を跨いでクラスタが落ちていた場合)は通常の
「未収束の仕事」扱いになるため、既存の指数バックオフ(15 秒 → 10 分)の
上限が効く。取りこぼしを検知してから Job が作られるまでの間だけ、
そのレートでアラームが鳴る。

### 実測とベースライン比較 (2026-07-30)

「CronJob を置くとクラスタがパークしなくなるのでは」という懸念に対する
測定。各 12 分、5 秒間隔サンプリング、ノード 0 台のローカルクラスタ:

| 条件 | アラーム再武装 | パーク |
|---|---|---|
| CronJob なし | 0 | パークする |
| CronJob 1 個・次の発火まで待機 | 発火時刻に 1 回だけ | Cluster DO はパーク |
| **ベースライン: 素の Job 1 個(CronJob なし)** | **1.8/分** | **一度もパークせず** |
| CronJob `*/2 * * * *` | 3.8/分 | 一度もパークせず |

**ベースライン行が結論を決める**: CronJob が無くても、完走できない Job が
1 個あるだけでクラスタはパークしなくなる。これは S26 で扱った
「未収束ワークロード」の既存挙動であって、**CronJob ウェイクアップが
作った問題ではない**。`*/2` がその約 2 倍になるのは、2 分ごとに新しい
完走できない Job を作り続けるからで、スケジュールの内容がそのまま出た
数字である。

**未測定(重要)**: 上表はすべてノード 0 台での測定なので、Job が完走する
=実際に問題になるケースの定常コストは測れていない。設計上は
「発火 1 回 = アラーム 1 + dynamic worker ロード 1」で、その CPU 時間の
実測値は持っていない。ノードを持つ環境で測り直すこと。

**既知の改善余地**: 現在アラームは発火時に kcm/sched/gc/clusterop を
すべてロードしている(`controllers/index.ts` の `for (const name of
COMPONENTS)`)。cron の発火に必要なのは kcm だけなので、絞れば 1 発火
あたりのロードは約 1/4 になる。未着手 — このアラーム経路は過去 2 回
壊しているため(94-alarms インシデントと d1a9503)、実測を伴わない変更を
避けた。

## 進行中の削除をアラームの仕事に数える (actual + estimate, 2026-09-10 — 不変条件 #5)

docs/platform-verification.md S36 の修正で、Controllers DO の park 判定
(`hasUnconvergedWork()`)に 2 つの読み取りを足した。どちらもアラームが
既に鳴っているときだけ走る = アイドルでは 0 件。

| 追加 | 単価 | アイドル時 |
|---|---|---|
| `GET /api/v1/replicationcontrollers` | 1 tick あたり list 1 回 | 0(パーク中は tick が無い) |
| `GET /internal/pending-deletions` | 1 tick あたりリクエスト 1 回。サーバー側で namespaced リソースを走査するので、1 リクエストで済むのが要点(DO から 30 本の HTTP を張るのではない) | 同上 |

**アラームの寿命への影響**: foreground/orphan 削除が飛んでいる間、park 判定が
「仕事あり」を返すのでアラームが 15 秒 → 10 分の既存バックオフで鳴り続ける。
削除が完了すれば `pending` が 0 に戻り、次の tick でパークする。**新しい
固定間隔ポーリングは入れていない**(不変条件 #3)。修正前はカスケードの
途中でパークしていたので、これは「アイドル時のコスト増」ではなく「これまで
落としていた仕事のぶんだけ起きている時間が伸びる」変化である。

**outbound 再送(`windowHandoffAttempts`)**: 放棄された呼び出しを最大 2 回に
限って再発行する。待ちはチャンネル受信でタイマー無し(不変条件 #2 — I/O 待ちは
無課金)。最悪ケースは同じリクエストが 2 回サーバーに届くことで、subrequest
1 件ぶんの増分。**応答ヘッダの 10 秒上限**は one-shot タイマー 1 本を呼び出し
ごとに張り、成功時に `Stop()` する(不変条件 #3 — ポーリングではない)。

**未実測**: 上の 2 リストが cost-gate の rows read にどれだけ乗るか。
## 匿名 `/readyz`(actual, 2026-09-11 — 不変条件 #5)

`/readyz` はトークン不要(外形監視が叩けなければ readiness の意味が無い)
なので、コストを押さえるのは認証ではなくキャッシュである。実チェックは
cluster ごと・isolate ごとに ready なら 30 秒、not-ready なら 5 秒に 1 回まで。
冷えた判定に同時に到着した呼び出しは同じ実行を待ち合わせる。

| 項目 | 単価 | 実測(`wrangler dev`) |
|---|---|---|
| 実チェックを走らせた呼び出し | Cluster DO `GET /revision` 1 回(sqlite 読み 1 行、書き込み 0)+ apiserver DW への Loader ディスパッチ 1 回(`GET /readyz`、storage には触らない)+ ASSETS の manifest 読み 4 本 | 約 5ms(warm) |
| キャッシュから返した呼び出し | Worker リクエスト 1 件のみ | 約 1.7ms(`/healthz` と同じ) |

匿名 150 リクエスト(逐次 50 + 同時 50 + TTL 満了後に同時 50)を約 35 秒で
投げて、実チェックの実行は 2 回。つまりリクエスト量では増幅しない。

**アイドルとのトレードオフ**: 上限は isolate 単位なので、全体量はリクエスト
数ではなく生存 isolate 数に比例する。また 30 秒間隔でポーリングされ続ける
クラスタは Cluster DO と apiserver isolate が起き続けるため不変条件 #1 の
「アイドル」ではなくなる。これは k8flare 側の常駐ではなく運用者が選ぶ
ポーリング間隔の問題なので、docs/admin-guide.md と
docs/adopter-quickstart.md にトレードオフとして書いた。

**未実測**: 本番(`workers.dev`)での isolate 数あたりの実効レートと、
cost-gate の rows read への寄与。
