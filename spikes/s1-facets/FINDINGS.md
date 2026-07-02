# S1 — DO Facets spike findings (remaining questions)

Continuation of the empirical facets verification from commit 46df0c0.
Verified 2026-07-02 both locally and against a real deployment
(`k8flare-verify-facets` on the KOOFFICE account — deployed for this spike and
deleted afterwards; deletion double-checked via 404 and
`wrangler deployments list` → code 10007). No secrets were used; no alarms were
ever armed (facets reject setAlarm, and the supervisor DO never called it).
Spike code: `wrangler.jsonc` + `src/index.ts`. The agent that ran this spike
could not write report files, so this document was transcribed by the
coordinating session from its full report.

## 0. Local tooling status (new, important)

- The repo-pinned wrangler 4.77.0 (workerd@1.20260317.1) **does not implement
  facets at all**: `ctx.facets` is `undefined` and the loader's WorkerStub has
  no `getDurableObjectClass` (prototype chain inspected via a debug endpoint).
  workerd 1.20260317.1 predates the facets launch (Agents Week, Apr 2026).
- `npx wrangler@latest` (4.106.0) supports both locally; items 1, 2 and 4 were
  verified locally on it and cross-checked in production. **Use
  `npx wrangler@latest` for any facets work until the repo pin is bumped**
  (bumping the pin was out of scope for this spike).
- Even on wrangler@latest, **alarms on SQLite-backed DOs cannot be tested
  locally at all** (facet or not): `ctx.storage.setAlarm()` throws
  `Error: alarms are not yet implemented for SQLite-backed Durable Objects`.
  Item 3 was verified in production only.

## 1. Facet count — no ceiling found up to 1,000 (local and production)

Staged growth 100 → 250 → 500 → 1,000 with `/ping` forcing real instantiation.
No errors at 1,000 in either environment; production was notably faster
(last 500 creations: 911 ms prod vs 3,856 ms local). 1,000 was the requested
target; higher counts are unexplored.

Two transient errors were observed across 20+ burst operations:
`Internal error in Durable Object storage caused object to be reset` — once on
the second batch of the first production test (target=50, batch=10) and once on
the first parallel test (n=20). Immediate retry with a fresh prefix succeeded,
and five follow-up bursts all passed. No correlation with facet count or batch
size was found. Interpretation: not a hard limit, but a rare retryable error
during mass facet creation. **Production implication: any code path that
creates multiple facets (e.g. namespace creation) needs a retry wrapper.**

## 2. Facet execution parallelism — dispatch is NOT serialized

Each facet's `/ping?sleepMs=N` waits via `setTimeout` before replying;
`Promise.all` over N distinct facets measures wall time. First runs appeared to
scale with N (n=100 → 823 ms) but warm re-runs collapsed to a flat ~230 ms for
n=100 and n=300 alike (n=5 baseline: 240 ms) — the growth was facet-creation
cost (consistent with item 1), not dispatch serialization. Confirmed locally on
real workerd and in production (n=20 ×5 runs: 92–188 ms; serial execution
would be ≥4,000 ms).

Honest limitation: the workload is I/O-bound (`setTimeout`), so this proves
non-serialized dispatch, not multi-core parallelism for CPU-bound work. The
design expects await-based I/O work in facets, so this is sufficient.

## 3. Alarms inside facets — explicitly forbidden (production-confirmed)

`ctx.storage.setAlarm()` inside a facet throws immediately in production:
`Error: Facets currently cannot set alarms.` This is a deliberate rejection,
not a generic failure. (Untestable locally per item 0.)

Design impact: event-armed alarms (controller re-arm, safety-net alarms) must
live on top-level DOs, never in facets. This is consistent with the current
target design (controller alarms on the cluster DO), but the option of
"namespace-level alarm-driven work inside its facet" is now definitively
closed and should be stated explicitly rather than assumed.

## 4. delete() semantics — identical locally and in production

- Recreate after delete: write → `delete()` → `get()` same name → `/read`
  returns null and `/ping` reports `freshlyConstructed: true` (new
  `constructedAt`). Truly fresh state; re-confirms 46df0c0 from another angle.
- In-flight requests during delete: a request sleeping 3 s, with `delete()`
  issued 500 ms in, rejects at the delete moment (~500 ms, not 3 s) with
  `Facet was deleted.` `delete()` itself neither blocks nor throws. No hangs,
  no silent success.
- Bonus: `delete()` on a never-created facet name is a harmless no-op.

## 5. 10 GB shared-vs-independent — intentionally NOT run

Non-blocking for the design (independent-per-facet is the working assumption;
a shared limit would only cap capacity, not break the design). Procedure if
ever needed: (1) fill one facet to ~9–9.5 GB (e.g. ~9,500 × 1 MB rows);
(2) attempt small writes (a few MB) to a sibling facet and to the parent DO —
shared model fails near the cap, independent model succeeds unaffected;
(3) optionally push the filler toward 9.9 GB+ to narrow the boundary.
Estimated cost: well under $1 (~9,500 rows written; ~9.5 GB held for <1 hour,
prorated from the monthly GB rate; duration is I/O-wait-free per cost
invariant #2). Verify current rates at
https://developers.cloudflare.com/durable-objects/platform/pricing/ before
running. The real bottleneck is wall-clock time to write 9.5 GB, not money.

## Unexpected findings summary

1. Repo-pinned wrangler/workerd predates facets entirely — not merely
   inconvenient; the API is absent locally.
2. Facets cannot hold alarms (previously undetermined; now definitively
   rejected with a clear error message).
3. Mass facet creation can hit a rare transient "DO storage reset" error
   (2 of 20+ bursts) — retry logic required in production paths.
