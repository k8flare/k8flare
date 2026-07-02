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

| Primitive | Billing axis | Known numbers | Source |
|---|---|---|---|
| Workers | CPU time actually consumed only. Waiting on I/O is free. No wall-clock limit on an HTTP streaming response (CPU time limit of 5 min/invocation applies) | Unit rate itself not recorded in this document (TBD) | v2 rewrite plan's research findings (primary source: Cloudflare Workers pricing docs, URL not recorded — to be added when referenced) |
| Durable Objects | requests / duration (GB-s) / rows read-written (SQLite) / alarm invocation count | 10GB storage/DO, ~1,000 req/s soft ceiling (200–500 for complex ops), single-threaded, 2MB max value/row, 32,768 WebSockets/DO, 30-day PITR. $ rates not recorded in this document (TBD) | [DO limits](https://developers.cloudflare.com/durable-objects/platform/limits/), [DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/) (cited via docs/multi-tenancy-and-hosting.md) |
| Cloudflare Containers | Wall-clock (uptime) based vCPU-second and GiB-second billing | vCPU cost $0.00002/vCPU-sec, memory cost $0.0000025/GiB-sec. Paid-plan included allowance: 375 vCPU-min/month, 25 GiB-hours/month, free. Instance ceiling 4vCPU/12GiB/20GB disk, paired 1:1 with a DO, scale-to-zero | v2 rewrite plan's estimate (Phase 5 route B), [Containers limits](https://developers.cloudflare.com/containers/platform-details/limits/) (cited via docs/multi-tenancy-and-hosting.md) |
| Worker Loader (Dynamic Workers) | Billed per unique load | $0.002/unique/day | CLAUDE.md cost invariant #5, v2 rewrite plan |
| R2 | Storage (GB-month, tiered Standard/Infrequent Access) + Class A ops (writes/lists/multipart) + Class B ops (reads) + data retrieval (Infrequent Access only). Egress and deletes (`DeleteObject`/`DeleteObjects`/`DeleteBucket`) are always free and uncapped | Standard: $0.015/GB-month storage, $4.50/M Class A requests, $0.36/M Class B requests, free tier 10GB-month + 1M Class A + 10M Class B/month. Infrequent Access: $0.01/GB-month storage (30-day minimum), $9.00/M Class A, $0.90/M Class B, $0.01/GB retrieval, no free tier. Minting a Temporary Access Credential is a control-plane call, not a Class A/B op — free via local JWT signing, or drawn from the shared 1,200 req/5min account budget if minted via the REST API | [R2 pricing](https://developers.cloudflare.com/r2/pricing/) (page dateModified 2026-05-28), [R2 limits](https://developers.cloudflare.com/r2/platform/limits/) (page dateModified 2026-06-08) — cited via `spikes/s6-r2/RESEARCH.md` |
| Workers KV | Read/write counts + storage (assumed, details unconfirmed) | TBD (not yet used; planned for use as `CLUSTER_ROUTES` in Phase 2, to be added before implementation) | — |

## Per-component estimates

| Component | Idle monthly cost (target: ~0) | Active unit cost | Estimate | Actual |
|---|---|---|---|---|
| gateway (TS Worker) | ~0 (stateless, no DO) | Workers request billing (actual CPU time) | TBD | Not yet done (not implemented, Phase 2) |
| apiserver (Go WASM) | ~0 | Workers request billing (actual CPU time). Whether the WASM startup tax applies depends on S5's outcome (isolate singleton-ization, see `docs/platform-verification.md` S5) | TBD (pending S5) | Not yet done (not implemented, Phase 2) |
| storage: Cluster DO | Target: storage cost only (alarm parked) | DO requests / duration GB-s / rows read-written + alarm invocation count (event-armed only) | TBD | Not yet done (not implemented, Phase 4) |
| storage: WatchHub DO | ~0 (while hibernating) | DO requests / duration GB-s (hibernating WS connections are assumed not billed for inactive time — needs measurement) | TBD | Not yet done (not implemented, Phase 4) |
| runtime (TS Worker + LOADER) | ~0 (when cron hasn't fired) | Workers request billing + Worker Loader $0.002/unique/day | TBD | Not yet done (not implemented, Phase 2) |
| controllers | See "controllers execution path: two-route estimate" below | Same | Same | Not yet done (pending `docs/platform-verification.md` S8) |
| nodes (Pod-on-Containers) | Target ~0, though a lightweight alarm is expected to be needed for virtual-node Lease renewal (regardless of whether Pods exist) | Containers vCPU/GiB-second billing. **Each running Pod container implies a DO alarm firing at least every ≤3 minutes** while it's up — confirmed by reading the `@cloudflare/containers` self-monitoring source (`spikes/s3-containers/FINDINGS.md`); it re-arms on that cadence while running and calls `deleteAlarm()` once stopped, so it parks when idle (event-armed rule satisfied, but not zero-alarm while any Pod is running). **The Pod's own running cost is the user's workload cost** (cost invariant #6, not counted as control-plane cost) | TBD | Not yet done (not implemented, Phase 7) |
| R2 PV/PVC | Target: storage cost only | R2 operation billing | TBD | Not yet done (not implemented, Phase 8) |

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

| Usage pattern | Assumption | Approx. monthly cost |
|---|---|---|
| Low frequency (solo development, 20 starts/day × 30s) | 10 min/day = 300 min/month | **$0**, within the free allowance |
| Medium frequency (small team, 50 starts/day × 60s) | 50 min/day = 1,500 min/month | **~$1.35/month**, billed for the overage past the free allowance |
| Constantly busy (minute-scale CronJobs, frequent HPA adjustments — never idles) | Effectively 24-hour uptime | **~$58/month** in vCPU+memory — more than an equivalent-spec VPS (from ~$5/month) |

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
- [ ] No DO alarm is scheduled (parked state — only re-armed on waiting
      events, with no fixed-interval polling left running)
- [ ] WebSocket connections are hibernating (WatchHub DO; the
      `ctx.waitUntil` keep-alive used while holding a connection isn't
      active)
- [ ] No billable DO operations (alarm firing, facet access, etc.) occur
      over a sustained period

As of this document's creation (Phase 1), every item here is
unimplemented and unverified. Alarm parking and hibernation will be
measured once Phase 4 (storage v2) is done, and controllers' idle
behavior once Phase 5/6 is done, to confirm this checklist is satisfied.
