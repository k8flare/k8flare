# Platform verification spikes (S1–S8)

The k8flare v2 rewrite (`feat/v2-rearchitecture`) is designed around
2026-era Cloudflare features: Dynamic Workers, DO Facets, Cloudflare
Containers, R2, and (pending verification) Cloudflare Mesh. This document
is where we record what we actually confirmed by running these features,
not just what the official docs say.

This is where CLAUDE.md's inviolable rule #2 — "verify by actually running
it; don't write source-reading conclusions as fact" — gets put into
practice. There's precedent for this being expensive to skip: the earlier
conclusion that "kube-proxy hangs the Worker" turned out to be wrong (the
real cause was stale DO state). Locking in design decisions from
documentation or guesswork alone has a proven cost.

## How to read this

- Each spike is defined in the v2 rewrite plan's Phase 1 as "throwaway code
  is fine, only the results need to be recorded." The verification code
  itself doesn't need to stay in the repo.
- **Status** is one of: `not started` / `partially confirmed` / `verified`.
- **Confirmed facts** must always carry a source (commit hash, official doc
  name, changelog date). If the source URL isn't recorded in this document,
  say so explicitly rather than guessing at one.
- **Honest correction convention**: if a previously written "confirmed
  fact" or decision later turns out to be wrong, don't rewrite or delete
  it — append a dated entry with what happened to the "Correction log" at
  the end (CLAUDE.md inviolable rule #4).

## Spike list

| # | What's being verified | Status | Primary dependent phase |
|---|---|---|---|
| S1 | Facets (limits, storage accounting, parallelism, alarms, delete) | partially confirmed | Phase 4 (storage v2) |
| S2 | Dynamic Workers Loader (bundling WASM, size limits, env bindings) | not started | Phase 2 / Phase 4 |
| S3 | Containers (startup, onActivityExpired, cold start, arbitrary images, UDP, wrangler dev) | not started | Phase 5 route B / Phase 7 |
| S4 | Cloudflare Mesh (billing scope, flannel prototype, Cluster DNS replacement) | not started | Phase 9 |
| S5 | WASM isolate singleton-ization (syumai fork) | not started | Phase 2 (apiserver) |
| S6 | R2 (PVC access isolation, S3 access from Containers) | not started | Phase 8 |
| S7 | Re-verifying apiserver residency (double-checking the rejection) | not started | Final confirmation of the rejection decision |
| S8 | Whether controllers can run WASM-resident | not started (in progress) | ★Highest priority. Decides Phase 5's execution technique (Containers fallback ruled out 2026-07-02 — see Correction log) |

---

## S1: Facets

**Verification items**
- Practical limit on facet count
- Storage accounting (is the 10GB shared between parent and children, or
  independent per facet?)
- Facet execution parallelism
- Whether alarms work inside a facet
- `delete()` semantics (whether it can be repurposed for namespace
  deletion)
- (Method) reuse the existing KOOFFICE live-verification deployment

**Status**: partially confirmed

**Confirmed facts**

Source: commit `46df0c0` ("Add empirical Facets verification against a
real KOOFFICE deployment", 2026-07-02). A throwaway supervisor-DO Worker
was deployed to the real KOOFFICE account and the documented facets
pattern (`cloudflare:workers`'s `DurableObject` + `worker_loaders`) was
exercised directly (this finding is also already reflected in
`docs/multi-tenancy-and-hosting.md`).

1. A facet's `class` must come from the Dynamic Workers loader
   (`env.LOADER.get(...).getDurableObjectClass(...)`). Passing a plain,
   statically-imported `DurableObject` subclass to `ctx.facets.get(name, ()
   => ({ class: LocalClass }))` fails at runtime with `TypeError: Incorrect
   type for the 'class' field on 'StartupOptions': the provided value is
   not of type 'DurableObjectClass or LoopbackDurableObjectNamespace or
   LoopbackColoLocalActorNamespace'`. This isn't stated as a requirement
   anywhere in the official docs (every sample just happens to use the
   loader) — confirmed here to actually be one. → Our own CRD-as-facet
   design must go through the same `worker_loaders` / `LOADER` machinery
   as user-supplied code.
2. Storage isolation holds under real data volume. Independent keys were
   written into two sibling facets and the supervisor, and cross-reads
   confirmed to return nothing. `abort()` preserves a facet's data across
   the next `get()`. `delete()` genuinely destroys it (a subsequent
   `get()` returns a fresh, empty facet). Neither operation affects
   sibling facets or the supervisor. → This backs the design (Phase 4) of
   repurposing `delete()` for namespace deletion.
3. `PRAGMA` statements inside `ctx.storage.sql.exec()` are rejected with
   `Error: not authorized: SQLITE_AUTH` — Cloudflare deliberately blocks
   this introspection path. There's no way for application code to
   directly query a facet's or a DO's actual SQLite size (no equivalent
   wrangler CLI command either). → Self-reporting "how close to 10GB are
   we" requires the application to accumulate its own byte count on
   write, rather than asking SQLite (not implemented yet — remains an
   open design issue).
4. ~1.01GB was written into one facet (1,010 × 1MB rows, confirmed via
   `SELECT COUNT(*)`), while concurrently writing 100MB into the
   supervisor and creating a new sibling facet — all succeeded
   immediately with no errors, and stayed isolated from the 1GB facet's
   data. → This confirms coexistence and isolation hold under real data
   volume, but **1GB is too small to distinguish shared vs. independent
   10GB** (negligible against a 10GB ceiling under either model).
   Settling this needs a boundary test — filling one side to ~9GB+ and
   checking whether the other side is constrained — which hasn't been
   run.

Based on the above empirical verification, the v2 rewrite plan adopts the
following as confirmed facts:

- Each facet has its own independent SQLite DB (confirmed empirically,
  item 2 above).
- Loading a facet class requires the Worker Loader (confirmed empirically,
  item 1 above; matches the official blog post's description too).
- **Whether the 10GB is shared between parent and children or independent
  remains officially undocumented.** This plan **designs on the
  assumption that it's not shared (10GB independent per facet)**
  (rationale: if it turns out to be shared, the design simply hits the
  ceiling naturally without breaking; but assuming shared when it's
  actually independent throws away real scale at the design stage. The
  downside is asymmetric, so we assume the optimistic side and fix only
  the capacity design if the boundary test proves it wrong).
- Facet execution parallelism remains unverified. The constraint that all
  traffic funnels through the parent DO's single thread holds under
  either assumption, so the design doesn't depend on this.

The general DO/Facets platform numbers (10GB storage per SQLite DO,
~1,000 req/s soft ceiling, single-threaded, 2MB max value/row, 32,768
WebSockets/DO, 30-day PITR; DO Facets is an Open Beta on Dynamic Workers,
Agents Week April 2026) are confirmed in `docs/multi-tenancy-and-hosting.md`'s
Verified Cloudflare platform facts table (sources:
[DO limits](https://developers.cloudflare.com/durable-objects/platform/limits/),
[DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/),
[facets blog post](https://blog.cloudflare.com/durable-object-facets-dynamic-workers/),
[facets docs](https://developers.cloudflare.com/dynamic-workers/usage/durable-object-facets/)).

**Open questions**

- Practical limit on facet count (unverified — this pass only tested a
  3-way setup of 2 sibling facets + supervisor).
- Boundary test for shared vs. independent 10GB (filling one side to
  ~9GB+, not yet run).
- Whether alarms work inside a facet (unverified).
- Facet execution parallelism (unverified; `docs/multi-tenancy-and-hosting.md`
  also flags this as "undocumented" and deliberately avoids depending on
  it in its design).
- **The assumption direction conflicts with the existing
  `docs/multi-tenancy-and-hosting.md` text**: that document conservatively
  assumes the 10GB is *shared* (reasoning: the downside of wrongly
  assuming independent is worse). This plan (v2 rewrite), by contrast,
  assumes *not shared* (rationale above). Both are different risk
  assessments drawn from the same underlying fact — "officially
  undocumented" — and the two docs will remain contradictory until this
  is settled by measurement. Once the boundary test is done, one of them
  needs to be updated as an honest correction.

---

## S2: Dynamic Workers Loader

**Verification items**
- Whether a dynamically-loaded Worker can bundle a WASM module
- The size limit on loaded code
- Whether bindings can be passed via `env`

**Status**: not started

**Confirmed facts**

- Dynamic Workers went to open beta in March 2026. Source: the v2 rewrite
  plan's "key facts confirmed during investigation" (primary source URL
  not recorded in this document — to be added when referenced).
- That loading a facet class requires the Dynamic Workers loader was
  confirmed empirically in S1 (`46df0c0`). However that only confirms "a
  facet class can be loaded" — the things S2 asks about (bundling a WASM
  module, the size limit on loaded code, passing `env` bindings) are all
  still unverified.

**Open questions**

- The size limit for loading apiserver (Go WASM, 7.07MiB gzip) via
  Dynamic Workers (unconfirmed whether it shares the Worker's own 10MiB
  Paid-plan limit or has a separate budget).
- Whether `env` bindings (DO namespaces, etc.) can be passed to loaded
  code.
- If S8 lands on the WASM-resident design for controllers, whether
  controllers also need to go through the Loader (the relationship
  between Cluster DO's service-binding calls and the Loader hasn't been
  worked out).

---

## S3: Containers

**Verification items**
- Starting the `cmd/scheduler` / `cmd/controller-manager` images and
  keeping a long-lived watch to apiserver
- `onActivityExpired` override behavior (does it stay resident if
  `stop()` is never called?)
- Cold start time (measured on this project's own images)
- Whether arbitrary images can be run dynamically (this is what makes the
  Pod backend viable; if not, settle on an "allowlist of
  wrangler-defined images" approach)
- Whether outbound UDP works
- The Containers + DO local dev experience under `wrangler dev`

**Status**: not started

**Confirmed facts**

- Cloudflare Containers is GA (April 2026). Instance ceiling of 4 vCPU /
  12 GiB / 20 GB disk; a container is paired 1:1 with a DO and scales to
  zero (sleeps after timeout); built-in autoscaling wasn't shipped at GA.
  Source: `docs/multi-tenancy-and-hosting.md`'s Verified Cloudflare
  platform facts table (source:
  [Containers limits](https://developers.cloudflare.com/containers/platform-details/limits/)).
- If `onActivityExpired()` is overridden and `stop()` / `destroy()` is
  never called, the container won't stop on its own. Source: a Warning
  note in the official docs (per the v2 rewrite plan; primary source URL
  not recorded in this document — to be added when referenced). → This is
  the basis for the self-managed lifecycle design (demand-start/idle-stop,
  route B).
- As a general figure, Containers cold starts are typically reported at
  1–3 seconds, with real-world cases reaching 3–15 seconds (per
  architecture articles and measurement blog posts, as cited in the v2
  rewrite plan — orders of magnitude apart from a Workers isolate's
  sub-5ms warm-up). **This is a general figure from other projects'
  measurements, not a measurement of this project's own scheduler/KCM
  images** (project-specific measurement happens in S7).

**Open questions**

- Whether arbitrary OCI images can be run dynamically (this is the very
  precondition for the Pod backend to work; if not, the design needs to
  change to an allowlist approach).
- Whether outbound UDP works (affects k3s/flannel-style networking).
- The Containers + DO local dev experience under `wrangler dev`.
- Measured cold-start time for this project's own scheduler/KCM images
  (→ planned for S7).
- The call granularity of `onActivityExpired` (needed to tune route B's
  idle-timeout).

---

## S4: Cloudflare Mesh

**Verification items**
- Whether it can be used within Workers Paid (license/billing check)
- A throwaway 2-node flannel-over-Mesh prototype
- Whether it can replace Cluster DNS (can Mesh/Gateway name resolution
  return `*.svc.cluster.local`? If not, keep the CoreDNS plan)

**Status**: not started

**Confirmed facts**

(None yet. `docs/cloudflare-mesh-networking.md` has a general survey of
Mesh itself, but none of what this spike asks — billing scope, prototype
behavior, DNS-replacement viability — has been verified empirically.)

**Open questions**

- All verification items are still not started.
- The fallback if this isn't adopted (a CoreDNS Deployment + `kube-dns`
  Service) is already designed as the Phase 4 procedure in
  `docs/general-purpose-k8s-plan.md` (treat that as authoritative if S4
  fails).

---

## S5: WASM isolate singleton-ization

**Verification items**
- Proving out isolate singleton-ization (the syumai fork's doneCh guard +
  mutable context holder)
- Whether cross-request IoContext errors occur
- The `WASM_INSTANCE_REUSE` fallback flag

**Status**: not started

**Confirmed facts**

- The current (v1) design re-instantiates the Go runtime on every request
  (only module compilation is cached). Every API call pays a Go startup
  tax, and concurrent requests risk OOM against the 128MB isolate limit.
  Source: the v2 rewrite plan's analysis of the current state ("key facts
  confirmed during investigation"). This is the motivation for verifying
  singleton-ization in S5.

**Open questions**

- All verification items are still not started. Whether S5 succeeds
  directly determines apiserver's (Phase 2) latency and stability, and
  it's continuous with the fork work needed for S8 (controllers WASM
  residency) — the same fork work is expected to resolve both; see S8
  for detail.

---

## S6: R2

**Verification items**
- Per-PVC access isolation (bucket/prefix + scoped tokens)
- S3 API access from Containers

**Status**: not started

**Confirmed facts**

(None)

**Open questions**

- All verification items are still not started. This is the precondition
  for Phase 8 (R2 PV/PVC backend).
- Whether volumes are provided to Pods via a FUSE mount or an
  S3-compatible endpoint + injected credentials depends on the results of
  both this spike and S3 (Containers' constraints).

---

## S7: Re-verifying apiserver residency (double-checking the rejection)

**Verification items**
- Measure the actual cold-start time for this project's own scheduler/KCM
  image sizes
- Compare against the general figures above (typical 1–3s, worst case
  15s; see S3)
- Final confirmation of the decision to keep apiserver on WASM (revisit
  if the numbers diverge significantly from expectations)

**Status**: not started

**Confirmed facts**

The provisional rejection decision and its rationale, pending S7, based
on the v2 rewrite plan's "Rejected idea: also move apiserver to
Containers" section (subject to reopening if S7's measurements diverge
significantly):

- Latency comparison: a Workers isolate warms up in under 5ms. Cloudflare
  Containers cold-starts typically in 1–3 seconds, and 3–15 seconds in
  real-world use (the same general figures as S3, per architecture
  articles and measurement blog posts). That's 3–4 orders of magnitude
  apart.
- apiserver is on the hot path (every kubectl command goes through it).
  scheduler/KCM are asynchronous reconcilers the user isn't directly
  waiting on. This is where the two diverge on "does it need to stay
  warm at all times," even though both could run on Containers.
- Rough cost: keeping one cluster's 1vCPU+1GiB instance warm 24 hours a
  day works out to vCPU cost $0.00002/vCPU-sec × 2,592,000 sec/month ≈
  $52, plus memory cost $0.0000025/GiB-sec × 2,592,000 sec/month ≈ $6.5,
  for **~$58/cluster/month** (the plan's estimate). Under a hosted-product
  model with many idle clusters (the k8flare.com concept), this is fatal.
  If it isn't kept warm, the first `kubectl get` after idling waits
  anywhere from a few seconds to 15 seconds, which breaks down as an
  interactive CLI.
- The touted benefit of moving to Containers — "easier to keep up with
  upstream versions" — is runtime-independent: `cmd/k8flare-gen` is
  designed to generate from a table plus the `go.mod` pin, and gets the
  same benefit whether the generation target is WASM or a native
  Container binary. WASM's specific burden is size discipline (avoiding
  internal types, externalizing OpenAPI, etc. — Phase 3), not
  version-tracking itself.
- **Provisional conclusion (rejected)**: apiserver stays on WASM/Worker
  (lowest latency wins). S7 is the final confirmation of this conclusion
  via project-specific measurement, and hasn't been carried out yet.

**Open questions**

- Measured cold-start time for this project's own scheduler/KCM images
  (not yet measured).
- Whether the decision needs revisiting if the measurement diverges
  significantly from the general figures.

---

## S8: Whether controllers can run WASM-resident (★highest priority)

This determines Phase 5's (controllers implementation) execution
technique. **S8 determines which WASM execution shape is adopted for
`workers/controllers` — not whether to fall back to Containers**
(superseded by user decision, 2026-07-02; see the Correction log at the
end of this document for the full record and sources). This is taken up
before the other spikes because it still decides how
`workers/controllers` is built.

**Verification items**

(a) Whether forking syumai to keep a Go program alive as a "response
    stream that's never closed" is achievable
(b) Running a load that simulates client-go's informers (multiple
    goroutines concurrently holding long-lived watches) for several
    hours, and checking whether actual consumed CPU-ms stays as small as
    expected (measure CPU time via `wrangler tail` etc.)
(c) Whether ~10–15 concurrent long-lived streams stay stable within the
    post-2026-04 relaxed connection limits
(d) Whether the same "no wall-clock limit" property holds for internal
    calls made via service bindings too

**Status**: not started (in progress)

**Branch condition**: if all four of (a)–(d) check out,
`workers/controllers` is designed as a Go WASM-resident process using
the plain stream-resident technique tested here. If any one of them
fails badly, the fallback is a different **WASM-only** execution
shape — candidates: DO-hosted event-driven execution,
WebSocket-hibernation re-entry, wake-on-write — not Containers
(the Containers fallback and BYO VM framing this paragraph originally
described are superseded by user decision, 2026-07-02; see the
Correction log). Whatever fails gets recorded here with the specifics of
how it broke, as the basis for choosing an alternative shape.

**Confirmed facts**

- **Workers/DO billing is CPU time actually consumed only (not
  wall-clock). Waiting on I/O is free**, an HTTP streaming response has
  no wall-clock limit, and only the CPU time limit (5 minutes per
  invocation) applies. Source: the v2 rewrite plan's "key facts confirmed
  during investigation" (primary source: Cloudflare Workers pricing
  docs, URL not recorded in this document — to be added when
  referenced). → This is the basis for S8's hypothesis that a workload
  shaped like an informer (holding a watch open, mostly waiting on I/O)
  could end up nearly free on Workers.
- **Concurrent connection limits were relaxed on 2026-04-09**: only the
  momentary "awaiting headers" phase is still capped at 6, while
  established long-lived streams became unlimited. Source: the
  Cloudflare changelog, dated 2026-04-09 (the plan records this as
  confirmed; the specific entry URL isn't recorded in this document — to
  be added when referenced). → Directly relevant to (c), "do ~10–15
  concurrent long-lived streams stay stable" (they shouldn't run into the
  momentary 6-connection headers-wait cap, but stability once
  established is still unverified).
- The current syumai/workers design is "one request = one execution; the
  WASM instance ends when the response closes." Keeping a Go program
  alive as a response stream that's never closed requires a fork
  (continuous with S5's isolate-reuse fork — likely the same fork work
  resolves both, per the plan).

**Open questions**

- (a)–(d) are all unverified.
- kube-scheduler/KCM internally run a per-informer goroutine, workqueue
  workers, and periodic resync timers concurrently. Whether this scale of
  concurrent I/O-waiting stays stable for hours to days on a
  `GOOS=js/wasm` target is unproven.
- Whether client-go's transport (via syumai's fetch-based RoundTripper)
  correctly handles reconnection, backoff, and resourceVersion
  continuity for long-lived watch connections is unproven.

---

## Correction log (honest corrections)

**2026-07-02 — S8's "route A vs. route B" branch condition superseded by
a user decision.** The S8 section above (and the "Route B: Containers"
framing in `docs/cost-model.md`) originally treated Containers +
demand-start/idle-stop as the fallback if plain WASM stream-residency
(verification items (a)–(d)) proved infeasible. Per user decision,
recorded in commit `62c9c43` ("Record user decision: controllers run as
WASM on Workers/DO, no Containers fallback") and the v2 rewrite plan's
"controllers 実行方式(追加指示 2026-07-02)" entry, **that fallback is no
longer available**: kube-scheduler/KCM must run as WASM on Workers or
Durable Objects regardless of S8's outcome. S8 therefore no longer
selects between WASM and Containers — it verifies whether the specific
technique in (a)–(d) is viable. If it isn't, the plan is to explore
alternative **WASM-only** designs instead (candidates named in the
decision: DO-hosted event-driven execution, WebSocket-hibernation
re-entry, wake-on-write), not to fall back to Containers. The
verification items (a)–(d) and status above are unchanged — they still
describe what's being tested. The S8 intro and branch-condition text
have since been updated in place to state the new branching directly;
this entry preserves the original reasoning, wording, and sources for
that change.
