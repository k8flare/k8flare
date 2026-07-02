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

| Component                    | Idle monthly cost (target: ~0)                                                                                                   | Active unit cost                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | Estimate         | Actual                                                    |
| ---------------------------- | -------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------- | --------------------------------------------------------- |
| gateway (TS Worker)          | ~0 (stateless, no DO)                                                                                                            | Workers request billing (actual CPU time)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | TBD              | Not yet done (not implemented, Phase 2)                   |
| apiserver (Go WASM)          | ~0                                                                                                                               | Workers request billing (actual CPU time). Whether the WASM startup tax applies depends on S5's outcome (isolate singleton-ization, see `docs/platform-verification.md` S5)                                                                                                                                                                                                                                                                                                                                                                                   | TBD (pending S5) | Not yet done (not implemented, Phase 2)                   |
| storage: Cluster DO          | Target: storage cost only (alarm parked)                                                                                         | DO requests / duration GB-s / rows read-written + alarm invocation count (event-armed only). A namespaced key op now costs 1 parent request + 1 facet request (GET/list) or 1 parent write + 1 facet `/apply` write (PUT/DELETE) -- roughly double the DO-request count of the pre-facet single-table design, in exchange for per-namespace storage headroom. Loader cost is a flat $0.002/day regardless of namespace count: one generic facet class, one content-hash loader key, reused via `ctx.facets.get(name, ...)` for every ns/events/ca-vault facet | TBD              | See Actual: idle alarm parking measured, Phase 4          |
| storage: WatchHub DO         | ~0 (while hibernating)                                                                                                           | DO requests: 1 `/push` per write that touches a watched key (from Cluster, awaited synchronously) + 1 `/replay` per new client connection (to Cluster) + N WebSocket sends per push (N = matching connected clients). No upstream connection to hold open (see design note below), so nothing keeps WatchHub resident between events -- nothing added to WatchHub's own idle cost beyond the DO-request cost of whichever push/replay call last touched it                                                                                                    | TBD              | See Actual below, Phase 4                                 |
| runtime (TS Worker + LOADER) | ~0 (when cron hasn't fired)                                                                                                      | Workers request billing + Worker Loader $0.002/unique/day                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | TBD              | Not yet done (not implemented, Phase 2)                   |
| controllers                  | See "controllers execution path: two-route estimate" below                                                                       | Same                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | Same             | Not yet done (pending `docs/platform-verification.md` S8) |
| nodes (Pod-on-Containers)    | Target ~0, though a lightweight alarm is expected to be needed for virtual-node Lease renewal (regardless of whether Pods exist) | Containers vCPU/GiB-second billing. **Each running Pod container implies a DO alarm firing at least every ≤3 minutes** while it's up — confirmed by reading the `@cloudflare/containers` self-monitoring source (`spikes/s3-containers/FINDINGS.md`); it re-arms on that cadence while running and calls `deleteAlarm()` once stopped, so it parks when idle (event-armed rule satisfied, but not zero-alarm while any Pod is running). **The Pod's own running cost is the user's workload cost** (cost invariant #6, not counted as control-plane cost)     | TBD              | Not yet done (not implemented, Phase 7)                   |
| R2 PV/PVC                    | Target: storage cost only                                                                                                        | R2 operation billing                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | TBD              | Not yet done (not implemented, Phase 8)                   |

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

## Idle-cluster verification checklist

A checklist for mechanically confirming cost invariant #1, "nothing is
allowed to run while idle." Turning this into a CI cost gate is planned
for Phase 6 (see the v2 rewrite plan's "add a cost gate" section).

- [ ] Container instance count is 0 (if route B is adopted; for route A,
      confirm via `wrangler tail` etc. that the controllers Worker's
      actual consumed CPU time is effectively zero)
- [x] No DO alarm is scheduled (parked state — only re-armed on waiting
      events, with no fixed-interval polling left running) -- **verified
      2026-07-02 against real `wrangler dev`** for the Cluster DO's
      safety-net alarm: idle (no Nodes/Services) parks to `alarm: null`
      and stays there; a single write resumes it; deleting the last
      live Node/Service lets the next scheduled fire park it again
      rather than re-arming forever. See "Phase 4 (storage v2) actuals"
      above for the full sequence.
- [ ] WebSocket connections are hibernating (WatchHub DO) -- client-facing
      sockets use `ctx.acceptWebSocket` as designed, but end-to-end watch
      consumption by a real Kubernetes client (client-go/kubectl) could
      not be verified this phase (see the known gap above); the
      `ctx.waitUntil` keep-alive this line originally referred to no
      longer exists in gateway's watch relay in the same shape after
      Phase 4 -- worth re-auditing once the client-go gap is resolved.
- [ ] No billable DO operations (alarm firing, facet access, etc.) occur
      over a sustained period -- alarm firing is now verified (above);
      facet access under sustained idle (i.e. confirming a namespace
      with no activity causes zero facet fetch() calls) was not
      separately measured this phase.

As of Phase 4 (storage v2, 2026-07-02): the alarm-parking item is
verified against real `wrangler dev`, the strongest evidence available
without a production deployment. The other items remain open --
WebSocket hibernation specifically is blocked on the client-go
transport gap documented above and in the WatchHub redesign commit.
Container/controllers items remain for Phase 5/6.
