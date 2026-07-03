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

| #   | What's being verified                                                                                                                                                       | Status                                                                                                                                                                                                                                                                                                                                                                                                                                                                | Primary dependent phase                                               |
| --- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| S1  | Facets (limits, storage accounting, parallelism, alarms, delete)                                                                                                            | verified (the 10GB shared-vs-independent boundary test alone was intentionally not run — non-blocking)                                                                                                                                                                                                                                                                                                                                                                | Phase 4 (storage v2)                                                  |
| S2  | Dynamic Workers Loader (bundling WASM, size limits, env bindings)                                                                                                           | verified (local wrangler dev; production limits unconfirmed)                                                                                                                                                                                                                                                                                                                                                                                                          | Phase 2 / Phase 4                                                     |
| S3  | Containers (startup, onActivityExpired, cold start, arbitrary images, UDP, wrangler dev)                                                                                    | verified (local + desk research; production confirmation of egress-policy enforcement and UDP blocking still open)                                                                                                                                                                                                                                                                                                                                                    | Phase 7 (controllers motivation dropped — WASM-only by user decision) |
| S4  | Cloudflare Mesh (billing scope, flannel prototype, Cluster DNS replacement)                                                                                                 | verified (desk research)                                                                                                                                                                                                                                                                                                                                                                                                                                              | Phase 9                                                               |
| S5  | WASM isolate singleton-ization (syumai fork)                                                                                                                                | partially confirmed (doneCh reuse fork verified; state persists across reused-instance requests; a _new_ timer wait fails only when nothing else is concurrently active — an open stream, a blocked outbound read, or a `ctx.waitUntil` task all keep the whole scheduler pumped for every goroutine — see S8)                                                                                                                                                        | Phase 2 (apiserver)                                                   |
| S6  | R2 (PVC access isolation, S3 access from Containers)                                                                                                                        | verified (desk research + one read-only check)                                                                                                                                                                                                                                                                                                                                                                                                                        | Phase 8                                                               |
| S7  | Re-verifying apiserver residency (double-checking the rejection)                                                                                                            | not started                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Final confirmation of the rejection decision                          |
| S8  | Whether controllers can run WASM-resident (reframed 2026-07-02: WASM execution-_shape_ design material, not a go/no-go gate — Containers isn't an option under any outcome) | partially confirmed — (a)(c)(d) verified locally with real 10+ min runs; the outbound-`net/http` crash found mid-spike has a verified one-file library-level fix (`wasm_exec.js` patch); startup-tax measured against the real apiserver binary (~14ms cold); `ctx.waitUntil` confirmed to keep the whole scheduler pumped with no client connected (45s+ observed) — see the S8 follow-up subsection for the failure-mode layer table and candidate execution shapes | ★Highest priority. Informs which WASM execution shape Phase 5 adopts  |

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

**Status**: verified (the 10GB shared-vs-independent boundary test alone
was intentionally not run — non-blocking, see the boundary-test entry
under Open questions below)

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

Continuation of this verification (`spikes/s1-facets/FINDINGS.md`,
commit `e19280c`, 2026-07-02) covered the remaining S1 verification
items empirically, both locally and against a fresh throwaway KOOFFICE
deployment (`k8flare-verify-facets`, deployed for this spike and deleted
afterward — deletion double-checked via a 404 and
`wrangler deployments list` returning code 10007). No secrets were used;
no alarms were ever successfully armed (facets reject `setAlarm`, and
the supervisor DO never called it).

5. **Facet count**: no ceiling found up to 1,000, tested both locally
   and in production via staged growth (100 → 250 → 500 → 1,000, each
   `/ping` forcing real instantiation). Production was notably faster
   than local for the final batch (911ms vs. 3,856ms for the last 500
   creations). 1,000 was the requested target — higher counts are
   unexplored. → The namespace = facet design (Phase 4) is viable at the
   scale tested.
6. **Mass facet creation can hit a rare transient error**:
   `Internal error in Durable Object storage caused object to be reset`
   appeared twice across 20+ burst operations (once in an early
   production batch, once in a parallel n=20 test), with no correlation
   found to facet count or batch size; an immediate retry with a fresh
   prefix succeeded both times, and five follow-up bursts all passed
   cleanly. → **Any production code path that creates multiple facets
   (e.g. namespace creation) needs a retry wrapper around facet
   creation.**
7. **Facet dispatch is not serialized**, confirmed both locally and in
   production: an I/O-bound workload (`setTimeout`-based
   `/ping?sleepMs=N`) run via `Promise.all` across N facets showed warm
   re-runs collapsing to a flat ~230ms for both n=100 and n=300 (vs. a
   serial-execution floor of ≥4,000ms for n=20 alone); production n=20
   ×5 runs measured 92–188ms. **This proves non-serialized dispatch for
   I/O-bound work specifically, not multi-core parallelism for
   CPU-bound work** — the design's expectation of await-based I/O work
   inside facets is what this actually needs.
8. **Alarms inside facets are explicitly and deliberately forbidden**,
   confirmed in production: `ctx.storage.setAlarm()` inside a facet
   throws immediately with `Error: Facets currently cannot set alarms.`
   (a deliberate rejection, not a generic failure — untestable locally,
   see item 10 below). → Event-armed alarms (controller re-arm,
   safety-net alarms) must live on top-level DOs only;
   "namespace-level alarm-driven work inside its own facet" is now a
   closed option, not just an unverified one.
9. **`delete()` semantics reconfirmed from a new angle**, identical
   locally and in production: recreating a facet after `delete()`
   yields genuinely fresh state (`freshlyConstructed: true`, new
   `constructedAt`); an in-flight request (mid 3s sleep) rejects at the
   moment of deletion (~500ms in, not the full 3s) with
   `Facet was deleted.` — `delete()` itself neither blocks nor throws;
   and `delete()` on a facet name that was never created is a harmless
   no-op.
10. **Local tooling constraints** (both discovered during this pass):
    the repo-pinned wrangler 4.77.0 (workerd 1.20260317.1) **does not
    implement facets at all** (`ctx.facets` is `undefined`, predates the
    Agents Week Apr 2026 facets launch) — `npx wrangler@latest` (4.106.0,
    used for this spike) is required for any local facets work until the
    repo pin is bumped. Separately, even on `wrangler@latest`, **alarms
    on SQLite-backed DOs (facet or not) cannot be tested locally at
    all** — `ctx.storage.setAlarm()` throws
    `Error: alarms are not yet implemented for SQLite-backed Durable
Objects` — so item 8 above, and any future Phase 4 alarm-parking
    test, can only be verified against a real deployment.

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
- Facet execution parallelism is now confirmed non-serialized for
  I/O-bound work (item 7 above; not proven for CPU-bound work, which the
  design doesn't require). The constraint that all traffic funnels
  through the parent DO's single thread for facet _dispatch_ still
  holds — this finding is about concurrent execution once dispatched,
  not about removing that single-thread hop.

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

- **Boundary test for shared vs. independent 10GB** (filling one facet
  to ~9–9.5GB, then testing whether a sibling facet and the parent DO
  can still take small writes) is **intentionally deferred, not
  blocking**: the working assumption (independent-per-facet) only
  affects capacity headroom if wrong, not correctness, and the
  procedure is cheap to run later (estimated well under $1, per
  `spikes/s1-facets/FINDINGS.md` item 5 — the real cost is wall-clock
  time to write ~9.5GB, not money). Run it before Phase 4 capacity
  planning leans on the assumption being right.
- Facet count ceiling beyond 1,000 is unexplored (1,000 was the
  requested target and passed cleanly; higher counts untested).
- **The assumption direction conflicts with the existing
  `docs/multi-tenancy-and-hosting.md` text**: that document conservatively
  assumes the 10GB is _shared_ (reasoning: the downside of wrongly
  assuming independent is worse). This plan (v2 rewrite), by contrast,
  assumes _not shared_ (rationale above). Both are different risk
  assessments drawn from the same underlying fact — "officially
  undocumented" — and the two docs will remain contradictory until this
  is settled by measurement. Once the boundary test is done, one of them
  needs to be updated as an honest correction.
- Relatedly, `docs/multi-tenancy-and-hosting.md` also flags facet
  execution parallelism itself as "undocumented" and deliberately
  avoids depending on it — that framing is now partly superseded (the
  I/O-bound case is confirmed non-serialized here, see item 7 above) but
  the sibling doc hasn't been updated to reflect it. Reconciling that is
  a follow-up for whoever next touches that file, not done here.
- Bumping the repo-pinned wrangler version (currently 4.77.0, predates
  facets) is a prerequisite for any further **local** facets work —
  `npx wrangler@latest` works today but isn't the repo default.

---

## S2: Dynamic Workers Loader

**Verification items**

- Whether a dynamically-loaded Worker can bundle a WASM module
- The size limit on loaded code
- Whether bindings can be passed via `env`

**Status**: verified (local `wrangler dev`; production limits unconfirmed)

**Confirmed facts**

Source: `spikes/s2-loader/FINDINGS.md` (empirical, 2026-07-02), exercised
with real HTTP requests against a running multi-config `wrangler dev`
session (wrangler 4.77.0) — not doc-reading. Repro commands are recorded
in that file.

1. **WASM modules load fine**: `modules[k] = { wasm: ArrayBuffer }` in
   `WorkerCode`, and the import resolves to an uninstantiated
   `WebAssembly.Module` (matches
   `@cloudflare/workers-types@4.20260317.1`'s
   `WorkerLoaderModule.wasm?: ArrayBuffer` field). Only a raw
   `ArrayBuffer` is accepted — neither a base64 string nor a precompiled
   `WebAssembly.Module`. Not tested: WASM as the `mainModule` itself
   (only "JS main imports wasm" was exercised).
2. **Size**: no limit was hit locally up to 500MB of loaded JS text;
   load time grows superlinearly (33s at 500MB), though the test method
   (one giant string literal the JS engine must parse) may itself be the
   dominant cost rather than the Loader — the curve shape is indicative
   only. **Production limits (e.g. whether the ordinary 10MiB gzip
   script cap applies to loaded code) remain unverified.**
3. **`env` binding forwarding — the most important finding of this
   spike**: plain values and `Fetcher` (service bindings) pass through
   `WorkerCode.env` with a real RPC round-trip confirmed; **both
   `DurableObjectNamespace` and `DurableObjectStub` fail** with
   `DataCloneError: Could not serialize object of type
"DurableObjectNamespace"/"DurableObject". This type does not support
serialization.` — and if any DO-typed key is present, the whole `env`
   clone fails outright (offending keys aren't silently skipped). →
   **Any loader-loaded module (facet class, DynamicWorker) that needs to
   reach a DO must be handed a `Fetcher` to a fronting Worker/RPC
   entrypoint instead of a DO namespace/stub directly.** KV/R2/D1/Queues/
   AI/Vectorize/Workflows binding types were not tested.
4. **`globalOutbound`** has three real modes, verified against actual
   outbound fetches: `null` blocks outbound entirely (documented error
   message confirmed), `undefined` allows real internet access (verified
   against a live external URL), and a genuine `Fetcher` (service
   binding) proxies **all** outbound fetches through it regardless of
   target URL. A duck-typed plain object (`{ fetch: async ... }`) is
   rejected with a `TypeError` — outbound filtering must be a real
   Worker bound via `services`, not an inline JS object.
5. **`.get(id, factory)` caching** behaves as the content-hash-key
   design assumes: the factory runs only on the first `.get()` for a
   given id (cache miss), module-scope state survives across subsequent
   requests for that id even though `.get()` is called every request,
   distinct ids are fully isolated from each other, and loading a new id
   does not evict existing ones. **Idle-eviction timing was not tested
   (production-only)** — directly relevant to the cost invariant of
   never assuming residency.
6. Two local-dev footguns worth adding to the existing list: the
   multi-config `wrangler dev` startup banner's `[not connected]` status
   is unreliable (it stayed `[not connected]` for an entire session
   during which dozens of RPC calls succeeded — never use it as a
   readiness signal), and wrangler 4.77.0's workerd silently falls back
   to its max supported compatibility date (2026-03-17) when a newer
   date like this repo's `2026-03-24` convention is requested
   (pre-existing repo-wide quirk, not introduced by this spike).

**Design implications** (from the research): facet-class delivery
(build-time bundled string + content-hash key) matches the
`.get(id, …)` id model exactly; the
`cacheId = ${namespace}/${name}:${uid}:${resourceVersion}` pattern in
`packages/dynamic-worker/src/run.ts` is validated (spec changes bust the
cache naturally); no loaded worker's `env` should ever be designed to
carry a DO namespace/stub — use Fetcher indirection instead;
`globalOutbound: <Fetcher>` is a proven mechanism for a future
DynamicWorker `networkAccess: "restricted"` mode.

**Open questions**

- Actual module-bytes limit/quota for the Loader in production (only
  local, up to 500MB, was tested).
- Idle-eviction timing of the `.get(id, …)` cache in production.
- Behavior of non-Fetcher, non-DO binding types in `env` (KV/R2/D1/
  Queues/AI/Vectorize/Workflows — untested).
- Untested API surface, not known to be broken: `WorkerLoader.load()`,
  `allowExperimental`, `compatibilityFlags`, `tails`/`streamingTails`,
  `getEntrypoint(name, { props })`.
- If S8 lands on the WASM-resident design for controllers, whether
  controllers also need to go through the Loader (the relationship
  between Cluster DO's service-binding calls and the Loader hasn't been
  worked out) — now sharper given finding 3 above: if controllers ever
  need direct DO access, it would have to go through a `Fetcher`, not a
  passed-through DO namespace/stub.

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

**Status**: verified (local testing + desk research; production
confirmation of egress-policy enforcement and UDP blocking remain open)

**Confirmed facts**

Source: `spikes/s3-containers/FINDINGS.md` (commit `7568a98`,
2026-07-02), measured locally (Docker 29.4.0 via OrbStack, wrangler
4.106.0, `@cloudflare/containers` 0.3.7) plus desk research against
official docs and the Containers changelog (read 2025-09-25 through
2026-07-01). Scope was narrowed mid-spike once the user decision on
controllers (WASM-only, no Containers fallback) landed — S3's original
controllers-fallback motivation is moot, but its Phase 7 (Pod backend)
motivation stands, so this section now speaks primarily to Phase 7.

1. **Real binaries measured**: `cmd/scheduler` 73.5MB (`FROM scratch`) /
   75.6MB (distroless), `cmd/controller-manager` 95.2MB / 97.2MB.
   **`FROM scratch` has no CA bundle** — a request to
   `https://cloudflare.com` from a scratch image failed TLS verification
   (`x509: certificate signed by unknown authority`); distroless adds
   ~2MB and includes the bundle. → **k8flare-provided base images
   (including any Pod base images) should be distroless-family, not
   scratch.** `docker stop` completes in 0.2s (graceful `SIGTERM`
   shutdown via `signal.NotifyContext` works). These images are no
   longer needed for Phase 5 (controllers are WASM-only) but remain
   useful for BYO VM Docker distribution.
2. **`wrangler dev` + Containers + DO developer experience**:
   `wrangler dev` performs a real local `docker build` (same path as
   deploy). **The Dockerfile must contain `EXPOSE <port>`** — setting
   `defaultPort` on the DO class alone fails at startup (undocumented,
   found empirically). Local demand-start latency measured at ≈0.86s —
   **not representative of production cold start** (official figures
   remain 1–3s typical, up to 3–15s in practice, per S7 and
   `docs/multi-tenancy-and-hosting.md`).
3. **`onActivityExpired` override behavior confirmed accurate against
   the official Warning**, comparing two DO variants side by side:
   without an override, the container stops after `sleepAfter` and
   `onStop` fires; with the hook overridden and `stop()`/`destroy()`
   never called, the container stays `healthy` indefinitely and
   **`onActivityExpired` fires repeatedly (not a one-shot hook)** —
   observed every ~20s for a 20s `sleepAfter`. Reading the shipped
   `@cloudflare/containers@0.3.7` source directly (not just docs)
   additionally found: the Container class self-arms a DO `alarm()` at
   most every 3 minutes (or the remaining `sleepAfter`, whichever is
   shorter) while the container runs, and calls
   `ctx.storage.deleteAlarm()` once stopped with no pending schedule —
   i.e. **it parks itself when idle**, matching this repo's event-armed
   alarm rule. → **Quantitative Phase 7 cost consequence: each running
   Pod container implies a DO alarm firing at least every 3 minutes**
   while that Pod is up (folded into `docs/cost-model.md`'s `nodes` row).
4. **Arbitrary images at runtime are not possible**, confirmed via Image
   Management docs and a full changelog read (2025-09-25 through
   2026-07-01, including the 2026-07-01 Google Artifact Registry entry):
   `containers[].image` is fixed at deploy time (a Dockerfile path or a
   fully-qualified registry reference); no API lets a Worker/DO select
   an arbitrary image at runtime, and recent platform changes only
   extend deploy-time configuration. → **Phase 7 v1 is settled on a
   "wrangler-defined image allowlist" model** — `kubectl
run --image=<anything>` cannot work; Pods can only run images
   k8flare has pre-registered. Must be stated honestly in the README.
5. **Outbound UDP is officially unsupported, and non-80/443 ports have
   no handler at all**: DNS resolution is Cloudflare's own resolvers
   only, and the port restriction applies regardless of
   `enableInternet`. → CoreDNS running as a Containers-hosted Pod cannot
   serve UDP:53; VXLAN-over-UDP also can't run on Containers as-is.
   **However, local `wrangler dev` does not enforce any of this** — a
   default-configured container successfully reached a non-80/443 port
   (`host.docker.internal:8846`), and so did a container with
   `enableInternet=false` and no `allowedHosts` explicitly set. → A
   concrete instance of "worked locally ≠ same in production": egress
   policy (`enableInternet`/`allowedHosts`/`deniedHosts`) must be
   re-verified against a real deployment before anything relies on it
   for Pod network isolation.
6. **Container → host long-lived streaming works**: a host-side chunked
   HTTP stream (1s interval × 25 ticks) relayed from inside the
   container preserved timing and completed fully, including under
   `enableInternet=false` (consistent with finding 5's local
   non-enforcement). This proves Docker-level stream mechanics only —
   the production "container → apiserver watch" path (reconnects,
   resourceVersion continuation) remains a Phase 5/7 implementation-time
   concern.
7. **Instance sizing is also deploy-time-fixed** (same-day addendum,
   double-sourced from the Limits page and the 2026-01-05 "Custom
   instance types" changelog entry): `instance_type` is set per
   `containers[]` entry at deploy time (six predefined tiers from `lite`
   1/16 vCPU·256MiB·2GB up to `standard-4` 4vCPU·12GiB·20GB, or a custom
   `{vcpu, memory_mib, disk_mb}` — opened to all users 2026-01-05,
   previously Enterprise-only); runtime `startOptions` can only override
   `envVars`/`entrypoint`/`enableInternet`/`labels`, never size.
   Account-level caps: 1,500 concurrent vCPU / 6TiB memory / 30TB disk /
   50GB image storage — no documented cap on the number of Container
   classes. → **Phase 7 consequence: the allowlist is effectively
   (image × size-tier) pairs**, since honoring Pod
   `resources.requests/limits` needs a separate Container class per size
   tier — realistic v1 is a small curated set of base images × a few
   size tiers, rounding each Pod's requested resources up to the
   nearest tier (to be settled during Phase 7 detailed design).

**Correction recorded by the spike itself**: the report behind this
section initially inferred that S8 had "succeeded" from the task-list
title alone ("WASM 一本化" / WASM-only). That's wrong as to cause — the
WASM-only direction for controllers is a user decision (2026-07-02) made
independently of S8's outcome; S8 was still running when this spike
completed. Recorded here so the incorrect causal claim doesn't
propagate, per this document's honest-correction convention.

**Open questions**

- Whether `enableInternet`/`allowedHosts`/`deniedHosts` are actually
  enforced in production (confirmed **not** enforced locally — see
  finding 5).
- Empirical confirmation that UDP is fully blocked in production
  (currently based on official docs, not a production test).
- How the egress-control sidecar (`docker.io/cloudflare/proxy-everything`)
  is billed.
- Cold-start penalty for pulls from non-Cloudflare registries.
- Project-specific cold-start measurement against a real deployment
  (this was S7's original charter; S7 is now moot for controllers per
  the WASM-only decision, but the measurement is still relevant to
  Phase 7 Pod startup latency).

---

## S4: Cloudflare Mesh

**Verification items**

- Whether it can be used within Workers Paid (license/billing check)
- A throwaway 2-node flannel-over-Mesh prototype
- Whether it can replace Cluster DNS (can Mesh/Gateway name resolution
  return `*.svc.cluster.local`? If not, keep the CoreDNS plan)

**Status**: verified (desk research — no live 2-node prototype was run;
see the recommendation below on redesigning that prototype before
building it)

**Confirmed facts**

Source: `spikes/s4-mesh/RESEARCH.md` (desk research, 2026-07-02),
cross-checked against official Cloudflare and k3s/flannel documentation
(full citation list in that file). This pass answers S4's three
verification items from documentation rather than a live deployment —
see "Open questions" for what still needs a real test.

1. **Billing**: Cloudflare's blog states Mesh includes "50 nodes and 50
   users free... included with every Cloudflare account"
   ([Introducing Cloudflare Mesh](https://blog.cloudflare.com/mesh/)).
   Mesh lives under Zero Trust / Cloudflare One, a separate product
   surface from Workers, and Zero Trust has its own always-available
   Free plan (50 seats, $0). The working inference is that Mesh's
   50-node/user allowance draws from that same Zero Trust Free seat
   pool, so a Workers Paid account can likely enable Mesh at no extra
   cost by separately turning on Zero Trust Free — but no official
   source states it's bundled automatically with Workers Paid, and
   pricing beyond 50 nodes/users isn't published anywhere (confidence:
   medium — involves inference, not a direct statement).
2. **Connection model**: confirms and extends
   `docs/cloudflare-mesh-networking.md` — true L3, CIDR route
   advertisement, scriptable enrollment (confidence: high, multiple
   official docs). New finding: the default transport is MASQUE (QUIC
   over UDP, HTTP/3, TLS 1.3), with WireGuard as an alternative
   ([Zero Trust WARP: tunneling with a MASQUE](https://blog.cloudflare.com/zero-trust-warp-with-a-masque/)).
   This reframes the existing doc's "~14% UDP loss" concern: since
   Mesh's own transport is UDP, that loss characteristic is a
   structural property of everything crossing Mesh (including TCP
   payloads), not something specific to UDP-based applications like
   CoreDNS. Also new: MTU mishandling is a real failure mode, not just
   a tuning recommendation — the official Tips page states that packets
   near 1,460 bytes get pushed over 1,500 bytes by the double
   encapsulation and are silently dropped
   ([Tips and best practices](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/tips/)).
3. **flannel-over-Mesh viability** (desk judgment, not run): splits by
   backend. `host-gw` requires direct L2 connectivity per flannel's own
   docs ([flannel backends.md](https://github.com/flannel-io/flannel/blob/master/Documentation/backends.md)),
   which Mesh's virtual `100.96.0.0/12` address space doesn't provide —
   judged **not viable**, backed by precedent of the same failure mode
   on Tailscale+host-gw
   ([k3s-io/k3s#8372](https://github.com/k3s-io/k3s/issues/8372)).
   `vxlan` only needs L3 UDP reachability, which Mesh does provide, so
   it's viable in principle but means double UDP encapsulation (vxlan
   inside Mesh's own MASQUE/QUIC tunnel), tightening the MTU budget
   further. Separately, **k3s already ships an official alternative
   that doesn't need Mesh at all**: `--flannel-backend=wireguard-native`
   plus `--node-external-ip`
   ([K3s: Distributed hybrid or multicloud cluster](https://docs.k3s.io/networking/distributed-multicloud)).
   This changes what S4 is actually asking: not "can flannel run over
   Mesh" but "what does Mesh add on top of wireguard-native" — the
   leading (untested) hypothesis is NAT traversal, letting BYO VMs join
   without opening any inbound port, since wireguard-native requires
   each node to accept inbound UDP/51820 directly.
4. **Cluster DNS replacement**: judged **not viable**, confidence high.
   Mesh's own hostname routing hasn't shipped yet (officially announced
   for "this summer" 2026,
   [Connect and secure any private or public app by hostname, not IP](https://blog.cloudflare.com/tunnel-hostname-routing/));
   the only programmatically-accessible DNS product, Cloudflare Internal
   DNS, is Enterprise-only
   ([Internal DNS: Get started](https://developers.cloudflare.com/dns/internal-dns/get-started/),
   explicitly requires "an Enterprise account with access to Gateway
   resolver policies"); and Gateway's DNS features (resolver policies,
   Local Domain Fallback) only forward queries to an existing DNS server
   rather than acting as an authoritative source, so they don't remove
   the need to run something CoreDNS-shaped anyway. → The CoreDNS
   Deployment + `kube-dns` Service fallback already named below (and
   detailed in `docs/general-purpose-k8s-plan.md` Phase 4) should be
   treated as the primary plan, not a fallback.
5. **Maturity**: Mesh GA'd 2026-04-14
   ([changelog](https://developers.cloudflare.com/changelog/post/2026-04-14-cloudflare-mesh/)),
   raising the node cap from 10 to 50 at launch. But the Linux client
   that Mesh server nodes actually depend on only reached its own GA on
   2026-06-29
   ([changelog](https://developers.cloudflare.com/changelog/post/2026-06-29-warp-linux-ga/))
   — three days before this research pass. Container/Docker support
   remains "later this year," unchanged from the existing doc. No
   Kubernetes/CNI integration examples were found anywhere.

**Recommendation on the originally-scoped prototype**: if a live 2-node
prototype is still run, it should target `vxlan` (not `host-gw`, now
judged structurally incompatible) and be framed as a comparison against
k3s's own `wireguard-native` backend, since that's the real alternative
Mesh needs to justify itself against.

**Open questions**

- Mesh's pricing beyond 50 nodes/users (per-node or bandwidth-based
  charges) — not published anywhere; would need to ask Cloudflare
  directly.
- Real-world MTU / throughput / UDP loss under vxlan-over-Mesh
  specifically (double encapsulation) — no first-party measurement
  exists; third-party competitor benchmarks (NetBird) exist but
  shouldn't be used as design numbers.
- Whether Mesh's NAT-traversal advantage over wireguard-native (no
  inbound port needed) actually holds up — the core hypothesis for why
  Mesh would be worth using at all, and it's untested.
- Whether Mesh's hostname routing (once shipped this summer) becomes
  programmatically controllable — if so, the Cluster DNS judgment above
  may be worth revisiting.
- The fallback if this isn't adopted (a CoreDNS Deployment + `kube-dns`
  Service) is already designed as the Phase 4 procedure in
  `docs/general-purpose-k8s-plan.md` — per finding 4 above, treat that
  as the primary plan already, not a fallback contingent on S4 failing.

---

## S5: WASM isolate singleton-ization

**Verification items**

- Proving out isolate singleton-ization (the syumai fork's doneCh guard +
  mutable context holder)
- Whether cross-request IoContext errors occur
- The `WASM_INSTANCE_REUSE` fallback flag

**Status**: partially confirmed. Done as part of S8's spike
(`spikes/s8-wasm-resident/`, local `wrangler dev` only, not deployed).

**Confirmed facts**

- The current (v1) design re-instantiates the Go runtime on every request
  (only module compilation is cached). Every API call pays a Go startup
  tax, and concurrent requests risk OOM against the 128MB isolate limit.
  Source: the v2 rewrite plan's analysis of the current state ("key facts
  confirmed during investigation"). This is the motivation for verifying
  singleton-ization in S5.
- **The doneCh guard is real and necessary, and one line fixes it.**
  Unpatched `syumai/workers` v0.32.0 closes a package-level `doneCh`
  unconditionally whenever any request's response body reaches EOF
  (`appCloser.Close()` in `handler_js.go`) — harmless under the library's
  assumed one-instance-per-request model, but the _second_ request ever
  dispatched to a reused instance crashes the whole Go runtime with
  `panic: close of closed channel`, reproduced and logged
  (`spikes/s8-wasm-resident/vendor/syumai-workers-fork/`, unpatched vs.
  patched comparison). Fix: guard the close with `sync.Once` — the only
  line changed in a full fork of the module (confirmed via
  `diff -rq` against the pristine module cache copy).
- **Cross-request IoContext errors: only found for one specific case, not
  as broadly as feared, but for a more fundamental reason than "IoContext
  errors."** `cloudflare.WaitUntil()` called from a request other than the
  one that originally instantiated the WASM module did **not** throw or
  warn in local `wrangler dev` (3 calls, 2 on a stale context, all
  returned `{"result":"ok"}`, task counters matched) — this was
  unexpected and is flagged as not fully understood / possibly
  wrangler-dev-specific, not to be trusted in production without
  re-testing. What _does_ reliably break across a request boundary is
  more basic than any specific Cloudflare binding: **the Go scheduler's
  own JS-side timer/resume machinery (`wasm_exec.js`) only makes forward
  progress for the request whose IoContext hosted the original
  `go.run()` call.** A handler that needs to block on a _new_ timer,
  dispatched to a reused instance as any request other than the
  originating one, fails almost immediately
  (`"The Workers runtime canceled this request because it detected that
your Worker's code had hung..."`, resolved in ~2ms — looks structural,
  not a timeout heuristic) and permanently leaks the stuck goroutines.
  Plain short-lived, non-blocking request/response handlers (the shape
  most apiserver endpoints already have) are unaffected and were
  confirmed working correctly across many reused-instance calls in a
  row (same instance ID, state — e.g. a request counter — correctly
  accumulating call after call). Full detail, exact panic traces, and
  the goroutine-leak evidence: S8 section below and
  `spikes/s8-wasm-resident/` logs.
- **`WASM_INSTANCE_REUSE` fallback flag**: not evaluated — no such flag
  exists in this spike; noting it as still open below.

**Open questions**

- Whether apiserver's actual request handlers (as opposed to this
  spike's synthetic `/status`) are all short-lived and non-blocking is
  not verified — anything in apiserver's handler path that ends up
  blocking on a _new_ timer/goroutine-resume (not just reading request
  state and writing a response) would hit the same wall found in S8.
  Recommend an explicit audit of the request path before adopting
  instance reuse for apiserver.
- The `WASM_INSTANCE_REUSE` fallback flag mentioned in the original
  verification items was not implemented or evaluated in this pass.
- Not verified in production `workerd` — only local `wrangler dev`. See
  S8's "production-only residual items" for the full list; the same
  caveats apply here (this is the same fork/mechanism).

---

## S6: R2

**Verification items**

- Per-PVC access isolation (bucket/prefix + scoped tokens)
- S3 API access from Containers

**Status**: verified (desk research + one read-only account check; write
operations — bucket/token/credential creation — not yet performed)

**Confirmed facts**

Source: `spikes/s6-r2/RESEARCH.md` (desk research + one read-only account
check, 2026-07-02), citing official Cloudflare R2 and Containers
documentation (full citation list in that file, including page
`dateModified` timestamps).

1. **PVC isolation mechanism**: Temporary Access Credentials — minted
   from a parent R2 API token — is the fit-for-purpose primitive. Each
   credential is bound to exactly one bucket and can be narrowed with
   `prefixes` / `objects`; R2 itself enforces the boundary by returning
   403 for anything outside scope (Cloudflare's own worked example
   demonstrates this: 200 for an in-prefix key, 403 for an out-of-prefix
   one). Two minting paths: (a) the Temporary Credentials REST API
   (draws from the account's shared 1,200 req/5min budget), or (b)
   **local JWT signing** (zero Cloudflare API calls, the only path that
   supports `actions`-level scoping today, and Cloudflare's own
   recommended approach for this use case). Long-lived R2 API tokens are
   bucket-scoped only — no prefix restriction exists for them. Presigned
   URLs are single-object/single-operation (max 7 days) and don't fit a
   PV's session-like access pattern.
2. **Bucket-per-PVC vs. prefix-in-shared-bucket**: prefix-in-shared-bucket
   is the recommended shape. The bucket cap (1,000,000/account) is
   generous but bucket-per-PVC spends down a finite, account-wide
   resource on something (PVC count) with no natural ceiling. Bucket
   management operations (create/delete/list/config) share a 50/sec
   limit and the account's 1,200 req/5min REST budget; object read/write
   and locally-signed credential minting touch neither. Bucket-per-PVC
   also means a durable, bucket-scoped secret per PVC; prefix-based
   isolation needs exactly one durable parent secret (e.g. per cluster)
   and mints disposable, self-expiring credentials on demand.
3. **Containers → R2 egress works with no special config**: internet
   access from a Container is on by default. This one required a
   correction during the research itself, worth recording per the
   honest-correction principle: an initial LLM-summarized read of
   `cloudflare/containers`' `docs/egress.md` claimed egress is _blocked_
   by default, contradicted by a second summarized source claiming the
   opposite. Going to primary sources resolved it — both the raw
   markdown of Cloudflare's official outbound-traffic docs and the
   actual `@cloudflare/containers` SDK source on GitHub
   (`enableInternet: ... = true`) confirm **internet access is on by
   default**; the "blocked by default" claim was wrong.
4. **FUSE mounting an R2 bucket is officially supported**, not a
   workaround — Cloudflare's own example Dockerfile installs `fuse` and
   mounts via `tigrisfs` (an S3-compatible FUSE adapter), with no
   `privileged`/`SYS_ADMIN`/`/dev/fuse` configuration exposed anywhere in
   the Container class or `wrangler.jsonc` (inference from absence of a
   config knob — not a documented guarantee, worth confirming against a
   real deployment). Three caveats: no native prefix-scoped mount exists
   (every adapter mounts the whole bucket name; whether a prefix-scoped
   credential can even complete that mount is unverified), whether the
   adapter honors `AWS_SESSION_TOKEN` end-to-end is unconfirmed, and
   **FUSE cannot be verified in local `wrangler dev`** — it reportedly
   needs a real deployment to even attempt (a new local-dev footgun
   worth adding to CLAUDE.md's list once Phase 8 starts touching
   Containers).
5. **A third access pattern exists but isn't recommended for
   tenant-facing PVs**: an `outboundByHost` binding proxy lets a
   Container hit a virtual hostname that a Worker resolves via a real R2
   binding, with zero S3-style credentials ever leaving the Worker/DO
   trust boundary. Not recommended for the general-purpose PV backend
   because it's a bespoke HTTP scheme, not real S3 (no SigV4, no
   `ListObjectsV2`, not FUSE-mountable) — using it would mean
   reimplementing S3 surface, against this repo's reuse-over-reimplement
   principle. Worth keeping in mind for k8flare's own internal component
   storage, where a bespoke protocol is fine.
6. **Read-only account check**: `wrangler r2 bucket list` succeeded
   against the current OAuth session (`k2wanko` account, 17 pre-existing
   buckets unrelated to k8flare) — confirms R2 read access works today.
   No bucket, R2 API token, or temporary credential was created; write
   operations remain unverified for this account.
7. **The one real open problem: credential refresh for long-running
   Pods.** Temporary credentials are deliberately short-lived by design,
   but a PVC can be mounted for the life of a long-running workload
   (days/weeks). Nothing in Cloudflare's docs or the FUSE example
   refreshes an in-place credential — the FUSE example bakes credentials
   into env vars once at container start. Phase 8 needs to resolve this
   before it can rely on short TTLs: either a refresh sidecar (contingent
   on unverified FUSE-adapter re-read behavior) or a deliberately longer
   TTL as a pragmatic v1 trade-off against Cloudflare's own "scope
   narrowly, use short TTLs" guidance.

**Recommendation for Phase 8** (from the research): v1 = prefix-scoped
Temporary Access Credentials, minted via local JWT signing, on a shared
bucket (one per cluster or tenant — not one per PVC), delivered to the
Pod's Container as env vars for direct S3-SDK use. Offer FUSE mounting
as an opt-in pattern once its two caveats above are verified against a
real deployment.

**Open questions**

- The actual `ttlSeconds` minimum/maximum bound for Temporary Access
  Credentials (undocumented).
- Whether a prefix-scoped credential can successfully be used to
  FUSE-mount a whole bucket (root-level list behavior under a
  prefix-restricted credential is the open question).
- Whether `tigrisfs` (or another adapter) actually honors
  `AWS_SESSION_TOKEN` end-to-end.
- Whether R2 bucket names are globally unique or account-scoped (only
  matters if bucket-per-PVC is reconsidered later).
- Whether this OAuth session (or a purpose-built API token) can actually
  create a bucket / R2 API token / temporary credential — only listing
  was verified.
- Whether Cloudflare's FUSE support genuinely needs no privileged-mode
  config in practice, or the docs are simply silent about a knob that's
  required and currently missing from the example.
- Credential refresh for long-running Pod mounts (the core Phase 8
  blocker — see finding 7 above).

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

**Status**: partially confirmed (local `wrangler dev` only; not deployed
— (b)'s real CPU-ms measurement explicitly needs production and was not
attempted). Spike code, raw logs and exact reproduction commands:
`spikes/s8-wasm-resident/` (throwaway, not part of the build).

**Results for (a)(c)(d)**, each backed by a live local run, not source
reading alone (CLAUDE.md rule #2):

- **(a) confirmed, and — correcting the note below — no fork is needed
  for this part.** `GET /stream` on a completely unmodified
  `github.com/syumai/workers` v0.32.0, with the standard generated glue
  (one `WebAssembly.Instance` per request, same as this repo's actual
  `main.go`), stayed open for a full 11-minute `curl -m 660` run: 329
  heartbeat lines, zero gaps, a background "ticker" goroutine and a
  separate, independent "informer-like" goroutine both alive and
  correctly scheduled the entire time (`ticker_count` exactly equal to
  the running sequence number throughout; informer tick reached 132 at
  the 5s cadence over 660s). One live instance for the whole run. This
  works today, with zero library changes, because `Serve()`'s blocking
  `<-Done()` only waits on _that one request's_ own response body — as
  long as the handler keeps writing and the client keeps reading, the
  program simply never returns. See the correction log entry below.
- **(c) confirmed.** 12 concurrent `curl -m 90 .../stream`, launched
  together and run simultaneously with the (a) and (d) 11-minute soaks
  (14 long streams open on one isolate at peak): all 12 completed
  uniformly (same start/end timestamps, 44 heartbeats each, 12 distinct
  instance IDs proving true per-request isolation, zero dropped
  heartbeats, no errors in the dev log).
- **(d) confirmed.** A 2-line TS relay worker
  (`return env.GOWORKER.fetch(request)`) bound via a service binding to
  the Go worker, same 11-minute soak run through the binding: 330
  heartbeats, zero gaps, single instance the whole time. Streaming
  semantics (headers returned early, body trickles in over minutes)
  survive the service-binding hop unchanged — directly relevant since
  `workers/gateway` → `workers/apiserver` and any future
  `workers/controllers` → `workers/apiserver` call are both modeled as
  service bindings.
  Pitfall found along the way: `wrangler dev -c A -c B` only exposes
  **the first** config's port on localhost; list the worker you want to
  `curl` directly first.
- **(b)**: not attempted, as scoped (needs a real deploy + billing
  dashboard/`wrangler tail`).

**Additional item beyond the original (a)–(d): isolate-level WASM
instance reuse across _independent_ requests (the S5 crossover).** Not
one of the original four letters, but explicitly requested alongside
them and directly relevant to whether "start the controller once, let it
keep running" is achievable at all. Full detail is in the S5 section
above; summary: **state reuse works** with a one-line fork
(`sync.Once`-guard a `doneCh` close that upstream closes unconditionally
and which otherwise panics — `panic: close of closed channel` — on the
second request dispatched to a reused instance). **Continuous
goroutine/timer progress across independent requests does not.** With
zero idle gap, a _second_, different request that needs to block on a
fresh timer (e.g. a second `/stream` call) fails almost immediately
(`"...detected that your Worker's code had hung..."`, resolved in ~2ms,
looks structural rather than timeout-based) and permanently leaks the
stuck goroutines. A single request that itself stays open the whole time
(the (a) shape above) does _not_ hit this — the deciding factor is
whether a given request is the one whose IoContext originally hosted
`go.run()`, not idle duration (tested 9s/12s/31s/57s gaps: a live
`main()`-started 2s ticker only ever advances **+1 tick per incoming
request, never proportionally to elapsed time**, when idle). **Practical
implication: the viable resident shape is "instantiate once, keep one
long-lived stream open for the whole program's lifetime" (proven by (a)
above), not "instantiate once, dispatch N independent incoming requests
over time" (S5's literal ask, not reliable beyond short synchronous
handlers).** A controller's own outbound watch to the apiserver is the
natural candidate for that one always-open stream.

**New, unplanned, and currently the single most consequential finding:
outbound `net/http` calls crash the WASM instance, independent of
everything above.** While building a realistic informer simulation (a
goroutine issuing its own outbound `http.Get` to consume a streamed
response — the _client_ side of a watch, which is what
kube-controller-manager/kube-scheduler's informers actually do, as
opposed to (a)'s server-side response streaming), the very first request
to a fresh instance crashed:

```
panic: JavaScript error: Illegal invocation: function called with incorrect `this` reference.
net/http.(*Transport).RoundTrip(...)  roundtrip_js.go:129
```

Reproduced independently on the **completely unmodified** library (not
the S5 fork) with a trivial one-shot `http.Get`, and also with
`github.com/syumai/workers/cloudflare/fetch`'s own `NewClient()` (the
library's documented alternative to the stdlib transport) left at its
default `namespace: js.Global()`. Root cause: `wasm_exec.js` wraps the
real `globalThis` in a `Proxy` so it can inject the per-request
`context`; `js.Global()` in Go resolves to that Proxy, and native
`fetch()` rejects being called with a Proxy as `this` (V8/`workerd`'s
receiver/brand check) — this is a property of syumai/workers v0.32.0
itself, present in the current, unmodified `main.go` this repo already
ships (it hasn't surfaced yet because apiserver's only outbound-shaped
call, the Durable Object fetch, is binding-shaped, not
`js.Global()`-based). **Working fix, verified**: route outbound calls
through a real service binding object instead —
`cloudflare.GetBinding("NAME")` + `cloudflare/fetch.NewClient(fetch.WithBinding(binding))`
— tested against a self-binding, returned a clean response, no panic.
This matches the target architecture (controllers would call the
apiserver via a service binding anyway) but was only tested against a
self-binding toy, not the real apiserver Worker, and does not help for
genuinely arbitrary external URLs (service bindings only target other
Workers in the same account). **If any upstream k8s/k3s controller code
pulled in for Phase 5 calls `http.Get`/`http.DefaultClient`/a bare
`*http.Transport` anywhere in its dependency graph without a binding
override, it will crash the instance.** Recommend treating an explicit
audit of client-go's/KCM's outbound call paths as a prerequisite for
committing to Phase 5, regardless of which WASM-only execution shape
(stream-resident, DO-hosted event-driven, hibernation re-entry) is
ultimately chosen — this bug is orthogonal to that choice.

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
**Update given the results above**: (a)(c)(d) all check out locally, so
the plain stream-resident technique is not ruled out by this round of
testing — but the outbound-`net/http` crash is a hard blocker for _any_
WASM-only shape (it isn't specific to the stream-resident technique) and
must be resolved (at minimum: confirm every outbound call path is
binding-routed) before treating S8 as a green light. Production-only
verification (below) is also still outstanding.

**Confirmed facts**

- **Workers/DO billing is CPU time actually consumed only (not
  wall-clock). Waiting on I/O is free**, an HTTP streaming response has
  no wall-clock limit, and only the CPU time limit (5 minutes per
  invocation) applies. Source: the v2 rewrite plan's "key facts confirmed
  during investigation" (primary source: Cloudflare Workers pricing
  docs, URL not recorded in this document — to be added when
  referenced). → This is the basis for S8's hypothesis that a workload
  shaped like an informer (holding a watch open, mostly waiting on I/O)
  could end up nearly free on Workers. Not itself re-verified here (that
  needs a production deploy, see below); what _was_ verified locally is
  the load-bearing precondition — that the stream and its background
  goroutines actually keep running for minutes at a time without being
  killed (see (a) above).
- **Concurrent connection limits were relaxed on 2026-04-09**: only the
  momentary "awaiting headers" phase is still capped at 6, while
  established long-lived streams became unlimited. Source: the
  Cloudflare changelog, dated 2026-04-09 (the plan records this as
  confirmed; the specific entry URL isn't recorded in this document — to
  be added when referenced). → (c) above is consistent with this (12
  concurrent established streams were stable), though local `wrangler
dev` does not enforce or model production connection limits either
  way, so this isn't an independent confirmation of the limit itself.
- The current syumai/workers design is "one request = one execution; the
  WASM instance ends when the response closes" **for the standard
  generated glue's instantiate-per-request pattern** — but (a) above
  shows that pattern alone already supports keeping _that one request's_
  Go program alive indefinitely with no fork, as long as the handler
  never returns and the client keeps reading. A fork is only required
  for the separate S5 goal of reusing one instance across _multiple,
  independent_ incoming requests (see S5 section and the additional item
  above).

**Open questions**

- (b): real production CPU-ms measurement, not attempted here.
- The outbound-`net/http` crash and its binding-based workaround need
  re-verification against production `workerd` and the real apiserver
  Worker, not a local self-binding toy.
- kube-scheduler/KCM internally run a per-informer goroutine, workqueue
  workers, and periodic resync timers concurrently. Whether this scale of
  concurrent I/O-waiting stays stable for hours to days on a
  `GOOS=js/wasm` target is unproven (this spike ran minutes, not hours;
  and only 1-2 concurrent goroutines per instance, not the full
  KCM-scale count).
- Whether client-go's transport (via syumai's fetch-based RoundTripper,
  or a binding-routed replacement given the fetch bug above) correctly
  handles reconnection, backoff, and resourceVersion continuity for
  long-lived watch connections is unproven.
- Real connection/subrequest limits and isolate eviction behavior under
  concurrency, and whether production idle-isolate eviction changes the
  "one open stream keeps the instance alive" story — `wrangler dev` does
  not model either.
- Whether the surprising "`cloudflare.WaitUntil` didn't throw on a stale
  context" result (S5 section) holds in real `workerd` and isn't a
  wrangler-dev-only emulation quirk.

**Follow-up (2026-07-02, same day): reframed as WASM execution-shape
design material, not a go/no-go gate.** Per user decision, Containers is
not available as a fallback under any outcome (see Correction log
entry above) — scheduler/KCM **will** run as WASM on Workers or Durable
Objects regardless. The remaining work below is therefore in service of
choosing _which_ WASM execution shape, not whether to use one.

**Failure-mode layer attribution.** For every break found in this spike,
which layer owns it, the exact error, and whether a workaround exists:

| #   | What breaks                                                                                      | Layer                                                                                                                                                                                                                                                                               | Exact error                                                                                                                                              | Workaround?                                                                 |
| --- | ------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| 1   | 2nd request dispatched to a reused instance                                                      | syumai/workers library design (`handler_js.go`'s `doneCh`, assumes one instance = one request)                                                                                                                                                                                      | `panic: close of closed channel`                                                                                                                         | Yes — `sync.Once` guard, 1 line, verified                                   |
| 2   | A _new_ `time.Ticker`/`time.Sleep`, on a reused instance, with **nothing else currently active** | Go's wasm scheduler (`wasm_exec.js`'s `setTimeout`-driven resume) × `workerd`'s per-request IoContext isolation (a deliberate platform boundary, not a bug)                                                                                                                         | `"...detected that your Worker's code had hung..."`, ~2ms, looks structural                                                                              | Narrower than first thought — see below                                     |
| 3   | Any outbound `net/http` call, on **any** instance (not reuse-specific)                           | Go stdlib (`net/http/roundtrip_js.go` calling `js.Global().Call("fetch",...)`) × syumai's `wasm_exec.js` `Proxy`-wrapped `globalThis` (added to inject per-instance `context`) × `workerd`/V8's native receiver/brand check on `fetch()` (standard JS platform behavior, not a bug) | `panic: JavaScript error: Illegal invocation: function called with incorrect \`this\` reference`                                                         | **Yes, at the library level** — see the `wasm_exec.js` patch below          |
| 4   | A naive "bind every function retrieved off the proxy" attempt at fixing #3                       | Same Proxy, but the fix itself: `Function.prototype.bind()` does not forward a function's own properties, so e.g. `Array.bind(target).from` is `undefined`                                                                                                                          | `TypeError: Cannot read properties of undefined (reading 'exports')` (cascades from a WASM VM-level failure once a bound class loses its static methods) | Yes — narrow the bind to `fetch` only, verified working with no regressions |

**Corrected understanding of #2, the practical scope is much
narrower than "any reused-instance timer wait fails":**

- Outbound blocking I/O (reading a live outbound HTTP response body,
  the shape a real watch client uses) works fine on a reused instance,
  **including as a non-originating request**, as long as _this_
  request itself stays open for the duration — verified with a
  synchronous outbound read against a separate worker process: 3 real
  heartbeat lines, 6.05s elapsed, exactly matching the source's 2s
  cadence, both as request #1 and as request #2. The failure in #2 is
  specific to Go's _shared internal timer scheduler_
  (`time.Sleep`/`time.Ticker`/`time.After`), not to blocking I/O in
  general.
- Whenever _anything_ is actively keeping the scheduler pumped — an
  open inbound stream, a blocked outbound read, or (see below) an
  active `cloudflare.WaitUntil` task — **every other goroutine's
  timers also make correct real-time progress**, not just the one
  doing the active work. Confirmed by watching `main_ticker` (a plain,
  unwrapped background ticker) advance correctly in lockstep while a
  _different_ goroutine was blocked on an outbound read.
- The failure only actually occurs when a _new_ timer-blocking wait is
  started and **nothing at all** is concurrently active (no stream, no
  blocked read, no waitUntil task) — confirmed as a clean, isolated
  negative case: with zero other traffic, `/stream` dispatched as a
  reused instance's 2nd request still failed immediately even after
  the above was understood, ruling out "maybe it always secretly
  works now."
- Self-referential outbound calls (a Go program calling _its own_
  route via HTTP loopback, re-entering `binding.handleRequest`)
  inherit this same failure if the inner route itself needs a fresh
  timer and the instance has already served prior requests — the
  inner call is dispatched exactly like any other new request. Don't
  have a resident worker call its own timer-blocking routes via
  self-loopback; call a genuinely separate process/worker, or avoid
  self-loopback for such routes.

**`wasm_exec.js` fetch fix, verified working.** Root cause of failure #3
above is specific and fixable: `wasm_exec.js`'s `Proxy` `get` trap
returns raw (unbound) function references, and `fetch()` rejects being
invoked with a `Proxy` as `this`. Patch (in
`spikes/s8-wasm-resident/vendor/syumai-workers-fork/cmd/workers-assets-gen/assets/wasm_exec_go.js`,
the source `workers-assets-gen` copies into every project's
`wasm_exec.js`):

```js
get(target, prop) {
  if (prop === 'context') { return context; }
  const val = Reflect.get(target, prop, target);
  if (prop === 'fetch' && typeof val === 'function') {
    return val.bind(target);   // fetch specifically; NOT every function -- see failure #4
  }
  return val;
}
```

Regenerated `wasm_exec.js` from this patched fork and re-ran, with **no
service-binding workaround at all**: a plain `http.Get()` to this same
worker's own loopback route returned a clean 200, and a plain
`http.Get("https://example.com/")` to a genuinely external URL also
returned a clean 200 with real HTML content. Existing behavior
unaffected (streaming, instance reuse, `/status` all still worked
identically after the patch). **This is a ~10-line, one-file fix that
would let every outbound call client-go/KCM makes internally work
unmodified**, instead of requiring an audit-and-rewrite of every call
site to use explicit service bindings. This substantially de-risks the
"any upstream code that calls `http.Get` will crash the instance"
open question from the original S8 write-up above. Not yet verified
against production `workerd`, and not yet re-verified against the real,
much larger apiserver/controllers-shaped binary (only tested against
this spike's minimal Go programs).

**WASM instantiation "startup tax", measured** (assumes module bytes
already compiled/cached, which is how this library always works — only
the `WebAssembly.Instance` + Go runtime bootstrap + all package `init()`s

- `main()` up to first byte is being timed here, via `curl -w
"%{time_starttransfer}"`, 10 samples each, local `wrangler dev`):

| Target                                                                                                               | Pattern                                                                                              | `time_starttransfer`           |
| -------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | ------------------------------ |
| This spike's minimal `stock` binary (~5MB uncompressed)                                                              | fresh instantiate every request                                                                      | ~4.2–5.5ms (avg ~4.6ms)        |
| This spike's minimal `resident` binary, already warm                                                                 | dispatch only, no instantiate                                                                        | ~1.8–3.5ms (avg ~2.3ms)        |
| **The real k8flare apiserver** (`packages/worker/build/app.wasm`, 38MB raw / 7.07MB gzip, current build, unmodified) | fresh instantiate every request (`GET /api`, no-auth discovery endpoint, current production pattern) | **~13.6–17.2ms (avg ~14.4ms)** |

Delta between minimal-fresh and minimal-warm (~2.3ms) is the pure
instantiate+bootstrap tax for a _trivial_ program. The real
apiserver's fresh-instantiate cost (~14.4ms) is markedly higher,
confirming the tax scales with program complexity/size, not a fixed
constant — expected, since a larger binary means more package-level
`init()`s (more resource stores, more registered routes) to run
before the first request can be served. **Even so, ~14ms is fast in
absolute terms** — far below human-perceptible latency, and orders of
magnitude below Cloudflare Containers' typical 1–3s cold start (see S3
section). This is a meaningful, favorable data point for any
"event-driven re-entrant" execution shape (fresh instantiate per
wake-event: a DO alarm firing, a WebSocket hibernation wake) — it
would not need to keep an instance "resident" at all to stay fast, at
least not for apiserver-sized binaries. **Caveat**: a real
`workers/controllers` binary (embedding kube-scheduler/KCM and much of
client-go) is likely to be substantially larger/more complex than
apiserver, so its own instantiate tax could be higher still — this
needs measuring against an actual controllers-shaped binary once one
exists, not extrapolated from apiserver's number. Also: local
`wrangler dev` only; production instantiate cost (isolate
pooling/warm-start optimizations Cloudflare may apply) is unverified.

**Global state (Go package vars) persistence conditions, precisely
stated:**

- **Persists** across every request dispatched into the same reused
  `WebAssembly.Instance` (the S5 fork's model) — confirmed via a request
  counter and instance ID staying constant and a ticker's count
  correctly accumulating across many separate `curl` invocations,
  minutes apart.
- **Resets to zero** the instant a _new_ `WebAssembly.Instance` is
  created — this is what happens on every request under the _current,
  unmodified_ generated glue (one instance per request, confirmed via a
  different random instance ID on every call).
- **Is destroyed entirely** if the reused instance's Go program panics
  unrecovered (fatal to the whole program, same as any Go program) —
  demonstrated twice, by the pre-fix `doneCh` double-close and
  separately by the pre-patch outbound-fetch crash; every subsequent
  request to that isolate then fails with `"Error: Go program has
already exited"` until the dev server itself reloads.
  **Practical implication for a reused-instance design: every goroutine
  must be wrapped in `recover()`, since one unrecovered panic anywhere
  takes down all accumulated state for every tenant/request sharing that
  instance, not just the request that triggered it.**
- **Not observed to expire from mere idleness** in local `wrangler dev`
  — an instance that received zero requests for almost a minute still
  answered correctly with all prior state intact when the next request
  arrived. This is very likely a `wrangler dev` characteristic (a single
  long-running local process, not realistic isolate lifecycle
  management) rather than a property of production `workerd`, which is
  expected to evict idle isolates on its own schedule — **not
  verified against production**, listed as an open question above.

**`ctx.waitUntil` life-extension past the visible response closing —
the single most useful new result for shape selection.** Tested whether
wrapping a background goroutine in `cloudflare.WaitUntil` lets it keep
making real, wall-clock progress with **zero other requests open**,
which earlier testing had shown a plain (unwrapped) background ticker
cannot do (it freezes, advancing at most +1 tick per later incoming
request, never proportionally to elapsed time). Handler
(`/close-then-extend`) returns its own short response immediately
(closes in ~40ms) but, before returning, registers via
`cloudflare.WaitUntil` a goroutine that ticks a counter once per second.
Polled `/status` afterwards with **no other traffic in between**:

```
after ~45.5s idle:  extend_ticker=45   main_ticker=22   (both ≈ real elapsed time)
after ~41.9s idle:  extend_ticker=41   main_ticker=20   (both ≈ real elapsed time)
```

**Confirmed: yes, `ctx.waitUntil` keeps the goroutine progressing in real
time with the visible response fully closed and no client connected.**
Re-run with a 600s (10 min) budget and polled at increasing intervals
with zero other traffic in between:

```
uptime= 45.5s: extend_ticker= 45  main_ticker=22
uptime=105.5s: extend_ticker= 92  main_ticker=47
uptime=150.8s: extend_ticker=137  main_ticker=70
uptime=255.2s: extend_ticker=241  main_ticker=122
```

Both counters track real elapsed time closely the entire way out to
**4+ minutes (255s)**, no slowing, no cap hit. (Numbers come from two
consecutive runs on the same instance — the first was accidentally
interrupted at ~50s by an unrelated test that had to restart the dev
server; the counters above are the second, uninterrupted run. Raw logs:
`spikes/s8-wasm-resident/logs/resident-dev-14.log`,
`resident-dev-15.log`.) And — matching the "anything active pumps everything" pattern found for
failure #2 above — the _plain, unwrapped_ `main_ticker` **also** tracked
real elapsed time correctly during this same window, purely because the
waitUntil task was active; with no waitUntil task running (all earlier
tests), the identical `main_ticker` code only ever got +1 tick per
incoming request while idle. **This means a single `cloudflare.WaitUntil`
call is sufficient to keep the _entire_ shared scheduler pumped for
every goroutine on that instance, not just the wrapped one.**
Practical implication: **"return an ack immediately, then run the real
reconcile/watch loop via one `cloudflare.WaitUntil` call" is a
credible, simpler alternative to "keep a client-visible stream open"**
for a resident-style execution shape — it doesn't require a connected
client at all. Caveats, all unverified: (1) local `wrangler dev` only —
production may cap `waitUntil` duration or bill it differently than
streaming CPU time (the pricing model referenced earlier for streaming
responses was not confirmed to also apply to `waitUntil`); (2) not
tested past ~45s of continuous observation in this pass; (3) whether
`waitUntil`'s extension is itself vulnerable to the same "only works if
it's the originating/currently-valid IoContext" constraint when called
from a _non-originating_ request was separately tested and did **not**
throw (S5 section, point 4) but that result is flagged there as not
fully understood.

**Candidate WASM execution shapes, informed by all of the above** (for
Phase 5 design, not a recommendation to pick one yet):

1. **Stream-resident** (this spike's (a)): one instance per logical
   "session," keeps a client-visible response stream open for its whole
   life. Proven to work cleanly for 11+ minutes. Needs a client willing
   to hold a connection open.
2. **WaitUntil-resident** (new, from the finding above): one instance
   answers a triggering request immediately, then keeps running via a
   self-perpetuating `cloudflare.WaitUntil` task. No open client
   connection required; same underlying scheduler-pumping mechanism as
   (1). Needs production verification of duration limits/billing.
3. **Event-driven re-entrant** (fresh instantiate per wake, no
   residency at all): viable if per-wake instantiate tax stays low
   enough — apiserver-scale measured at ~14ms locally, likely higher for
   a controllers-scale binary, not yet measured. Sidesteps every
   cross-request scheduling problem in this document entirely, at the
   cost of losing in-memory state between wakes (would need to
   externalize informer caches/resourceVersions, e.g. to DO storage).
4. **DO-hosted execution**: running the WASM instantiate/dispatch inside
   a Durable Object's own methods instead of a Worker `fetch()` handler,
   to combine DO's persistent in-memory object lifetime with
   alarm/hibernation-driven wake-ups. **Minimal feasibility confirmed**
   (`spikes/s8-wasm-resident/do-hosted/`, a `WasmDO` class with the same
   instantiate-once-then-reuse pattern as the plain-Worker `resident`
   spike, moved to DO instance fields instead of module-scope `let`
   vars):
   - Instantiating and dispatching into the Go program from inside the
     DO's own `fetch(request)` method works cleanly; state (instance ID,
     request counter) persists across separate requests routed to the
     same DO id, exactly like the plain-Worker case.
   - **Dispatching from the DO's `alarm()` handler also works**: armed a
     one-shot alarm (`storage.setAlarm(Date.now()+2000)`, fired once, not
     re-armed — event-armed per the cost invariants, not polling), and
     when it fired, the `alarm()` handler successfully called into the
     _same_ already-running Go instance (`binding.handleRequest`) and
     got back correct state (matching instance ID, correctly incremented
     request counter, `main_ticker` having advanced proportionally to
     real elapsed time since the instance was created). This is the
     concrete mechanism a "wake via alarm, reconcile, sleep" execution
     shape would depend on, and it works in local `wrangler dev`.
   - Not yet tested: whether the same timer-scheduling constraints found
     in the plain-Worker case (a _new_ blocking timer wait failing when
     nothing else is active) also apply to alarm-triggered dispatch —
     expected to, by the same underlying `wasm_exec.js` mechanism, but
     not independently re-verified here. Also not tested: WebSocket
     hibernation wake-up specifically, or production DO behavior.

### Production verification (2026-07-02 follow-up, real Workers deployment)

Deployed `k8flare-verify-s8-resident` (the DO-hosted variant with the
fetch-bind glue patch) to the KOOFFICE account, ran the tests, then deleted
the Worker (delete confirmed via API error 10007 and a 404 on the URL; no
secrets used). Code: `spikes/s8-wasm-resident/prod-resident/`; logs:
`spikes/s8-wasm-resident/logs/prod-resident/`.

- **`ctx.waitUntil` in production far exceeds the commonly assumed ~30 s
  cap.** Two independent trials (separate DO ids): a background goroutine
  wrapped in `WaitUntil` kept tracking real wall-clock time (±1 s over 26
  samples polled every 15 s with zero other traffic) out to **332 s
  (5 m 32 s)** in trial 1 and 117 s in trial 2 — both stopped deliberately
  by the operator, not by the platform. This is a **confirmed lower bound
  of ~5.5 minutes, not the actual cap**; neither trial was observed to
  die. Design takeaway: multi-minute background reconcile work via
  waitUntil is real in production; the exact ceiling and death mode
  remain unmeasured.
- **The wasm_exec.js fetch-bind patch works in production**: plain
  `http.Get` from Go succeeded against both a self-loopback URL and a
  genuinely external URL (`https://example.com`, HTTP 200 with real
  HTML), with no service-binding workaround.
- **DO-hosted instantiate-once reconfirmed in production**: consistent
  instance ID and a monotonically increasing request counter across
  dozens of requests over 5+ minutes.
- **Unplanned operational finding: redeploying a new script version
  resets already-running DO instances.** After trial 1 had run >5 minutes,
  a redeploy changed the DO's instance ID and reset its uptime on the
  next request. Any resident-controllers design must assume the instance
  can vanish at any deploy (in addition to panics and eventual eviction):
  keep critical state in DO storage, make reconcile loops idempotent and
  resumable, and use an event-armed `alarm()` safety net to self-heal.
- **Resulting recommendation for Phase 5** (recorded here, decision made
  at design time): DO-hosted + WaitUntil-resident with an event-armed
  `alarm()` safety net, built so any wake (fetch, alarm) can resume from
  durable state; event-driven re-entrant (no residency) remains the
  simpler fallback.

### S8(b): production CPU-ms of an idle open stream (2026-07-02, measured)

Deployed the unmodified `stock` variant as `k8flare-verify-s8` (KOOFFICE
account; deleted afterwards, deletion double-confirmed via error 10007 and a
404). Code: `spikes/s8-wasm-resident/prod/`; raw tail JSON:
`spikes/s8-wasm-resident/logs/prod-cpu/`.

- Measurement channel: `wrangler tail --format json` reports `cpuTime` and
  `wallTime` per completed request — no GraphQL fallback needed.
  **Methodology pitfall**: a naive long-lived tail misses the completion
  event of a long stream. Working pattern (used for both runs): open the
  tail session BEFORE the request and keep it warm with a lightweight
  `/status` ping every ~60 s.
- Baseline `/status` ×5: 67, 83, 12, 10, 23 ms CPU (first two cold; wallTime
  ≈ cpuTime confirms no I/O wait).
- **10-minute open stream (2 s heartbeats + 5 s informer-sim goroutine):
  246 ms CPU over 599,959 ms wall.** → ~1,476 ms CPU/hour → ~1.06M
  CPU-ms/month.
- **30-minute open stream: 912 ms CPU over 1,799,964 ms wall.** → ~1,824 ms
  CPU/hour → ~1.31M CPU-ms/month. Roughly linear with the 10-minute run
  (0.4–0.5 ms CPU per wall-second); the 24% deviation from a naive 3×
  extrapolation is within GC/scheduler jitter at these tiny absolute values
  (only two data points — not claiming a strict law).
- **Cost conclusion** (Workers Paid: $5/mo incl. 30M CPU-ms, $0.02 per
  extra 1M CPU-ms): an idle stream-resident controllers session costs
  **~$0.02–0.03/month per cluster** at overage rates, and the plan's
  included allotment alone covers **~23–28 idle clusters**. This empirically
  validates cost invariant #2: CPU-time-billed residency is compatible with
  the scale-to-zero concept. (The DO-side anchor duration cost remains a
  separate design consideration — see the Phase 5 recommendation above.)

---

## Phase 4 implementation findings (2026-07-02, local wrangler dev)

Discovered while implementing storage v2 (facets + WatchHub) — recorded here
by the coordinating session because the implementing agent's scope excluded
this file. Reproductions live in the Phase 4 branch's code comments
(`workers/storage/src/index.ts`) and `docs/multi-tenancy-and-hosting.md`.

1. **A WebSocket obtained from another DO's `fetch()` response cannot be
   adopted with `ctx.acceptWebSocket()`** — the platform rejects it, so a
   "WatchHub holds one upstream WS to Cluster" relay design is impossible.
   WatchHub was redesigned: Cluster POSTs events to WatchHub (`/push`), and
   WatchHub fans out to clients over hibernatable WebSockets.
2. **Facet names cannot be safely reused across repeated `delete()` /
   recreate cycles**: from the 4th cycle the facet deterministically enters
   a contradictory state (reads 404 while creates 409 "already exists").
   One delete/recreate cycle works (as S1 verified); repetition breaks.
   Consequence: namespace deletion tombstones objects via cascade delete but
   does NOT `delete()` the facet; facet GC needs a different design (e.g.
   UID-suffixed facet names).
3. **Open issue (production check required): Go `net/http` clients receive
   zero bytes from streaming responses under local wrangler dev**, while
   curl on the same endpoint streams fine. Reproduced with a minimal Go
   program (not client-go-specific). Until verified against production
   Workers, kubectl/client-go watch through the new WatchHub path must be
   treated as unconfirmed. (Untested hypothesis worth trying first:
   Go's default `Accept-Encoding: gzip` causing a buffering compression
   layer in the dev proxy — curl sends no such header.)

## Correction log (honest corrections)

**2026-07-03 — Go net/http streaming block against local wrangler dev was
a client/dev-stack Accept-Encoding interaction, not a platform-wide bug;
confirmed fixed locally and confirmed absent in production.** The Phase 4
finding above (item 3, "Open issue: Go net/http clients receive zero bytes")
left open whether this reproduced in production. It does not.

Root cause, isolated by varying one client setting at a time against local
wrangler dev: Go's `Transport` sends `Accept-Encoding: gzip` unless the
caller overrides it, and something in the local wrangler dev stack honors
that by gzip-compressing the chunked watch response without flushing per
write — curl doesn't request gzip by default, so it always saw bytes
immediately, which made this look client-go-specific rather than
encoding-negotiation-specific. Confirmed both directions: `curl -H
'Accept-Encoding: gzip'` against local dev reproduces the same indefinite
hang; `Transport.DisableCompression = true` or an explicit `Accept-Encoding:
identity` request header both make the default Go client receive the first
line in single-digit milliseconds. Forcing HTTP/1.1 made no difference,
ruling out an h2-framing explanation.

Fix (`packages/k8s/src/watch.ts`, Phase 4 branch commit `8040dd1`): send an
explicit `Content-Encoding: identity` response header on the watch
endpoint. The existing `Cache-Control: no-cache, no-transform` header (what
a real apiserver relies on to stop transforming proxies from compressing
watch responses) is not honored by whatever does this locally, but the
explicit `Content-Encoding` is. Verified end to end with a real client-go
`watch.Interface` (`pkg/apiserver/apiserver_test.go`'s new `TestPodWatch`):
Added/Modified/Deleted all delivered promptly with the default
client-go/net/http transport; `go test ./pkg/apiserver/...` green.

**Production verification** (real Workers deployment, `k8flare-verify-stream`,
deleted after use — confirmed via API error 10007 and a 404 on the URL): the
local-dev hang does not reproduce. A copy of the S8 spike's unmodified
`/stream` handler (`spikes/watch-stream-verify/`) was deployed to the
KOOFFICE account and driven with the same Go client varied the same way.
All variants — default Go client (`Accept-Encoding: gzip` negotiated,
`resp.Uncompressed=true` confirming the response actually was
gzip-compressed), `DisableCompression`, and explicit `Accept-Encoding:
identity` — received the first chunk in ~2.1–2.3s (matching the handler's 2s
ticker) and continued receiving every subsequent tick over an 11s
multi-line read with no stalls.

**Conclusion**: this was a local-wrangler-dev-only limitation (its
compression layer appears not to flush gzip output per chunk on a
long-lived stream); production's real compressor flushes per chunk, so a
plain Go `net/http` client — and therefore client-go, kubectl, kubelet,
every controller — was never actually blocked in production, even before
this fix. The `Content-Encoding: identity` fix is still worth keeping (it
matches real apiserver behavior, and local dev usability depends on it),
but it was not the production-functionality blocker the original finding
implied.

**Side finding, not pursued** (flagged for whoever next works on
kubectl-based local dev usability): the system `kubectl` binary does not
send an `Authorization` header at all against a plain `http://` (non-TLS)
target — confirmed by pointing it at a throwaway Go server dumping every
received header, both via kubeconfig `user.token` and via
`--token`/`--insecure-skip-tls-verify` flags directly. Separate from the
streaming bug above (fails at the auth step, before any watch/streaming
logic runs); doesn't affect `apiserver_test.go`'s client-go-based tests,
which construct `rest.Config{BearerToken: ...}` directly. Real
kubectl-driven local testing will likely need wrangler dev to terminate TLS
(`--local-protocol https`).

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

**2026-07-02 — S6 research-time correction on Containers egress
defaults.** While researching S3 access from Containers for the S6
section above, an initial LLM-summarized read of `cloudflare/containers`'
`docs/egress.md` claimed internet access from a Container is _blocked_
by default; a second summarized source claimed the opposite. Rather than
picking one, the research (`spikes/s6-r2/RESEARCH.md` §2) went to
primary sources: the raw markdown of Cloudflare's official
outbound-traffic docs, and the actual `@cloudflare/containers` SDK
source on GitHub (`enableInternet: ... = true`). Both agree: **internet
access is on by default**. This never appeared as a "confirmed fact" in
this document before now, so it isn't a correction of prior published
text — it's recorded here as an example of the "verify, don't trust a
read" principle (CLAUDE.md inviolable rule #2) catching a wrong
intermediate answer before it became a documented fact.

**2026-07-02 — S8's "(a) requires a fork" claim was wrong; corrected
after actually running it.** The S8 section previously stated, under
"Confirmed facts," that "keeping a Go program alive as a response stream
that's never closed requires a fork (continuous with S5's isolate-reuse
fork)." A live local test (`spikes/s8-wasm-resident/stock/`, completely
unmodified `github.com/syumai/workers` v0.32.0, standard generated glue)
kept a single request's response stream open for a full 11 minutes with
background goroutines running the whole time, with **zero library
changes**. The earlier claim conflated two different things: (1) keeping
_one already-in-flight request's_ Go program alive indefinitely (no fork
needed — `Serve()`'s blocking `<-Done()` only depends on that one
request's own response body, so a handler that never returns and a
client that keeps reading is sufficient), and (2) reusing _one instance
across multiple, independent_ incoming requests (S5's actual goal, which
does need the `doneCh` fork — confirmed separately). This document's
prior text was written before either was run; both are now verified
against real `wrangler dev` output, not just source reading. The S8
section above has been updated in place to state the distinction
directly; this entry preserves the original (incorrect) claim and why it
was wrong, per CLAUDE.md inviolable rule #4.

**2026-07-03 — Phase 5 implementation: two hard blockers S8's toy spike
could not have caught, found only by actually building the real
kube-scheduler/kube-controller-manager for GOOS=js/wasm.** S8 (above)
verified the _execution shape_ (stream/DO-hosted residency, WaitUntil,
outbound fetch) using minimal, hand-written Go programs — it never
imported the real `k8s.io/kubernetes/cmd/kube-scheduler` or
`cmd/kube-controller-manager` package trees. Phase 5's implementation
attempt did, and found two blockers the shape-level spike had no way to
surface. Per CLAUDE.md rule #2, both are backed by actual `GOOS=js
GOARCH=wasm go build` runs, not source reading alone; per rule #4, this
is recorded rather than silently narrowing Phase 5's scope.

1. **kube-scheduler cannot compile for GOOS=js/wasm at all — not a
   library-level issue, unfixable without vendoring a patched fork of all
   of k8s.io/kubernetes.** `k8s.io/kubernetes/pkg/scheduler/scheduler.go`
   unconditionally imports
   `pkg/scheduler/backend/cache/debugger`(`cachedebugger.New(...)` /
   `debugger.ListenForSignal(ctx)`, used for a SIGUSR2-triggered cache
   debug dump). That package's `signal.go` is `//go:build !windows` and
   references `syscall.SIGUSR2`, which does not exist for GOOS=js;
   `signal_windows.go` shows the fix is trivial in isolation (`var
compareSignal = os.Interrupt`, a portable `os.Signal`) but there is no
   GOOS=js variant anywhere in the tree. Because this is a plain
   subpackage of the `k8s.io/kubernetes` main module (not a separately
   replaceable staging module like `k8s.io/mount-utils`), fixing it would
   require `replace k8s.io/kubernetes => <local fork>` pointing at a
   _complete_ local copy of the module (Go module replace has no
   file-level overlay mechanism) — concretely, `du -sh` on this repo's
   pinned `github.com/k3s-io/kubernetes@v1.36.2-k3s1` module cache entry
   is **107MB across 5,231 `.go` files**. Vendoring that into this
   repository to patch one `var` declaration is a different order of
   commitment than `third_party/syumai-workers-fork/` (a few hundred KB,
   one small upstream library, two changed files, see the fork-adoption
   commit) and was not attempted here; it would also need re-syncing by
   hand on every future k8s version bump (`docs/k8s-version-bump.md`
   currently assumes a version-pin-only process). **Consequence: `cmd/scheduler`
   is not, and cannot currently be, part of `workers/controllers`.** It
   remains exactly as it was — an unmodified binary that only runs as a
   host process (BYO VM, or as `.github/workflows/e2e-conformance.yml`
   already does today).

2. **kube-controller-manager's individual controller packages _do_
   compile for GOOS=js/wasm (verified, see below) — but `k8s.io/client-go`'s
   generated typed clientset + informers alone already exceed Cloudflare
   Workers' 10MiB gzip deployment limit, before any controller logic is
   added.** Isolated, incremental `GOOS=js GOARCH=wasm go build` +
   `gzip | wc -c` measurements (`workers/apiserver/build/app.wasm`, this
   repo's only other real data point, is 7.07MiB gzip for comparison):

   | Build content                                                                                              | gzip size                              |
   | ---------------------------------------------------------------------------------------------------------- | -------------------------------------- |
   | Empty `main()` + `github.com/syumai/workers` only                                                          | 1.58 MiB                               |
   | `k8s.io/client-go/kubernetes` (typed Clientset) + `k8s.io/client-go/informers` only, zero controller logic | 8.87 MiB                               |
   | + one controller (`pkg/controller/tainteviction`, the smallest of the ten)                                 | 15.03 MiB                              |
   | + all 5 controllers this phase adds (endpoint, endpointslice, nodeipam, nodelifecycle, tainteviction)      | 15.06 MiB                              |
   | + all 10 controllers (`cmd/controller-manager`'s full `--controllers` list)                                | **18.98 MiB raw (126MB uncompressed)** |

   The jump is almost entirely in the client-go baseline (1.58→8.87MiB),
   not per-controller cost (8.87→15.06MiB for five real controllers,
   →18.98MiB for all ten) — this is a property of needing _any_ typed,
   generated Kubernetes client at all, not of how many controllers use it.
   Every real upstream controller constructor in this codebase requires a
   `clientset.Interface` and typed `SomeKindInformer` parameters, so there
   is no way to use the real controllers without paying this cost.
   `-ldflags="-s -w"` (strip debug info) saves under 5% (18.98→18.29MiB) —
   nowhere near enough. **Consequence: `workers/controllers`, as currently
   scoped (all ten controllers in one Go WASM binary), cannot pass
   `wrangler deploy`'s size check.** This was not attempted against a real
   deploy in this pass (out of context budget) — the gzip byte count
   against the same 10MiB ceiling `workers/apiserver` is already measured
   against is conclusive enough on its own not to need that confirmation
   to act on.

   **What _is_ confirmed working, mechanically and functionally, despite
   the deploy-time size blocker**: individually, every one of the ten
   controller packages (`pkg/controller/{replicaset,deployment,daemon,job,
cronjob,endpoint,endpointslice,nodeipam,nodelifecycle,tainteviction}`)
   _compiles cleanly_ for GOOS=js/wasm — confirmed by isolating each
   failure from the naive first attempt (importing the top-level
   `cmd/kube-controller-manager/app` package, which unconditionally
   references _every_ controller including ones this repo never enables,
   several of which pull in `k8s.io/mount-utils`,
   `k8s.io/kubernetes/pkg/probe`, `pkg/securitycontext`, and
   `pkg/util/filesystem` — all Linux/Windows-only, no GOOS=js variant,
   same class of problem as the scheduler's debugger package, just in
   controllers this repo doesn't use) down to importing each of the ten
   real controller packages directly, bypassing `app`'s
   `NewControllerDescriptors()`/`ControllerContext`/`Run()` orchestration
   entirely. `pkg/controllers/controllermanager.go`
   (`RunControllerManager`) hand-wires all ten against a plain
   `k8s.io/client-go/informers.SharedInformerFactory` (the same type
   `ControllerContext.InformerFactory` is itself declared as) and a
   `*rest.Config` built in-memory with a `Transport` routed through a
   Cloudflare service binding (`pkg/controllers/restconfig.go`,
   `cloudflare.GetBinding` + `cloudflare/fetch.NewClient(WithBinding(...))`
   — the S8-verified binding-routed outbound pattern) instead of a file-
   based kubeconfig (client-go's `BuildConfigFromFlags`, read directly:
   even with a bare master-URL override that avoids the filesystem, it
   never attaches a Bearer token — confirmed by reading
   `client-go/tools/clientcmd/client_config.go`, not assumed).

   Deployed via `wrangler dev` (does not enforce the production size
   cap) with the DO-hosted pattern (`workers/controllers/src/index.ts`,
   same instantiate-once-per-DO-instance shape as
   `spikes/s8-wasm-resident/do-hosted`), against the real
   `workers/gateway`→`workers/apiserver`→`workers/storage` stack, all
   five local, over a `services` binding named `GATEWAY` (not `APISERVER`
   directly — `workers/gateway`'s own `index.ts` intercepts `?watch=true`
   requests and serves them from `@k8flare/k8s`'s `handleWatch` in TS,
   never forwarding them to the Go apiserver Worker at all, so a client
   that needs working watches — which client-go's informers absolutely
   do — has to go through gateway, not around it):
   - All ten controllers logged `"Starting ..."` and
     `"Caches are synced"` within ~130ms of the resident program starting.
   - **nodeipam: real end-to-end PodCIDR allocation.** Creating a Node
     produced a log line from the actual upstream
     `range_allocator.go:433 "Set node PodCIDR" node="test-node-1"
podCIDRs=["10.42.0.0/24"]` and the Node object was correctly patched
     — not a simulation, the genuine `pkg/controller/nodeipam` allocator
     logic running against this repo's storage through the full
     gateway→apiserver→Cluster DO chain.
   - **nodelifecycle + taint-eviction-controller: real end-to-end
     staleness detection**, re-verified in the same shape as the original
     kill-the-agent check (`8a9d92c`, done against the old TS
     `nodelifecycle.ts`) but via API-level Lease-staleness simulation
     instead of an actual killed agent process (this session's sandbox is
     macOS; `cmd/agent` cannot run natively — `pkg/cgroups` build
     constraints exclude non-Linux — so a real k3s agent could not be
     started to kill, unlike the original check's environment). A Node
     with a Lease last renewed at t+0, never renewed again: Ready flipped
     True→Unknown and the `node.kubernetes.io/unreachable` NoExecute taint
     was applied between **t+31s and t+36s** (task success bar: ≤100s —
     comfortably inside it, and via the real upstream
     `node_lifecycle_controller.go`, not the deleted TS version's
     hand-rolled 40s-grace-period check — the real default
     `--node-monitor-grace-period` measured directly off a running
     `cmd/controller-manager -v=2` is **50s**, not the 40s the old
     `nodelifecycle.ts` comment claimed matched "upstream's default"; this
     was never corrected before because the TS version hardcoded its own
     40s rather than reading the real default). **Eviction did not
     reproduce**: a Pod bound to that node was still present (HTTP 200 on
     a direct GET) at **t+434s (~7.2 minutes)**, well past the ≥5m
     success bar the deleted TS version's hand-rolled eviction met, when
     observation was stopped (context budget, not a deliberate cutoff).
     `taint_eviction.go` logged `"Starting"` and `"Sending events to API
server"` at controller startup and nothing else for the rest of the
     run — no sync/eviction-attempt log lines at all around t+36s when the
     taint was actually applied, despite `pkg/controller/nodelifecycle`
     (sharing the exact same `informers.SharedInformerFactory`-provided
     Node informer instance) correctly observing and reacting to the same
     Node object. Not root-caused in this pass: plausible causes include a
     `tainteviction.New` wiring gap specific to this hand-wired
     (non-`ControllerContext`) construction path, an event-handler
     registration ordering issue, or something specific to running two
     controllers that both watch Nodes against one shared informer in this
     environment — genuinely unknown, not guessed at further here per
     CLAUDE.md rule #2. **This is a confirmed, currently-unresolved
     functional gap** in `pkg/controllers/controllermanager.go`'s
     taint-eviction wiring, on top of (independent from) the size blocker
     above — flagged for whoever picks this up next, before relying on
     automatic Pod eviction from this execution path.
   - **endpoint/endpointslice: controllers confirmed running and reacting
     to real writes**, partially verified. The real
     `endpoints_controller.go`/`endpointslice_controller.go` create
     placeholder `Endpoints`/`EndpointSlice` objects immediately on Service
     creation (labels `endpoints.kubernetes.io/managed-by:
endpoint-controller` / `endpointslice.kubernetes.io/managed-by:
endpointslice-controller.k8s.io` confirm this is the real controller,
     not a stub), and correctly _rejected_ a malformed test Pod lacking
     `spec.nodeName` with a genuine upstream validation error (`"skipping
Pod test-svc-pod for Service default/test-svc: Node  Not Found"`) —
     proving the real controller logic is executing, not a no-op. Getting
     the addresses to actually populate after fixing the test Pod's
     `nodeName` was not achieved within this pass's remaining time (the
     manual test sequence involved several overlapping PUTs and
     resourceVersion churn that likely raced with the informer's resync,
     rather than a defect in the controller wiring itself, but this was
     not root-caused) — worth a clean re-test (create Service, then create
     an already-fully-formed Pod with `nodeName` + `status.podIP` +
     `status.conditions` in one shot, rather than this session's
     incremental PUT sequence) before relying on this path.

   **Net position for Phase 5**: the DO-hosted + WaitUntil-resident +
   event-armed-alarm execution shape S8 recommended is confirmed correct
   and working end-to-end for kube-controller-manager specifically (real
   upstream controller code, real client-go informers over a real
   Cloudflare service binding, real reconciliation against real storage)
   — the remaining blocker is purely the compiled artifact's _deployed_
   size, not the execution model. `workers/controllers` as implemented in
   this pass is verified-correct-but-not-yet-deployable. Candidate next
   steps (not attempted, listed for whoever picks this up): split the ten
   controllers across two or more separate WASM Workers, each under
   budget (mirrors this project's existing per-component Worker-splitting
   strategy for exactly this kind of size pressure — see the v2 rewrite
   plan's "デプロイサイズ制限からの脱出"); investigate whether a
   hand-written, narrower REST client (satisfying just the specific
   `clientset.Interface` methods each controller actually calls, instead
   of the fully generated `k8s.io/client-go/kubernetes.Clientset`) can
   shrink the ~8.87MiB baseline meaningfully — unexplored, and a
   significant undertaking; or accept kube-controller-manager as
   host-process/BYO-VM-only alongside kube-scheduler, the same way this
   finding already settles kube-scheduler, and treat
   `workers/controllers` as not viable for either binary until one of the
   above changes the size math.

**2026-07-03 — Lean client-go client investigated for KCM's size blocker:
real but small win (1.2 MiB), does not come close to fitting.** Per the
user's 2026-07-03 decision recorded above ("軽量クライアントを調査"), a narrow
hand-written client satisfying only what the five in-scope controllers
(nodeipam, nodelifecycle, taint-eviction-controller, endpoint,
endpointslice) actually call was designed, built, and measured
(`spikes/leanclient-kcm/leanclient/` — kept as investigation evidence, not
wired into `pkg/controllers/controllermanager.go`). Every number below is
from a real `GOOS=js GOARCH=wasm go build` + `gzip | wc -c` run this same
session (Go 1.26.2), per CLAUDE.md rule 2.

Before writing any hand client, a code-generation alternative the
coordinator proposed (re-running `k8s.io/code-generator`'s `client-gen`
scoped to fewer API groups, on the theory that it's the same generator
`k8s.io/client-go` itself was built with) was actually tried:
`go run k8s.io/code-generator/cmd/client-gen --input-base k8s.io/api --input
discovery/v1 --output-pkg .../gencheck/clientset --clientset-name versioned`
ran successfully and its output was inspected directly. It does not help:
the generator's own template
(`cmd/client-gen/generators/generator_for_clientset.go`) always emits a
**fresh** `type Interface interface {...}` in the output package, scoped
only to the groups given via `--input` — confirmed by the real run, whose
generated `Interface` was `{ Discovery(); DiscoveryV1() }`, a brand-new type
in a brand-new package. This can never substitute for the specific
`k8s.io/client-go/kubernetes.Interface` (54 methods) that all five
controllers' constructors, `k8s.io/component-helpers/node/util.PatchNodeCIDRs`
(which `nodeipam` calls), and every `informers/<group>/<version>.NewXInformer`
hardcode as their parameter type — Go requires the concrete type to
implement all 54 of that exact interface's methods, not a
structurally-similar smaller one from a different package. The only way to
make generated code literally _be_ `kubernetes.Interface` would be `replace
k8s.io/client-go => <local fork>` in this repo's single root `go.mod` —
module-wide, breaking `cmd/agent`'s full k3s embed and
`cmd/controller-manager`'s host-process 10-controller build (both need far
more groups), the same class of problem (and against the same "single root
Go module, no replace duplication" CLAUDE.md rule) that already ruled out
forking `k8s.io/kubernetes` for `cmd/scheduler` above. Diffing the
freshly-generated `discovery/v1` typed client against the one already in
`k8s.io/client-go/kubernetes/typed/discovery/v1` showed them nearly
identical (the existing one is actually more complete — it has
`Apply()`/protobuf support the naive regen lacked) — there is no missing
narrow artifact for `client-gen` to produce; the per-group typed clients
needed already exist, standalone, in client-go, each independently
constructible via its own `NewForConfig`.

Client-layer measurements, incremental:

| Build content                                                                                                                                 | gzip                                    |
| --------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------- |
| Empty `main()` + `github.com/syumai/workers` only                                                                                             | 1.39 MiB                                |
| `client-go/rest` + `apimachinery/runtime/serializer`, **empty** scheme, no typed client                                                       | 4.19 MiB                                |
| `typed/core/v1` **alone** (own `NewForConfig`, references the aggregate `kubernetes/scheme` package)                                          | 9.59 MiB                                |
| Same, but with its own **narrow** scheme (only core/v1 registered) bypassing `kubernetes/scheme` via the raw `New(rest.Interface)` entrypoint | 9.59 MiB (no difference)                |
| Full aggregate `kubernetes.Clientset` (all ~54 groups)                                                                                        | 9.62 MiB (+0.03 MiB over core/v1 alone) |
| Aggregate Clientset + real `informers.SharedInformerFactory`                                                                                  | 9.97 MiB                                |
| `leanclient.Clientset` (4 real groups + ~50 panic stubs) alone                                                                                | 9.60 MiB                                |
| `leanclient.Clientset` + `leanclient.Informers` (hand-rolled, all 7 needed informers)                                                         | 9.94 MiB                                |

`k8s.io/api/core/v1`'s own type graph (dominated by `Pod`) plus
`client-go/rest`/`apimachinery/runtime/serializer`'s shared infrastructure is
already ~9.6 MiB before any other group is added; registering the _other_
~53 groups in the shared `kubernetes/scheme` package (imported by every
per-group typed client for content-negotiation, confirmed by reading
`core_client.go`'s `setConfigDefaults`) costs only **+0.03 MiB** on top —
essentially free once core/v1's own weight is paid. A hand-built narrower
scheme bypassing that shared package entirely produces the _same_ size.
**This investigation's core premise — that the generated Clientset's
breadth across ~54 groups is what costs multiple MiB — is false.**

Controller-layer measurements, the actual blocker:

| Build content                                                                                                                                                                                            | gzip                                                                                       |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `leanclient` (Clientset + Informers), zero controllers                                                                                                                                                   | 9.94 MiB                                                                                   |
| + isolated `k8s.io/kubernetes/pkg/apis/core/helper.Semantic.DeepEqual` only (one of tainteviction's imports; pulls in the internal/unversioned `apis/core` type system parallel to `k8s.io/api/core/v1`) | 10.06 MiB (+0.13)                                                                          |
| + isolated `client-go/tools/record` event broadcaster only (every one of the 5 controllers builds one)                                                                                                   | 9.98 MiB (+0.04)                                                                           |
| + isolated `k8s.io/kubernetes/pkg/features` + `apiserver/pkg/util/feature` only (project-wide feature-gate registry, imported transitively by pod-utility helpers tainteviction calls)                   | 10.17 MiB (+0.24)                                                                          |
| **+ real `pkg/controller/tainteviction` whole** (smallest of the 5 by LOC)                                                                                                                               | **16.05 MiB (+6.12)**                                                                      |
| + real aggregate Clientset/SharedInformerFactory + tainteviction (control run, same toolchain, not leanclient)                                                                                           | 17.29 MiB                                                                                  |
| **+ all 5 in-scope controllers**, leanclient                                                                                                                                                             | **16.26 MiB (+0.20 over tainteviction alone)**                                             |
| All 5 + `-ldflags="-s -w"`                                                                                                                                                                               | 15.72 MiB (-0.54, ~3% — consistent with this section's earlier "<5%, nowhere near enough") |

Three plausible culprits among tainteviction's imports were tested in
isolation and each individually cost under 0.25 MiB — nowhere near the
observed +6.12 MiB for the whole package. The remaining, not-fully-isolated
candidates are `k8s.io/kubernetes/pkg/apis/core/v1/helper` (a fuller
transitive graph than the plain `apis/core/helper` tested),
`k8s.io/kubernetes/pkg/api/v1/pod`, `k8s.io/kubernetes/pkg/util/pod`, and
`k8s.io/kubernetes/pkg/controller/util/node` — or, plausibly, no single
culprit but a cumulative effect across several moderate imports. Not
root-caused to one line (out of context budget), but conclusively **not**
in the client layer. More importantly: adding the other four controllers on
top of tainteviction cost only **+0.20 MiB more** — the same "pay once,
share across everything that needs it" pattern the client-layer
measurements showed for the other 53 API groups. This means the whole ~6
MiB cost of using real upstream KCM controller code is paid essentially
once, shared across all five, not five separate additive costs — and that
**Worker-splitting (putting different controllers in different Workers,
floated as a candidate next step above) would not help either**: a Worker
hosting even _one_ of these five controllers already costs ~16 MiB alone.

Controlled comparison (same session, same toolchain, isolating "did the
lean client help" from "did dependency/toolchain drift since this section's
original 15.03 MiB measurement" — a real concern, since this session's own
empty-`main()` baseline measured 1.39 MiB against the 1.58 MiB recorded
above): leanclient + tainteviction = 16.05 MiB vs. the real aggregate
Clientset + SharedInformerFactory + tainteviction, built the same session =
17.29 MiB. The lean client is real and does help — 1.23 MiB, about 7% — just
nowhere near enough to close a 6+ MiB gap it was never the cause of.

**Verdict: abandon.** Per CLAUDE.md rule 2 ("実際に動かして検証する") and rule 4
("訂正は隠さず記録する"), and per this task's own explicit permission to stop
rather than force a non-viable integration: the lean client does not make
`workers/controllers` deployable with these five controllers, and no
further client-layer work would change that, because the client was never
the dominant cost. `pkg/controllers/controllermanager.go` is unchanged.
This leaves kube-controller-manager in the same position this section
already settled for kube-scheduler: host-process/BYO VM only (no code
changes needed, `cmd/controller-manager` already works unchanged), unless a
materially larger undertaking (root-causing and forking/patching the
controllers' own dependency graph to cut the ~6 MiB tax at its source —
different in kind from a client shim, and not attempted here) is taken on
later. The taint-eviction-controller functional gap recorded above (eviction
not observed within that pass's time budget) was not re-investigated in
this pass: the blocker found here was size, not function, so a functional
re-test would not have changed the verdict and was not run to conserve
context. It remains open.

**2026-07-03 — Phase 6: S1's "alarms on SQLite-backed DOs cannot be
tested locally at all" does not hold for top-level (non-facet) DOs;
corrected after actually running it, twice.** S1's item 10 (this
document's S1 section above) states, unqualified, that "even on
`wrangler@latest`, alarms on SQLite-backed DOs (facet or not) cannot be
tested locally at all — `ctx.storage.setAlarm()` throws `Error: alarms
are not yet implemented for SQLite-backed Durable Objects`." This was
taken at face value into `docs/cost-model.md`'s Idle-cluster
verification checklist ("Turning this into a CI cost gate is planned
for Phase 6") as an open question. It is wrong for top-level DOs, on
the exact same wrangler version (4.106.0) S1 itself used.

Two independent checks, both against real `wrangler dev`, per CLAUDE.md
rule 2:

1. A minimal throwaway Worker + single SQLite-backed DO (`new_sqlite_classes`,
   no facets involved at all) exposing `/set` and `/get` for
   `ctx.storage.{set,get}Alarm()`: `setAlarm(+5s)` returned normally (no
   throw), `getAlarm()` correctly reported the scheduled timestamp, and
   the `alarm()` handler fired exactly on schedule, logged.
2. The real `workers/storage` Cluster DO (also `new_sqlite_classes`),
   driven the same way `docs/cost-model.md`'s Phase 4 actuals section
   describes (Node/Service create → delete → wait past the safety-net
   interval): `ctx.storage.setAlarm()`/`getAlarm()` calls inside
   `armSafetyNetSoon()`/`initialize()` (`workers/storage/src/index.ts`)
   ran without error the whole time, and the arm → fire → re-arm → park
   cycle completed exactly as that section originally reported.

Likely explanation, not fully root-caused: S1's item 8 (confirmed in
production, not locally) is that a **facet** throws immediately on
`setAlarm()` — a real, deliberate platform restriction. Item 10 appears
to have over-generalized that facet-specific finding to "SQLite-backed,
facet or not" without a from-scratch local re-test isolating a
top-level DO from a facet; this correction only re-verifies the
top-level-DO half now that a concrete use (the cost gate) depends on
knowing which half is actually true. The narrower, facet-specific claim
(item 8) is untouched by this correction and still stands as
production-confirmed.

Consequence: `.github/workflows/cost-gate.yml` reads
`ctx.storage.getAlarm()`'s persisted state directly from Miniflare's
on-disk store (`.wrangler/state/v3/do/<worker>-<Class>/metadata.sqlite`'s
`_cf_ALARM` table, `SELECT count(*)`) as an automated, non-invasive CI
assertion — no debug HTTP endpoint added to either Worker, and no
production deployment required for this specific check. See that
workflow's header comment for the full mechanism and what it does and
doesn't prove.
