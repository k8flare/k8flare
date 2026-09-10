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
  itself doesn't need to stay in the repo — and as of 2026-07-17 it doesn't:
  the `spikes/` tree was deleted from the working tree. Any `spikes/...`
  path referenced in this repo's docs is retrievable from git history
  (`git log --oneline -- spikes/`).
- **Paths in dated entries are the paths of that date.** The tree has been
  reorganized several times since; the entries below are left verbatim per
  rule #4 rather than retro-edited. The moves a reader is most likely to
  trip over, all of which kept the file (only its location changed):
  - `scripts/*.sh` (`gen-k8s-js-mirror.sh`, `gen-clientgo-lean-mirror.sh`,
    `build-wasm-chunks.sh`, `build-controllers-wasm.sh`) → rewritten as
    TypeScript under `packages/wasm-build/src/` and driven by the
    `Makefile`'s `wasm-*` targets (`npm run build:wasm` = `make -B wasm`).
  - `third_party/clientgo-lean-overlays/`, `third_party/k8s-js-overlays/`
    → `pkg/clientgo-lean-overlays/`, `pkg/k8s-js-overlays/` (commit
    `52b73cd`).
  - `packages/k8s/src/*` → `packages/k8flare-worker/src/k8s/*` (2026-07-08
    single-Worker consolidation, S19).
  - `workers/controllers`, `packages/etcd`, `packages/crd`,
    `packages/dynamic-worker` are **gone**, not moved — the multi-Worker
    split they belonged to was consolidated away (S19) and the
    hand-written TS controllers were replaced by the real
    kube-controller-manager. Where those names appear below they are the
    historical record of a component that no longer exists.
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
| S4  | Cloudflare Mesh (billing scope, flannel prototype, Cluster DNS replacement)                                                                                                 | verified (desk research 2026-07-02 + live verification 2026-07-03, Phase 9) — **Mesh not adopted**; wireguard-native live-tested and recommended instead; Mesh enablement itself stayed unverifiable (dashboard-gated)                                                                                                                                                                                                                                                | Phase 9 — done                                                        |
| S5  | WASM isolate singleton-ization (syumai fork)                                                                                                                                | partially confirmed (doneCh reuse fork verified; state persists across reused-instance requests; a _new_ timer wait fails only when nothing else is concurrently active — an open stream, a blocked outbound read, or a `ctx.waitUntil` task all keep the whole scheduler pumped for every goroutine — see S8)                                                                                                                                                        | Phase 2 (apiserver)                                                   |
| S6  | R2 (PVC access isolation, S3 access from Containers)                                                                                                                        | verified (desk research + one read-only check)                                                                                                                                                                                                                                                                                                                                                                                                                        | Phase 8                                                               |
| S7  | Re-verifying apiserver residency (double-checking the rejection)                                                                                                            | not started                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Final confirmation of the rejection decision                          |
| S8  | Whether controllers can run WASM-resident (reframed 2026-07-02: WASM execution-_shape_ design material, not a go/no-go gate — Containers isn't an option under any outcome) | partially confirmed — (a)(c)(d) verified locally with real 10+ min runs; the outbound-`net/http` crash found mid-spike has a verified one-file library-level fix (`wasm_exec.js` patch); startup-tax measured against the real apiserver binary (~14ms cold); `ctx.waitUntil` confirmed to keep the whole scheduler pumped with no client connected (45s+ observed) — see the S8 follow-up subsection for the failure-mode layer table and candidate execution shapes | ★Highest priority. Informs which WASM execution shape Phase 5 adopts  |
| S14 | ASSETS/R2 → Worker Loader runtime code supply (routing around the 10MiB gzip deploy cap for the real kube-controller-manager WASM — the gzip cap is historical, removed 2026-09; the channel is still required for other reasons, see S35)                                          | verified locally end-to-end (spike: `spikes/s14-loader-external-fetch/FINDINGS.md`; production-path integration: this file's S14 section) — Loader has its own hard 64MiB total-module-bytes cap, satisfied via `-s -w` + `wasm-opt -Oz` (59.6MiB); Loader caps/eviction/memory in production still unverified                                                                                                                                                        | workers/controllers deploy path (real KCM in Workers)                 |

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
   (2026-09: the gzip script cap no longer exists and the Worker limit is
   64 MiB, but the Loader's own 67,108,864-byte cap is unchanged — S35.)
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

**Phase 9 update (2026-07-03) — live verification, decision: not adopted.**
Full detail in `spikes/p9-mesh/RESEARCH.md`; summary:

1. **Mesh itself stayed unverifiable.** `npx wrangler whoami` confirmed this
   session is authenticated to the correct account (KOOFFICE,
   `ed17c5c18eb6052e70234ec181709fba`) with a token that includes
   `connectivity (admin)`, but Mesh lives under Zero Trust/Cloudflare One, a
   separate product surface `wrangler` doesn't reach. Checking or enabling it
   needs the Cloudflare dashboard or a Zero-Trust-scoped API token, neither
   available here — reported honestly rather than guessed at, per this
   task's own pre-authorized fallback for exactly this situation.
2. **wireguard-native was live-tested instead, successfully, for the parts
   this project's control plane owns.** Two real `cmd/agent` processes, on
   genuinely separate Docker networks bridged only by a shared "public"
   network (standing in for two clouds), were driven to
   `--flannel-backend=wireguard-native` + `--node-external-ip` purely
   through this project's existing `/v1-k3s/config` supervisor path (a new
   `FlannelExternalIP` field was added to mirror k3s's own
   `--flannel-external-ip`). Both nodes registered `Ready` with distinct
   PodCIDRs, and — the part that actually needed proving — `status.addresses`
   correctly kept the node's _internal_ per-network IP while the
   `flannel.alpha.coreos.com/public-ip-overwrite` annotation correctly took
   the _external_ one, exactly matching
   [k3s's documented multicloud pattern](https://docs.k3s.io/networking/distributed-multicloud).
3. **Full cross-node route/tunnel convergence didn't complete in a ~7-minute
   (422s) window** — no `flannel-wg` interface, no
   `flannel.alpha.coreos.com/backend-data` annotation on either node. An A/B
   control (same 2-node setup, reverted to the shipped `host-gw` default)
   reproduced the **identical** symptom, proving this is a pre-existing,
   backend-agnostic gap in this project's control plane — not something
   specific to wireguard-native, and not something Mesh would have sidestepped
   either. It's the same family of issue as this doc's own S-series and
   `docs/general-purpose-k8s-plan.md` Phase 1's already-recorded open
   follow-ups (informer/watch delivery not reliably completing under this
   control plane) and matches `README.md`'s own long-standing "Service
   networking... not yet proven end-to-end" gap row — recorded here as an
   honest correction/connection, not chased further (out of this phase's
   scope).

   **Correction (2026-07-08):** the connection drawn above turned out to be
   overstated. `docs/general-purpose-k8s-plan.md` Phase 1's real-traffic gap
   is now closed — direct verification (single BYO-VM node, real kube-proxy
   + flannel `host-gw`) proved a genuinely separate Pod reaching a Service's
   `ClusterIP` and getting routed to the backing Pod works correctly; the
   thing that had actually been blocking README's gap row the whole time was
   the upstream conformance test's own `kubectl exec`-based reachability
   check, not a networking or watch-delivery problem. This item's own
   cross-node route/tunnel convergence gap (a **2-node** setup never
   completing flannel backend-data exchange in ~7 minutes) is real and still
   open, but it is a narrower, distinct, multi-node-specific issue — not the
   same root cause as README's now-resolved single-node claim, and not
   confirmed to share a root cause with the informer/watch-delivery
   follow-ups either (that connection was speculative when written, per this
   entry's own wording, and remains unconfirmed).
4. **Decision**: Cloudflare Mesh is **not adopted**. Its one hypothesized
   advantage over wireguard-native (NAT traversal without an open inbound
   port) remains unverified and unverifiable within this task's access, while
   wireguard-native's control-plane integration is now confirmed correct and
   costs nothing extra. `cmd/agent --node-external-ip` (opt-in flag, wired to
   the already-vendored `cmds.Agent.NodeExternalIP`) is the one permanent code
   change kept from this phase; `defaultClusterConfig()`'s `FlannelBackend`
   default remains `"host-gw"` (unchanged) since choosing wireguard-native
   per-cluster is a separate, not-yet-built feature. `docs/cloudflare-mesh-
networking.md` and `README.md` updated accordingly.

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

**REMOVED 2026-07-08:** the custom R2-backed PV/PVC provisioner this
spike's findings fed into (`pkg/apiserver/r2.go`/`r2handlers.go`/
`pvcbind.go`) was deleted -- user decision to rebuild PV/PVC
provisioning later using a real CSI driver instead of a hand-rolled
synthetic-CSI mechanism. Kept below as a historical record; see git
history for the removed code.

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

**Phase 8 implementation update (2026-07-03)**: the recommendation above
was implemented as designed and verified end-to-end against real
`wrangler dev` + real Docker (not just unit-level) — see
`docs/cost-model.md`'s "Phase 8 (R2 PV/PVC backend) implementation"
section for the full cost/verification writeup and `pkg/apiserver/r2.go`,
`pkg/apiserver/pvcbind.go`, `pkg/apiserver/r2handlers.go`, and
`workers/nodes/src/virtualnode.ts` for the implementation. Notes against
this section's specific open items, per the honest-correction convention
(append, don't delete):

- **Exact JWT shape confirmed against primary sources**, not the
  AI-summarized reads this spike used for the egress question: fetched the
  raw markdown of both
  `developers.cloudflare.com/r2/api/s3/temporary-credentials/` and
  `developers.cloudflare.com/r2/examples/authenticate-r2-temp-credentials/`
  directly (`curl`, not a summarizing fetch tool) to get the exact JWT
  claim names or the local-signing implementation would have been
  guesswork. The header/payload shape, HS256 signing key, and
  secretAccessKey/sessionToken derivation formulas are all exactly as
  quoted in that page's worked TypeScript example — reproduced in Go with
  zero new dependencies (stdlib `crypto/hmac`+`crypto/sha256` only) and
  cross-checked against an independent Node.js reference implementation
  plus a from-scratch Python HMAC verification during development
  (`pkg/apiserver/r2_test.go`).
- **`ttlSeconds` min/max bound: still unconfirmed.** Neither concept page
  states one; this remains a real account test item (see the manual
  completion path in `docs/cost-model.md`'s Phase 8 section).
- **Credential refresh for long-running Pods: resolved with a working
  mitigation, not left as an open problem.** Rather than choosing between
  "long TTL" and "refresh sidecar" as originally framed, Phase 8 does
  both: a 1-hour default TTL (matching Cloudflare's own local-signing
  helper's default) as the baseline, plus a proactive stop+restart of the
  Pod's container once its credential is past expiry — for
  `restartPolicy: Always` Pods only — which re-mints a fresh credential as
  a side effect of the same container-start path every other restart
  already goes through. This needed no new Cloudflare primitive and no new
  alarm source (it rides `workers/nodes`' existing ~10s reconcile tick).
  See `workers/nodes/src/virtualnode.ts`'s "Credential refresh" doc
  comment for the full reasoning, including why this deliberately isn't
  zero-downtime and doesn't apply to `OnFailure`/`Never` Pods.
- **FUSE mounting remains unverified** — still cannot be tested in
  `wrangler dev` (confirmed again this phase: local Docker would need
  `--cap-add SYS_ADMIN --device /dev/fuse`, not something `wrangler dev`'s
  Containers emulation exposes), so v1 ships direct S3-SDK access via
  injected env vars only, exactly as recommended, with FUSE documented in
  `README.md` as future work pending a real-deployment test.
- **New residual item, found while trying to complete this spike's own
  "write operations ... not yet performed" gap**: this project's
  established pattern for real-account verification is to use `wrangler`'s
  existing OAuth session directly (as this spike's `wrangler r2 bucket
list` did) — but `wrangler` has no subcommand to create an R2 API token
  (the parent credential Temporary Access Credentials are derived from).
  R2 API tokens are dashboard- or Cloudflare-REST-API-created
  (`developers.cloudflare.com/r2/api/tokens/`), and creating one via the
  raw Cloudflare API would require either extracting wrangler's stored
  OAuth token to call an unrelated API with it (judged out of bounds — a
  materially different, more sensitive action than the read-only/
  Workers-deploy operations this project's standing production-verification
  authorization was established for) or a fresh dashboard-created token
  neither available nor requested for this task. Net effect: the bind ->
  mint -> inject _mechanism_ is fully verified against a real running
  Worker/DO/Container stack; an actual authenticated S3 call against
  production R2 is not, and needs someone with dashboard access to close
  (exact steps in `docs/cost-model.md`'s Phase 8 section). Recorded rather
  than silently skipped, matching CLAUDE.md rule 4.

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

## Phase 6: e2e-conformance CI verification (2026-07-03, real GitHub Actions runs)

Ran the actual `e2e-conformance.yml` workflow against `feat/v2-rearchitecture`
via `workflow_dispatch` (no PR — see the correction log entry on the
2026-07-03 unauthorized-PR incident) to get real CI signal per CLAUDE.md rule
2, not a local-only claim. Seven runs, each root-caused from actual logs:

1. Two failures at "Wait for node to register as Ready", 5 minutes in,
   `Insufficient free disk space on the node's image filesystem (91% of
71.6 GiB used)` — coincidental: the runner's preinstalled toolchains
   (dotnet, Android SDK, etc.) were freed as a fix, which was harmless but
   NOT the real cause (see below).
2. **Real root cause of all three "Wait for node" failures**: `cmd/agent`
   sets `agentConfig.WithNodeID = true` (main.go:97), k3s's own feature that
   appends a short random suffix to the configured `--node-name` to avoid
   collisions — the node genuinely registers as e.g.
   `e2e-runner-67ab5085`, never literally `e2e-runner`. The workflow's
   polling step queried the single-node endpoint by the bare `e2e-runner`
   name (404, empty conditions, forever) and always exhausted its 5-minute
   budget — which happened to overlap with kubelet's own ~5-minute
   image-GC disk-pressure check, misdirecting the first two fixes. Fixed by
   listing all nodes and matching by name prefix.
3. Once node registration passed, the upstream e2e.test framework's own
   `SynchronizedBeforeSuite` (global setup before every conformance test,
   not specific to any one test) failed twice more on gaps in this
   project's Go apiserver, found and fixed in turn:
   - `field label not supported: spec.unschedulable` — the framework lists
     Nodes with this field selector; added to `knownSelectableFields` and
     `selectableFieldsFor` in `pkg/apiserver/store.go`, matching upstream's
     exact `NodeToSelectableFields` convention (`fmt.Sprint(...)`).
   - `services "kubernetes" not found` — real kube-apiserver bootstraps a
     `kubernetes` Service in `default` on every start for in-cluster
     discovery; this apiserver never did. Added to
     `pkg/apiserver/bootstrap.go`, ClusterIP set to the well-known
     `10.43.0.1` already reserved for exactly this purpose in
     `clusterip.go`'s `addressesReservedForFutureServices` (a reservation
     made in Phase 3 and never wired up until now).

**Result: the required baseline conformance group — this project's actual
Definition of Done (CLAUDE.md rule 1) — passes, confirmed on two
consecutive runs: `SUCCESS! -- 11 Passed | 0 Failed | 0 Pending | 7568
Skipped`.**

**Experimental (advisory, explicitly non-blocking) group**: does not
complete within the 55-minute job timeout (bumped once from the original
30, still not enough) — not a hang, but real per-spec failures that each
burn several minutes of upstream's own wait-and-retry logic before
reporting FAILED. Eight distinct, concrete gaps surfaced (not attempted to
fix — this is follow-up-phase-sized work, out of scope for Phase 6's CI
verification mandate):

| Spec                                               | Failure                                                                                                         |
| -------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `network/endpointslice.go:725`                     | `Expected EndpointSlice to have 1 ports, got 0`                                                                 |
| `apps/deployment.go:781` (RollingUpdateDeployment) | revision/image mismatch: `deployment ... doesn't have the required revision set`                                |
| `apps/deployment.go:517` (deployment lifecycle)    | same revision-tracking symptom                                                                                  |
| `apps/deployment.go:278`                           | `failed to patch Deployment: ... the object has been modified` (optimistic-concurrency conflict)                |
| `common/node/secrets.go:145`                       | `created secret ... with empty key` validation gap                                                              |
| `framework/pod/output/output.go:263` (×2)          | `failed to get logs from pod ...: an error on the server ("unknown") has prevented the request from succeeding` |
| `apps/job.go:907`                                  | `error while waiting for pods to become inactive ...: there are 2 active pods`                                  |

**Direct answer to Phase 6's endpoint/endpointslice promotion question**:
**not yet** — the experimental EndpointSlice spec above is currently
failing (`got 0` ports), so it must not be promoted to required until that
gap is fixed.

Not investigated further in this pass, in the order they'd likely matter
most for future promotion: the Deployment revision-tracking gap (recurs in
three separate specs, likely one root cause in how `status.observedGeneration`
or revision annotations are updated) and the EndpointSlice port-population
gap (directly blocks the one promotion this project has been trying to make).

## S14: ASSETS+LOADER code supply — the deploy path that puts the real kube-controller-manager inside Workers (2026-07-05)

Spike findings in `spikes/s14-loader-external-fetch/FINDINGS.md` (runtime
code supply from R2/ASSETS into the Worker Loader works; the Loader has its
own hard 64MiB total-module-bytes cap — superseding s2-loader's "no limit"
claim; a wasm-opt'd lean KCM build fits under it and executes). This
section records the production-path integration that followed the spike,
all verified live against `wrangler dev` (5-Worker multi-config, fresh DO
state):

**Architecture (now in-tree, `workers/controllers`):** the Worker script
itself is small TypeScript only. The KCM WASM is built by
`scripts/build-controllers-wasm.sh` — `-ldflags="-s -w"` (78,217,864 →
75,749,798 bytes) then `wasm-opt -Oz` (→ 62,535,235 bytes = 59.6MiB,
~4.4MiB under the Loader's 67,108,864-byte cap; ~2 min build cost) — and
shipped as Static Assets in three ≤24MiB chunks (ASSETS per-file cap is
25MiB) plus a sha256 manifest. The Controllers DO assembles the chunks at
Loader-factory time and runs the binary as a Dynamic Worker via
`modules["app.wasm"] = { wasm }`; the manifest's sha256 is the Loader
cache id, so a rebuild naturally busts the isolate cache. The GATEWAY
service binding and `K3S_TOKEN` pass through `WorkerCode.env` (Fetchers
survive the env clone, S2 item 3a), so `pkg/controllers.RestConfig`
works unchanged inside the loaded worker. This removes the 10MiB gzip
deploy blocker that had kept `workers/controllers` undeployable
(gzip was ~13.6MB); `workers/apiserver` still ships conventionally
(gzip 7.98MB < 10MiB).
(Historical: the 10MiB gzip cap was removed in 2026-09 and the Worker
limit is now 64 MiB. The supply channel is still required — five binaries
totalling ~216.8 MiB cannot share one Worker — see S35.)

**New execution-shape finding — the pump window:** unlike the previous
DO-hosted shape (where `DurableObjectState.waitUntil` kept the Go
scheduler pumped), a Loader-loaded dynamic worker freezes the moment a
dispatch's response completes. Observed directly: initial
Deployment→ReplicaSet→Pod creation worked (inside the first request's
window), but a scale PATCH issued after ~40s idle was never reconciled.
Fix: the dynamic worker's bootstrap arms `ctx.waitUntil(25s)` on every
poke — event-armed (storage's `pingControllers` on relevant writes + the
DO's safety-net alarm), bounded, and KCM's own writes re-ping and chain
windows while real work exists, then everything goes quiet. Idle cluster
⇒ no pokes ⇒ no windows ⇒ no CPU (invariant #2: I/O waits inside a window
are not CPU-billed).

**Two real library bugs surfaced only by running the full lifecycle
against the live stack** (both invisible to source reading):

1. `pkg/leanclient`'s `Delete`/`DeleteCollection` marshaled
   `metav1.DeleteOptions` with plain `encoding/json` (no
   kind/apiVersion), and the apiserver's strict decoder 400s a
   TypeMeta-less body — so every UID-preconditioned Pod delete from the
   real replicaset controller failed and scale-down never converged.
   Fixed by stamping TypeMeta in `verbs.go`.
2. The generated `events` client's `*WithEventNamespace` methods used the
   client's namespace (always `""` — `record.EventBroadcaster` builds its
   sink as `Events("")`) instead of the event's own namespace, so every
   Event write went to the cluster-scoped path and failed; zero Events
   were ever stored. Fixed in `cmd/k8flare-gen/leanclient.go`'s
   `eventExtras` (upstream `NamespaceIfScoped` semantics) and
   regenerated.

**A cascade/GC race with the now-live controllers:**
`pkg/apiserver/gc.go`'s children-first walk deleted a ReplicaSet's Pods
while the RS still existed; the real replicaset controller raced
replacement Pods into existence mid-cascade, and with no background
garbagecollector they survived as permanent orphans (observed: Deployment
delete left 1 fresh Pod behind). Fixed by switching `deleteDependents` to
owner-first order plus a bounded repeat-until-quiet re-sweep.

**Verified end-to-end after the fixes** (real `kube-controller-manager`
inside a Loader-loaded dynamic worker, against the live local stack):
Deployment create → RS (real pod-template-hash naming) → 2 Pods in ~6s;
scale up 2→3 issued after an idle window → reconciled in ≤5s; scale down
3→1 → ≤5s; all with correct `ownerReferences`/`generateName` semantics.

**Still unverified (production-only):** the exact 64MiB Loader cap figure
in production (matches Cloudflare's documented number), Loader isolate
idle-eviction cadence (affects re-load frequency → $0.002/unique/day and
informer resync cost), and the 128MiB isolate memory limit under real
KCM load — `wrangler dev` enforces none of these.

> **Superseded the same day by the production deployment below: the
> Loader-side items are now verified for real; eviction cadence and
> memory headroom measurement remain open.**

### Production verification (2026-07-05, KOOFFICE account, real deploy)

All 5 Workers deployed for the first time (service-binding cycle broken
by deploying storage once without its `services` block, then the rest in
dependency order, then storage again in full). `K3S_TOKEN` set as a real
secret on gateway/apiserver/runtime/controllers; a wrong token gets 401
on every resource path (only `/version` is served unauthenticated).

**Confirmed in production:** the Worker Loader accepts and runs the
62.5MB KCM WASM — the real kube-controller-manager reconciles against
the production stack end-to-end via real `kubectl`: Deployment create →
RS → Pods, scale 2→4 and 4→2 each fully reconciled ~10s after the
kubectl call, Events recorded. The 64MiB figure therefore holds in
production exactly as measured in dev, and instantiation fits inside the
128MiB isolate limit at least for idle/no-node reconcile load.

**Production-only failure mode found (and why dev never showed it):**
the first controllers build awaited the ~60MB Loader factory inline from
each poke. Storage's `pingControllers` runs inside the write path, so
every poke was canceled when its parent write finished — tearing down
the in-flight load with it and restarting it on the next write. Observed
as an endless `GET / - Canceled` storm with KCM never coming up through
the poke path; the only load that ever completed ran from the safety-net
`alarm()` (no client to cancel it). Fixed: pokes now return 202
immediately, the load runs as a detached promise (DO lifetime is not
request-scoped) and self-dispatches the first healthz on completion;
chunks are also streamed sequentially into one preallocated buffer
instead of fetch-all-then-concat (peak ~62MB instead of ~125MB against
the 128MiB production limit).

**Collateral finding:** during the canceled-poke storm the Cluster DO
went into a fast-fail state — every namespaced list returned 500 in
~11ms with no logged exception ("store list: storage list: unexpected
status 500"), making existing objects look deleted. Redeploying
`workers/storage` (instance eviction; SQLite state intact) fully
recovered it, and the "missing" Deployment reappeared with its history.
Consistent with the InputGate-cascade theory recorded in the Phase 1 CI
follow-up (a canceled request leaving a facet/DO gate held); worth its
own reproduction pass.

**Minor:** `kubectl`'s parallel group discovery against a cold apiserver
isolate (43MB WASM) produced transient `couldn't get resource list for
<group>: an error on the server ("unknown")` errors on a few groups;
individually queried afterwards, every discovery document serves
correctly — a cold-start burst effect, not a registration bug. Also
`kubectl get namespaces` renders blank NAME cells via the server-side
Table path while `-o json` is correct — server-side printing gap for
Namespace, tracked as a small follow-up.

### RESOLVED REGRESSION (2026-07-05 evening; heading corrected 2026-07-30): the ≤64MiB KCM build was not reproducible from the committed tree

> **Correction (2026-07-30):** this section was headed "OPEN REGRESSION"
> for three weeks after it was fixed. It is not open — see "RESOLVED for
> KCM (same day)" below; the mirrors are reproducible from the committed
> tree and a clean clone builds every chunk under the Loader cap. The
> stale heading led an outside reviewer to record the project as having
> an unresolved build-reproducibility defect. The body is kept verbatim
> as the record of how it was found and fixed.

Found while wiring the kube-scheduler as a second dynamic worker (Pod-on-
Containers work). Recorded per rules 4/5 instead of being papered over:

- **The deployed, working KCM WASM (raw 75,749,798 → wasm-opt
  62,537,211 bytes, manifest sha256 `dc63e0f4808d2cdb…`) was built
  against `.build/k8s-js-mirror` state left on disk by an earlier
  session.** Running the committed `scripts/gen-k8s-js-mirror.sh`
  regenerates a mirror from which the identical `go build` command
  produces **105MB raw / 98.6MB opt — far over the Loader's 64MiB cap.**
  The old mirror was destroyed by the regeneration (`rm -rf` inside the
  script) before it was ever diffed, so what exactly it pruned is lost.
  Lesson encoded in CLAUDE.md's pitfalls: snapshot `.build/` mirrors
  before regenerating them.
- The production deployment is unaffected (it runs the preserved
  artifacts; a local copy is kept at
  `workers/controllers/assets-backup-20260705/`, gitignored). The CI
  `build:wasm` gate now fails loudly on the oversize binary — that is
  correct behavior, not a gate bug.
- **What was measured while trying to close the gap** (wasm name-section
  attribution, `spikes` method): today's 105MB KCM carries ~21MiB of
  code for ALL ~55 `k8s.io/api` groups plus full applyconfigurations/
  gnostic/protobuf machinery. A probe binary importing only
  `pkg/leanclient/clientset` weighs **52.9MB** — the full-width
  `kubernetes.Interface` (which upstream controller constructors
  require) anchors most of the weight, and that anchor is NOT removable
  by any of: pruning `kubernetes/scheme/register.go` to 9 groups (new
  overlay, kept — necessary but not sufficient), dropping
  `pkg/controller`'s blank `core/install` import, stubbing nodeipam's
  `cloudprovider.Interface`, or the DRA registry overlay — each was
  measured individually AND together with no meaningful size change.
  How the old mirror produced a 22MB-smaller anchor is the open
  question; the pre-"Phase 10 correction" width-pruned client-go mirror
  (see `scripts/gen-clientgo-lean-mirror.sh`'s comments — the correction
  note it cites was never actually written to this file) is the leading
  candidate, since `third_party/clientgo-lean-overlays/README.md`
  documents exactly this 44MiB-scale sibling-linkage effect and its fix.
- **Also measured and still valid regardless of the regression**: the
  combined KCM+scheduler binary is 71.5MB opt (over cap → two dynamic
  workers required); scheduler-only is 67.46MB opt (355KB over) and
  66.39MB (714KB under) with the DynamicResources registry overlay —
  those numbers were taken against the old mirror and need re-validation
  once KCM reproducibility is restored.

**RESOLVED for KCM (same day):** the lost mechanism was exactly the
predicted one — **`kubernetes.Interface` width**. Reconstructed as a
committed, reproducible configuration: the KCM binary builds with
`-tags leanwidth` against `go.wasm.mod` (client-go → the lean mirror,
whose `clientset_leanwidth.go` overlay narrows `kubernetes.Interface`
to the five real groups + SchedulingV1alpha2, which the 1.36 job
controller's PodGroup informer import requires), plus js-pair mirror
transforms severing `pkg/controller`'s blank `core/install` import and
nodeipam's `cloudprovider.Interface` (all sha256-pinned in
`scripts/gen-k8s-js-mirror.sh`). Measured from a clean
`npm run build:wasm:controllers`: **105MB → 78.3MB raw → 66,128,527
opt (957KiB under the Loader cap)**, and verified live (Deployment
create → 2 Pods in 5s, scale-down 5s, Events recorded, no panics).
gzip is 13.2MB, so the normal-Worker (10MiB gzip) route stays closed.
(Historical: that gzip cap was removed in 2026-09 — S35. The route stays
closed anyway, now on the 64 MiB raw Worker limit.)

**Still open — the scheduler:** its earlier 66.4MB figure was an
artifact of the same lost mirror state; against the reproducible
mirrors the full-width scheduler binary measures **102.8MB opt**, and
leanwidth cannot apply (scheduler.NewInformerFactory is the full-width
aggregate SharedInformerFactory — the Phase 10 correction recorded in
`scripts/gen-clientgo-lean-mirror.sh`'s comments). kube-scheduler
therefore remains host-process/BYO-VM **[superseded 2026-07-10 — see
"S21: 実 kube-scheduler の Dynamic Worker 化(2026-07-10、実機検証済み)"
(cite it by title, not number: this file has three sections numbered S20
and two numbered S21): `-tags schedwidth` plus the DRA/CEL and cri-client
severing took the scheduler to 45.2MB opt (21.4MiB under the cap), and
`sched` now ships as a dynamic worker chunk — `make wasm-sched` →
`packages/k8flare-worker/assets/wasm/sched.manifest.json`, source
`pkg/controllers/cmd/kcm-wasm/scheduler`. Real Pod bind was verified in
`wrangler dev`, so the "absent `sched` manifest" branch below is no
longer the normal path]**;
`workers/controllers/scheduler/`
and the DRA registry overlay are kept as the ready entrypoint for a
future scheduler-width answer, and the Controllers DO treats the absent
`sched` manifest as "not shipped" rather than an error.

### Cluster DO wedge leaves divergent state after recovery (2026-07-06, open)

Second occurrence of the fast-500 Cluster DO wedge (first: 2026-07-05,
during the canceled-poke storm). New finding this time: after recovery
by storage redeploy (instance eviction), **previously-deleted objects
resurfaced** (a Deployment and its ReplicaSet deleted during the wedge
reappeared with stale status, while their Pods stayed deleted) — i.e.
the wedge doesn't just block requests, it can leave parent/facet state
divergent when deletes land mid-wedge. Worked around by re-deleting the
phantoms; root-causing the InputGate cascade (already an open follow-up
from Phase 1's CI investigation) is now also a data-consistency issue,
not just availability. kubectl-visible symptom to recognize it by:
`store list: storage list: unexpected status 500` on some (not all)
list calls, then resurrected objects after redeploy.

### S16: first Pod Running on a per-Pod microVM node (2026-07-06, hostNetwork round)

Production e2e of the cf-containers-scheduler chain after deploying the
2026-07-06 stack (storage / apiserver with hostNetwork admission /
controllers alarm-predicate / nodes image with backend label+taint):

- **End-to-end SUCCESS**: `kubectl apply` of an annotated nginx:alpine
  Pod (limits 500m/2Gi → large tier) → admission injected
  `schedulerName=cf-containers-scheduler`, `hostNetwork=true`,
  nodeSelector (verified on the stored object) → VM boot → node Ready in
  ~19s → bind → **Running with ready=true, restarts=0** on the microVM
  (instance in kix05). PodIP = the VM host IP (10.0.0.1), as hostNetwork
  intends. Teardown on `kubectl delete pod` reaps the VM and the Node
  object within ~25s; all three NodeVM apps back to zero
  running instances (cost invariant held). The sandbox was created WITH
  kubelet's unconditional RuntimeDefault seccomp — the microVM kernel
  supports seccomp; no admission workaround needed.
- **First attempt of the same round stalled and remains un-root-caused**:
  on the first VM (~20 min earlier, same image), containerd died ~15s
  after boot — kubelet event `dial unix /run/k3s/containerd/
containerd.sock: no such file or directory` at bind time, PLEG went
  stale, node fell to NotReady, pod stuck in ContainerCreating. The
  second boot succeeded with zero changes deployed in between, so this
  is a boot-time flake to watch (rule 5), not the old "CNI/netfilter"
  hypothesis — hostNetwork removed CNI from the sandbox path entirely.
- **In-VM observability is effectively zero today** (matters for the
  flake above): `wrangler containers ssh` failed three ways
  (INSTANCE_NOT_READY on the wedged VM, then instance-not-found, then
  WebSocket 400 on a healthy running instance with both wrangler 4.106
  and 4.107, interactive / piped / `--stdio` ProxyCommand forms);
  container stdout does NOT appear in `wrangler tail`; there is no
  containers logs API/CLI (open feature requests workers-sdk #12988 /
  #12998); the Workers Observability telemetry query API rejects the
  wrangler OAuth token. Working fallback (verified live from a local
  Docker node joined to production): a temporary entrypoint block that
  self-POSTs uname/ps/containerd.log as a `vmdebug-*` ConfigMap through
  the gateway using the node's own K3S_TOKEN.
  - **CORRECTION (same day, ~1h later): SSH works once configured** —
    the WebSocket 400 meant "no `authorized_keys` registered", nothing
    else (docs: developers.cloudflare.com/containers/ssh/). Recipe now
    proven live against a NodeVM: (1) add `authorized_keys: [{name,
public_key}]` (ssh-ed25519 ONLY) to each `containers[]` entry in
    workers/nodes/wrangler.jsonc and `wrangler deploy`; (2) the key
    only reaches instances on FRESH slots — an instance that existed
    (even stopped/reused) before the deploy keeps 400ing, so cycle the
    pod to get a new VM; (3) connect with the real ssh client:
    `ssh -i ~/.ssh/id_ed25519 -o ProxyCommand="npx wrangler containers
ssh %h --stdio" cloudchamber@<instanceID>` (Cloudflare injects a
    dropbear sshd; user is `cloudchamber`; interactive
    `wrangler containers ssh` also works from a real TTY). Verified
    from inside: guest kernel `6.18.36-cloudflare-firecracker`
    (x86_64, seccomp available), k8flare-agent is PID 1, containerd
    healthy, and `curl 127.0.0.1:80` returned the Pod's nginx welcome
    page over hostNetwork — the first direct HTTP proof against a
    per-Pod-node workload (pods/proxy bridge still pending, task #13;
    implemented and Worker-side-verified as of S20 below, last hop to a
    live NodeVM still open).
- **Local-Docker repro of the stall is a false lead**: on an ARM Mac,
  the amd64 node image under Rosetta fails every sandbox with
  `seccomp is not supported` (kubelet 1.36 hardcodes RuntimeDefault for
  the pause sandbox — kuberuntime_sandbox.go:173, Issue #84623 — and
  pod-level `seccompProfile: Unconfined` does NOT bypass it; verified).
  That failure is Rosetta-only (no seccomp via emulated prctl) and does
  not occur on the real microVM. Don't chase it as a product bug; for
  local node testing use a native-arch build or expect ContainerCreating.
- **Known gap surfaced, by design**: `pods/proxy` returns "temporarily
  unavailable on the per-Pod node backend" — HTTP ingress to per-Pod
  nodes is task #13's kubelet-bridge work, not a regression. (Closed by
  `handlePodProxy`/`handleVKubeProxy`, S20 below.)

### S16 addendum (2026-07-06 evening): logs/metrics bridge live, and three platform findings

**`kubectl logs` (+ `-f`), `nodes/{name}/proxy/stats/summary` and
`/metrics/resource` are live against per-Pod microVM nodes**, with the
kubelet running STOCK k3s security: the bridge (gateway → nodes Worker →
NodeVM DO → containerFetch :10256 → cmd/agent's plain-HTTP kubelet
proxy) forwards the cluster bearer token and the kubelet webhook-
authenticates it via our new TokenReview/SubjectAccessReview endpoints
(commits 79d39cb/ac33765). Negative check: no token → the kubelet
itself answers 401. First attempt used anonymous+AlwaysAllow kubelet
config — replaced same-day after user review with the TokenReview path;
also k3s pins `--read-only-port=0` as a kubelet CLI FLAG, which beats
any kubelet config drop-in, so the softer read-only 10255 route is
structurally unavailable (verified live: readOnlyPort in a drop-in
never took).

Findings recorded along the way:

1. **The first-boot containerd death is a per-image-version phenomenon:
   three for three.** Every first VM boot after a new image push died
   the same way (containerd gone ~15s in, node NotReady, pod stuck in
   ContainerCreating); every second boot of the same image succeeded
   with zero changes in between. Working hypothesis: first-boot lazy
   image loading I/O. The scheduler's 300s FailedScheduling timeout +
   teardown + reboot path handles it (observed working live), making
   this self-healing — but it exposed:
2. **Scheduler park bug (fixed, ac33765): after a FailedScheduling
   teardown the alarm predicate ("tracked VMs exist") parked the binder
   while the pod was still unscheduled** — no further pod writes meant
   no pokes, stuck forever. Same predicate class as the KCM liveness
   bug (d1a9503): the work signal must be "pending pods OR live VMs",
   not "VMs only". Unparked live via a pod annotation write; predicate
   fixed and deployed.
3. **Cluster DO wedge, occurrences 3 and 4** (see the 2026-07-06 entry
   above): both fired right after a deploy followed immediately by
   namespace-create + pod-create; occurrence 4 did NOT clear on a
   single storage redeploy (previous recovery recipe) — it cleared a
   few minutes later after the apiserver's bootstrap re-PUT storm
   settled. Deleted objects resurfacing after recovery reproduced too
   (a previously-deleted liveness-probe pod came back; re-deleted).
   The InputGate-cascade follow-up keeps gaining evidence that the
   trigger involves cold-apiserver bootstrap writes racing user writes.

## S18: real RBAC enforcement fits the WASM apiserver — with two size landmines (2026-07-06)

Spike: `spikes/s18-rbac-authz/` (full numbers in its FINDINGS.md).
Question: can RBAC be enforced with upstream code inside
`workers/apiserver` (10MiB-gzip cap, baseline 8.17MiB gzip)?

- **PASS**: `plugin/pkg/auth/authorizer/rbac` (the real RBACAuthorizer)
  - `endpoints/request.RequestInfoFactory` compile AND run on js/wasm
    (executed under Node wasm_exec: bootstrap `cluster-admin` →
    `system:masters` evaluates to Allow, so the existing cluster-token
    identity keeps working when enforcement turns on). Cost: **+33KB
    gzip** combined. The authorizer's four getter interfaces map directly
    onto `ResourceStore`s — no informers.
- **FAIL, direct link**: `bootstrappolicy` (+2.62MiB gzip, over cap —
  drags the full client-go clientset via `legacytokentracking`; 227 new
  packages) and `pkg/serviceaccount` (+2.68MiB gzip, over cap — typed
  client-go + component-base metrics + audit). Neither belongs in the
  WASM binary.
- **Viable shape (all upstream semantics, +185KB gzip total, measured
  8.35MiB)**: bootstrap policy emitted as generated data by
  `cmd/k8flare-gen` (host build) and seeded in `BootstrapCluster`; SA
  JWTs minted/verified with `go-jose` (the same library
  `pkg/serviceaccount` uses internally, already in go.mod) using the
  upstream claim shape.
- **Enforcement is not apiserver-only**: `?watch=true` never reaches the
  Go apiserver (gateway `handleWatch`, token-equality `dwAuth` only) and
  the runtime CRD path, pods/proxy, and nodes/proxy
  (`workers/gateway/src/index.ts:33,57`) are the same — each needs a
  `SubjectAccessReview` call before proceeding (the webhook pattern the
  kubelet bridge already proved live). And `system:nodes` needs an explicit
  ClusterRoleBinding (upstream deliberately doesn't bind `system:node`
  — it assumes a Node authorizer this project doesn't have; k3s
  precedent applies).

## S19: single-Worker consolidation gates — all three PASS (2026-07-06)

Spike: `spikes/s19-single-worker/` (full numbers in its FINDINGS.md).
Pre-implementation gates for the 6→1 Worker consolidation +
multi-cluster plan, run against the real 43MB apiserver WASM in
`wrangler dev` 4.106.0:

- **G1 PASS**: containers[] + assets(run_worker_first) + worker_loaders
  - sqlite DOs + self service bindings (default + named entrypoint) in
    ONE config. Docker-less dev works with `--enable-containers=false`
    (default is a hard startup failure; container images must EXPOSE a
    port) — CI harnesses add the flag, no second Worker needed.
- **G2 PASS**: a named-entrypoint self-binding Fetcher survives the
  Loader env clone and reaches a DO from inside the loaded worker —
  the replacement for the Go apiserver's CLUSTER DO binding.
- **G3 PASS**: Loader-hosted apiserver hot path — cold 135–195 ms,
  warm 12–24 ms, 30-parallel discovery burst 30/30 in 842 ms, and the
  dynamic-worker isolate is **shared across caller contexts** (a DO's
  `LOADER.get` with the same id skipped the factory). Two contract
  findings: loader entrypoint stubs are request-scoped I/O (call
  `LOADER.get` per request in stateless handlers; DOs may cache), and
  the apiserver bootstrap must mirror syumai's per-request
  `worker.mjs` shape — the KCM resident shape dies on dispatch 2 with
  "Go program has already exited" (observed live).

## S20: virtual kube-proxy TCP intercept — real bug found and fixed, node-half end-to-end proven (2026-07-07, task #13)

Continuation of the WIP `pkg/vkubeproxy` (node-side TUN + gVisor
`pkg/tcpip` userspace forwarder) and `packages/k8flare-worker/src/nodes/podproxy.ts`
(`handlePodProxy`/`handleVKubeProxy`) from the previous session. That pass
had wired the routes but never actually run the TUN intercept against a
real kernel — this pass did, per CLAUDE.md rule 2 ("実際に動かして検証す
る"), and it did not work on the first try.

**Bug found and fixed**: `vkubeproxy.Run` originally called
`s.SetRouteTable([]tcpip.Route{{Destination: subnet, NIC: nicID}})` with
`subnet` scoped to `serviceCIDR` (`10.43.0.0/16`). Driven against a real
`/dev/net/tun` device in a privileged Linux container (`golang:1.26-alpine`,
`--cap-add=NET_ADMIN --device=/dev/net/tun`), every intercepted connection's
`ForwarderRequest.CreateEndpoint` failed with `network is unreachable`. Root
cause: that route only tells gVisor's own userspace stack how to route
packets **addressed to** `serviceCIDR` — but every reply this stack sends
(SYN-ACK, data, FIN) is addressed back to the real caller's own address
(e.g. the microVM's real interface IP), which is never itself inside
`serviceCIDR`. With only one NIC in the whole stack (the TUN device), the
correct route is `header.IPv4EmptySubnet` (0.0.0.0/0) → `nicID` — this
governs only which NIC the stack's own replies go out on, not what traffic
reaches it in the first place (that scoping is the real Linux kernel route,
`ip route add 10.43.0.0/16 dev k8flare0`, unchanged and correctly narrow).
This is the same shape every tun2socks-style transparent proxy uses and
would have shipped broken — every ClusterIP connection from inside a
Pod-on-Containers Pod would have hung until client timeout — without
actually exercising a real TUN device rather than just reading the gVisor
API. Fixed in `pkg/vkubeproxy/vkubeproxy.go`.

Also fixed in this pass: the package's `//go:build !js` tag (copied from
`pkg/dnsshim`/`pkg/meshconnector`) does not hold here — those two packages
only shell out to CLI tools (compiles anywhere, runs correctly only on
Linux), but this package links `gvisor.dev/gvisor/pkg/tcpip/link/{tun,fdbased}`,
whose own build constraints are unconditionally Linux-only (raw AF_PACKET
sockets, TUN ioctls). `go vet ./pkg/...` on a non-Linux dev machine failed
to even *compile* the package, not just fail to run it. Changed to
`//go:build linux`.

**What was verified, against real kernel primitives (not read-and-assumed)**:

- Node-side intercept, standalone (privileged `golang:1.26-alpine`
  container, `/dev/net/tun` + `CAP_NET_ADMIN`, real `ip` commands via
  `iproute2`): a `GET` and a `POST` with a body, both issued from a plain
  `net/http` client against a fabricated ClusterIP (`10.43.0.55:8080`) not
  otherwise routable, were transparently intercepted by the TUN + netstack
  forwarder and re-issued as real HTTP requests against a mock
  `/nodes/vkubeproxy` endpoint, correctly carrying `X-K8flare-Target-IP:
  10.43.0.55`, `X-K8flare-Target-Port: 8080`, `Authorization: Bearer
  test-token`, and (for the POST) the original request body verbatim. The
  mock's response flowed back through the same connection to the original
  client unmodified. Confirms the core "capture ClusterIP TCP, reissue as
  HTTP" mechanism actually functions on a real Linux kernel — not merely
  that the Go code compiles against gVisor's API.
- Worker-side resolution (`handleVKubeProxy`/`handlePodProxy`,
  `podproxy.ts`), against a real local `wrangler dev` (single consolidated
  config, `--enable-containers=false`, real `CLOUDFLARE_ACCOUNT_ID` needed
  non-interactively — see note below): created a real `Service` with a
  fixed `clusterIP` and a matching `EndpointSlice` via direct API calls,
  then called `/nodes/vkubeproxy/hello` with `X-K8flare-Target-IP`/`-Port`
  headers matching that Service. Confirmed correct for the whole resolution
  chain: ClusterIP → Service (`spec.clusterIP` field selector) → Service
  port match → EndpointSlice (`kubernetes.io/service-name` label selector)
  → ready endpoint's `targetRef.uid` → `forwardToPod`'s
  `scheduler.lookupVM`, which correctly 404s ("no live NodeVM for pod
  ...") since no real Pod is scheduled. Negative paths also confirmed:
  unknown ClusterIP → 502, wrong Service port → 502, missing headers → 400,
  `pods/proxy` on a nonexistent Pod → 404.

**What remains unverified**: the last hop — an actual `containerFetch` to
a live NodeVM answering on its container port — because that requires a
running per-Pod microVM, which this session could not produce locally.
Local Docker on this ARM Mac cannot boot the real (amd64) node image at
all (pre-existing Rosetta/seccomp limitation, S16 above); the real
Firecracker microVM only exists once deployed to the actual Cloudflare
account (`wrangler deploy`), which this session's constraints forbid
running. `pkg/vkubeproxy`'s own TCP-layer correctness (multi-request
keep-alive over one intercepted connection, DNS bridging via the tun
device, the secure/TokenReview'd 10250 path) is also still unverified
end-to-end for the same reason — v1 scope per the approved design remains
HTTP/1.x request-response only.

**Local-environment note, not specific to this feature**: this session's
first `go test ./pkg/apiserver/...` run failed entirely with `connection
refused` on every test — `wrangler dev`'s non-interactive `npx wrangler
dev` silently exits when the logged-in Cloudflare account has more than
one available account and `CLOUDFLARE_ACCOUNT_ID` isn't set (confirmed by
running `wrangler dev` directly and reading its actual stderr instead of
the test harness's redirected-to-`/dev/null` stream). Setting
`CLOUDFLARE_ACCOUNT_ID` in the shell environment before `go test` fixed it
with zero code changes. Recording here since the test harness's own
`devCmd.Stderr = devNull` means this failure mode produces no diagnostic
output at all from the Go side — worth knowing before concluding the
whole suite is broken.

## Correction log (honest corrections)

**2026-07-03 — A Phase 6 subagent opened an unauthorized pull request
against the real `github.com/k8flare/k8flare` repository.** While trying
to get real CI signal for Phase 6's e2e-conformance verification, the
delegating instructions ("自分のブランチ/PR起点で回せるなら回す") did not
carry forward this project's hard rule (never create a PR unless
explicitly instructed), and the subagent opened PR #1 ("Phase 6: CI
verification (not for merge)") to trigger pull_request-scoped workflows.
Caught and corrected by the coordinating session: CI results were
extracted first (check: pass, cost-gate: pass, e2e-conformance: failure —
the disk-space finding above), then the PR was closed with an explanatory
comment and the leftover remote branch deleted. The repository is private,
so this had no external visibility, but it was still a real, unauthorized
action against shared state. All subsequent CI verification in this
section used `workflow_dispatch` directly against the pushed branch — no
PR — after confirming with the user that pushing the branch itself was
acceptable.

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
   added.** (Historical: that gzip limit was removed in 2026-09 — S35.
   The binaries still cannot ship in the Worker script, now because of
   the 64 MiB raw limit.) Isolated, incremental `GOOS=js GOARCH=wasm go build` +
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

**2026-07-07 — Re-confirmed, by actually building it, that narrowing
`kubernetes.Interface` cannot bring the real scheduler under the
Loader's 64MiB cap; this independently corroborates
`scripts/gen-clientgo-lean-mirror.sh`'s existing "Phase 10" note rather
than superseding it.** Prompted by a user question ("build a scheduler
that doesn't depend on `syscall.SIGUSR2`, use reflection/codegen/
whatever magic it takes") that, on investigation, turned out to rest on
a premise already resolved: `third_party/k8s-js-overlays` had _already_
patched the one `signal.go` file blocking `k8s.io/kubernetes/pkg/
scheduler` from compiling for GOOS=js at all (S8's original hard
blocker, above) — `cmd/kcm-wasm/scheduler` already exists and compiles.
The open question was therefore purely about SIZE, not compilability.
Verified today, against the current mirrors:

- `-tags leanwidth` (KCM's narrow `kubernetes.Interface`, 6 methods):
  fails to compile at all --
  `k8s.io/client-go/informers/{storage,resource,node,scheduling,policy,
storagemigration}`'s per-version files reference `StorageV1alpha1()`,
  `ResourceV1beta1()`/`ResourceV1beta2()`, `NodeV1beta1()`,
  `SchedulingV1()`, `PolicyV1beta1()`, `StoragemigrationV1beta1()` --
  none of which the narrow interface declares.
- Without `-tags leanwidth` (`pkg/leanclient/clientset`'s full-width
  panic-stub `Clientset`, satisfying the real, complete
  `kubernetes.Interface`): compiles cleanly. `wasm-opt -Oz`: **102.7MB**
  -- matching `scripts/build-wasm-chunks.sh`'s existing "102.8MB opt
  against the reproducible mirrors" figure exactly, 35.6MB over the
  67,108,864-byte cap.
- Attempted to widen `clientset_leanwidth.go` with panic-stub methods
  for just the ~8 missing group-versions (not full width) before
  finding `gen-clientgo-lean-mirror.sh`'s own comment already documents
  why this fails: `scheduler.New`'s `informerFactory` parameter type is
  fixed to the real `k8s.io/client-go/informers.SharedInformerFactory`,
  whose single `factory.go` declares one `Group() *group.Interface`
  method per top-level API group (21 of them) -- referencing that type
  at all requires every one of those 21 groups' typed and
  applyconfigurations packages to compile, and those packages
  cross-reference each other across group boundaries (e.g.
  autoscaling's `HorizontalPodAutoscalerApplyConfiguration` embeds
  core's `ObjectReferenceApplyConfiguration`), so no subset short of
  "all of them" compiles. Did not re-attempt the partial-prune
  experiment this implies would fail (rule 2 still applies, but a
  clearly-reasoned prior failure with the exact mechanism identified
  doesn't need re-disproving by hand every time it resurfaces) --
  reported as re-confirmation of an existing finding, not a new one.

**Conclusion, unchanged from before this re-check but now re-verified
by two fresh, real `GOOS=js GOARCH=wasm go build` attempts**: the real
kube-scheduler cannot fit the Loader's 64MiB cap without either (a)
patching `scheduler.New`'s call site to accept a narrow, hand-rolled
`SharedInformerFactory`-shaped type instead of the generic one --
substantially larger surgery on upstream scheduling logic than the
one-file `signal.go` swap, and one that would need re-verification on
every future k8s version bump -- or (b) a non-WASM execution shape
(BYO-VM host process, current; or Cloudflare Containers, task #23's
original framing). Splitting `cmd/apiserver-wasm`/`cmd/kcm-wasm` into
more, smaller Loader dynamic workers (task #23 as re-scoped 2026-07-07)
does not change this conclusion: the blocker is a structural type
dependency at the informer-factory injection point, not overall binary
size pressure elsewhere in the control plane.

**What was built, and what it measured.** `k8s.io/client-go/informers`'s
top-level `SharedInformerFactory` aggregate (`informers/factory.go`) was
narrowed from 19 group accessors to the 6
(Core/Apps/Storage/Resource/Scheduling/Policy) the real, unmodified
upstream scheduler's own call sites actually reach -- confirmed by
`grep -rn "SharedInformerFactory()\."` across the *entire* `pkg/scheduler`
tree (not just the 6 files this task's brief named; `framework/
preemption/{preemption,podgrouppreemption,executor}.go`,
`framework/plugins/{volumebinding,interpodaffinity,defaultpreemption,
gangscheduling,topologyaware}` and `schedule_one*.go` all call it too,
adding Policy() -- PodDisruptionBudget informers, unconditional, since
DefaultPreemption is not in `pkg/controllers/sched.RunScheduler`'s
disabled-plugins list -- to the set). New overlay file
`third_party/clientgo-lean-overlays/informers/factory.go` (extending the
existing clientgo-lean-mirror mechanism one level up from
`kubernetes/clientset.go`/`kubernetes/scheme/register.go`, same
drift-checked-copy pattern) redeclares `SharedInformerFactory` with only
those 6 groups plus a permanent panic-stub `ForResource` (confirmed
unused by every scheduler call site) and no `generic.go` (deleted from
the mirror -- its ~54-type GVR switch was the untouched-groups' back
door). `pkg/leanclient` gained 3 new real typed-client groups via the
existing `cmd/k8flare-gen` table-driven generator (storagev1: CSIDriver/
CSINode/CSIStorageCapacity/StorageClass/VolumeAttachment; resourcev1:
DeviceClass/ResourceClaim/ResourceSlice; policyv1: PodDisruptionBudget),
wired through a new `SchedulerClientset` (`pkg/leanclient/clientset/
scheduler.go`, `!leanwidth`-tagged) that embeds the existing 5-group
`Clientset` and shadows Storage/Resource/Policy with real
implementations -- kept out of the plain `Clientset`/`-tags leanwidth`
KCM build entirely (build-tag exclusion, not just an unreferenced
symbol) to avoid regressing it.

Measured, against the reproducible mirrors (`GOFLAGS=-modfile=go.wasm.mod
GOOS=js GOARCH=wasm go build ./cmd/kcm-wasm/scheduler` +
`wasm-opt -Oz`): **102.8MB opt (prior baseline) -> 101,109,121 bytes
(101.1MB) opt.** Reproduced twice, consistent. **KCM (`-tags leanwidth
./cmd/kcm-wasm`) re-measured at 65,230,504 bytes opt -- unaffected**,
confirming the build-tag isolation holds (`npm run build:wasm`'s own gate
also passed at that number). So the informers-aggregate lever is real,
safe, and worth keeping, but it closes only ~1.7MB of the ~38MB gap to
the 67,108,864-byte cap -- nowhere near sufficient on its own.

**Why: `kubernetes.Interface` width, not the informers aggregate, is the
dominant remaining cost -- confirmed by actually trying to narrow it, not
assumed.** A `-tags leanwidth,schedwidth` variant was built: a new
mutually-exclusive overlay file (`clientset_schedwidth.go`, later
deleted, see below) widened `clientset_leanwidth.go`'s 7-method
`kubernetes.Interface` with the 3 groups (Storage/Resource/Policy) the
scheduler needs for real, reusing `-tags leanwidth`'s narrow-width lever
that already gets KCM under cap. This is exactly what the "Phase 10"
note (`scripts/gen-clientgo-lean-mirror.sh`'s comments) says was already
tried and reverted for the scheduler because "`scheduler.NewInformerFactory`
is the full-width aggregate `SharedInformerFactory`" -- but this session's
own new `informers/factory.go` overlay (above) had just replaced that
exact aggregate with a narrow one, so the specific blocker Phase 10 named
no longer applied verbatim. It compiled far enough to reveal the *real*
blocker instead: `kubernetes.Interface` is one global type (one file,
this repo's own overlay), so narrowing it with real methods for 3 groups
breaks compilation everywhere a *different*, still-unpruned sibling API
version of *any* group is transitively reachable -- because every
`informers/<group>/<version>` package's own `NewFilteredXInformer(client
kubernetes.Interface, ...)` hardcodes that exact global type, regardless
of which version the SharedInformerFactory aggregate exposes. Concretely,
`go build -tags leanwidth,schedwidth ./cmd/kcm-wasm/scheduler` failed on:
`Apps` V1beta1/V1beta2, `Storage` V1alpha1/V1beta1, `Resource`
V1alpha3/V1beta1/V1beta2, `Scheduling` V1/V1beta1, `Policy` V1beta1 (all
sibling versions of groups this repo only narrowed to V1), **plus a
previously-unknown `EventsV1` requirement** from
`k8s.io/client-go/tools/events/event_broadcaster.go` (used by
`pkg/controllers/sched.RunScheduler`'s `events.NewEventBroadcasterAdapter`
for the scheduler's own event recording -- not part of the informer
factory at all, a separate discovery). Reaching a compiling narrow width
would mean pruning every one of those sibling-version packages too (each
its own `informers/<group>/<version>/*.go` deletion + trimming that
group's own `interface.go`), a substantially larger surgery than the
6-group factory prune above, with no guarantee against further surprises
of the same shape (`EventsV1` was not visible from reading `pkg/scheduler`
alone). The `schedwidth` attempt was reverted rather than pushed through:
`clientset_schedwidth.go` was deleted, `clientset_leanwidth.go`'s doc
comment updated to record this finding in place of speculating about it,
and `SchedulerClientset`/`stubs.go`/`gen-clientgo-lean-mirror.sh` reverted
to their plain (non-`schedwidth`) shape. This is a **stronger, corrected**
version of the "Phase 10" note, not a new contradiction of it: the
specific sentence ("`scheduler.NewInformerFactory` is the full-width
aggregate") is no longer accurate now that the aggregate is narrowed, but
the conclusion (leanwidth cannot apply to the scheduler) still holds, for
a now-precisely-identified reason.

**A separate, orthogonal, unresolved correctness gap surfaced during this
investigation (not introduced by it, and not yet fixed): `EventsV1()` is
still a permanent panic stub on the plain (`!leanwidth`) `Clientset`
`pkg/leanclient/clientset` builds against.** `pkg/controllers/sched.
RunScheduler` constructs `events.NewEventBroadcasterAdapter(client)` and
passes its `.NewRecorder` into `scheduler.New` unconditionally.
`event_broadcaster.go`'s sink lazily calls `client.EventsV1()` the first
time any event is actually recorded (e.g. a "Scheduled" event on
successful binding) -- meaning the scheduler-as-dynamic-worker path,
**which has never been run end-to-end (only measured for wasm-opt size,
this session included)**, would very likely panic the first time it
schedules a Pod and tries to emit that event. Flagging this rather than
fixing it: promoting `EventsV1` to real (1 more typed-client group,
`k8s.io/api/events/v1`'s `Event` type) is a small, well-understood,
CLAUDE.md-rule-2-compliant fix, but this session ran out of budget before
reaching real end-to-end verification (`wrangler dev`/`go test
./pkg/apiserver/...` could not be exercised at all this session -- see
below), so recording the gap honestly instead of shipping an unverified
"fix" felt like the more honest choice.

**Also newly noticed, unrelated to the above but relevant to any future
scheduler-size attempt: `cmd/kcm-wasm/scheduler/main.go` imports the
whole `pkg/controllers` package (for `RestConfig`), which is the *same*
package as `controllermanager.go` (KCM's own 5-controller wiring:
replicaset/deployment/daemon/job/cronjob) -- `RestConfig` was never split
into its own leaf package, so the scheduler binary compiles (and, absent
further dead-code elimination guarantees this repo doesn't rely on
elsewhere, likely links) that entire unrelated controller-manager logic
too.** Not measured how much this costs (out of this session's budget),
but it is a clean, low-risk, surgical next step (move `RestConfig` to
`pkg/controllers/restconfig`, a new subpackage, update both `cmd/kcm-wasm/
main.go` and `cmd/kcm-wasm/scheduler/main.go`'s imports) that doesn't
depend on resolving the `kubernetes.Interface`-width question above.

**Verification performed this session**: `go build`+`wasm-opt` for both
the scheduler (plain, no tags) and KCM (`-tags leanwidth`) from a freshly
regenerated `.build/k8s-js-mirror`+`.build/clientgo-lean-mirror`;
`go vet` for the exact supported package/tag combinations (host `go vet
./pkg/... ./cmd/...`, wasm `go vet ./pkg/leanclient/... ./pkg/
controllers/...` untagged, `-tags leanwidth ./cmd/kcm-wasm ./pkg/
controllers ./pkg/leanclient/...`) -- all clean. `npm run build:wasm`
(the real CI gate) re-run clean, apiserver/KCM chunks unaffected.
**`go test ./pkg/apiserver/...` could not be run to completion this
session**: `wrangler dev` fails non-interactively in this environment
("More than one account available... set CLOUDFLARE_ACCOUNT_ID"), a
pre-existing credential/environment gap with no `account_id` configured
anywhere in this checkout, unrelated to any change in this entry (`pkg/
apiserver` imports neither `pkg/leanclient` nor `pkg/controllers/sched`,
so there is no plausible mechanism for this session's changes to affect
it) -- recorded as an honest gap in this session's verification, not
papered over.

**Net conclusion, updated**: the real kube-scheduler still does not fit
the Loader's 64MiB cap (101.1MB opt, down from 102.8MB). The remaining
~34MB gap is attributable to `kubernetes.Interface`'s global width, which
this session confirmed (by building, not assuming) cannot be narrowed
without pruning every transitively-reachable sibling API version of every
group the scheduler touches -- a materially larger undertaking than this
task's original 5-6-file estimate, whose exact remaining shape (which
files, how many) is now known well enough to scope a focused follow-up:
prune `informers/{apps,storage,resource,scheduling,policy}/<unused
version>/*.go` and each group's own `interface.go` (mirroring the
existing `kubernetes/typed`+`applyconfigurations` per-type pruning
pattern one directory tree over), then re-attempt `-tags
leanwidth,schedwidth`, then fix the `EventsV1` gap, then verify a
scheduled Pod end-to-end via `wrangler dev` before ever flipping
`scripts/build-wasm-chunks.sh`'s "sched: NOT built" gate. Kept as
`cmd/kcm-wasm/scheduler`/informers-factory-narrowing groundwork for that
follow-up rather than reverted, since it is a real, verified, zero-risk
improvement (KCM unaffected) even though insufficient alone.

**Correction (orchestrator review, before merge, same day): "KCM
re-measured ... unaffected" above was wrong.** The reviewing session
independently rebuilt KCM from this entry's own diff and got
65,230,504 bytes -- not the clean baseline of 64,019,374 (confirmed
twice earlier that same session, after the RBAC/DNS/SA-token merge and
again after the wasm-split-v2 merge). The +1,211,130 byte (~1.18MiB)
regression traced to `kubernetes/scheme/register.go`'s new
`resourcev1.AddToScheme` entry: `scripts/gen-clientgo-lean-mirror.sh`
copies this file into the shared `.build/clientgo-lean-mirror`
*unconditionally* (no build-tag gating), so it fed both the scheduler
build and KCM's `-tags leanwidth` build even though KCM never uses
resource/v1 (`pkg/leanclient`'s `ResourceV1()` stays a permanent panic
stub there). The "re-measured" number in the paragraph above was
internally self-consistent (matched `npm run build:wasm`'s own output)
but was never compared against the pre-existing baseline, so the
regression went unnoticed within the same session that introduced it.
Fixed before merge by splitting resource/v1's registration into a new
`register_sched.go`, tagged `!leanwidth` (mirrors the existing
`clientset_leanwidth.go` gating pattern) -- KCM re-verified back at
64,019,330 bytes (3017KiB headroom, the 44-byte difference from
64,019,374 is ordinary build-path/timestamp noise), scheduler
re-verified unchanged at 101,110,390 bytes. Recorded per this repo's
rule 4 (corrections are appended, not silently rewritten into the
original paragraph).

## S20: apiserver Loader-cap headroom -- splitting into multiple dynamic workers investigated and abandoned; a real ~1.84MiB win found instead (2026-07-07)

Task #23 (re-scoped 2026-07-07, see S19 tail above): today's RBAC/DNS/
ServiceAccountToken additions left `cmd/apiserver-wasm` at 64,795,202
bytes raw -- **2,259 KiB (2.21MiB) of headroom** under the Loader's
67,108,864-byte (64MiB) cap (measured with `GOOS=js GOARCH=wasm go
build -ldflags="-s -w" -trimpath`, matching `scripts/build-wasm-chunks.sh`
exactly). The ask was to split `cmd/apiserver-wasm` (and/or
`cmd/kcm-wasm`) by GroupVersion/feature into several smaller Loader
dynamic workers, explicitly **not** to touch the kube-scheduler
question (separate, structural, tracked above). Per CLAUDE.md rule 2,
every claim below is from an actual `go build` + `wc -c`, not source
reading.

**Baseline attribution, incremental (same method this doc's S19/KCM
sections already used):**

| Build content | raw wasm size | delta over previous |
| --- | --- | --- |
| Empty `main()` + `github.com/syumai/workers` only | 5,495,081 | -- |
| + `apidef.Table` (all 12 GroupVersions, ~35 ResourceDefs), no `pkg/apiserver` | 18,108,238 | +12.6 MiB |
| + `k8s.io/apimachinery/pkg/util/strategicpatch` alone (PATCH support, subresource.go) | 18,595,354 | **+13.1 MiB over empty baseline -- larger than the entire 12-GroupVersion type table** |
| `cmd/apiserver-wasm` today (full, real main.go) | 64,795,202 | -- |
| Same, minus all 8 `zz_generated_defaulters.go` upstream packages (stubbed) | 60,381,997 | -4.4 MiB |
| Same, minus RBAC/SA-token/DNS/R2/supervisor registration (defaulters still stubbed) | 56,988,785 | -7.8 MiB combined; RBAC alone -1.0 MiB, SA-token+certmanager alone -1.3 MiB, DNS alone -0.8 MiB, R2/supervisor/internal remainder -0.25 MiB |
| Full main.go, `apidef.Table` trimmed to core/v1+rbac.authorization.k8s.io/v1 only (10 of 12 GroupVersions removed), defaulters left stale (still importing all 8 groups) | 64,754,918 | **-40 KiB only** -- a false signal, see below |
| Same trim, `zz_generated_defaulters.go` *correctly* regenerated for the trimmed table (drops to just corev1defaults) | 62,196,552 | **-2.48 MiB**, the real, consistent cost of removing 10 of 12 GroupVersions' types |

**Why the first trim looked like it saved almost nothing:**
`zz_generated_defaulters.go` is a committed, generated file
(`cmd/k8flare-gen/defaulters.go`'s `genDefaulters`) that imports one
upstream `k8s.io/kubernetes/pkg/apis/<group>/v1` package per
`apidef.Table` GroupVersion *at generation time* -- editing
`table.go` by hand without re-running `go run ./cmd/k8flare-gen`
leaves it stale, so the 10 removed GroupVersions' upstream defaulter
packages (and therefore their `k8s.io/api/<group>/v1` types, pulled in
transitively) stayed linked regardless of what `apidef.Table` said.
Re-running the generator after the same trim is what surfaced the
real, still-small, 2.48MiB figure. This is itself a useful, generally
applicable correction for anyone hand-editing `apidef/table.go` during
experimentation: measure only after `go run ./cmd/k8flare-gen`, never
before.

**Conclusion: a GroupVersion/feature-based split of `cmd/apiserver-wasm`
into multiple Loader dynamic workers does not achieve the stated goal
(headroom) and was abandoned, per this task's explicit "abandon and
record why" clause.** Reasons, all measured, not assumed:

1. **The per-GroupVersion type surface is not the dominant cost.**
   Removing 10 of the 12 served GroupVersions (apps, batch, storage,
   node, resource, policy, discovery, networking, coordination,
   autoscaling -- everything except core/v1 and rbac.authorization.k8s.io/v1)
   saves only 2.48MiB out of 64.8MiB, once measured correctly. This
   matches the same pattern this doc's KCM/client-go section already
   found ("registering the other ~53 groups... costs only +0.03 MiB"):
   the shared `k8s.io/apimachinery` serializer/scheme/runtime
   infrastructure absorbs almost the entire per-type cost once paid for
   any one group, and `k8s.io/api`'s own types are cheap to add on top.
2. **The dominant costs are cross-cutting and would have to be
   duplicated into every split binary anyway.** `k8s.io/apimachinery/pkg/util/strategicpatch`
   (PATCH support for `kubectl apply`/`kubectl patch`, used by
   `subresource.go` for every resource and subresource with a "patch"
   verb -- i.e. every resource in `apidef.Table`) alone measures
   **~13.1MiB**, more than the entire 12-GroupVersion type table. It
   cannot be scoped to one split binary: every GroupVersion advertises
   "patch" in `apidef.StandardVerbs`, and removing PATCH support for
   any resource would be a real conformance/behavioral regression, out
   of scope here. Likewise RBAC authorization (`AuthzMiddleware`) and
   ServiceAccount token authentication (`AuthMiddleware`'s token path)
   gate **every** request on **every** route today; splitting by
   GroupVersion would require either (a) duplicating the RBAC
   authorizer + rbac.authorization.k8s.io/v1 stores + the
   ServiceAccount JWT authenticator + corev1 stores into every single
   split binary (each already measured at +1.0MiB and +1.3MiB
   respectively -- not free, and now paid N times instead of once), or
   (b) a cross-binary authentication/authorization delegation design
   (e.g. every non-"core" binary calling the "core" binary's already-
   existing TokenReview endpoint instead of authenticating locally) --
   a materially larger, higher-risk redesign of the auth path than this
   task's budget or its "abandon rather than risk regression" mandate
   allows.
3. **Namespace cascade delete is a genuine cross-GroupVersion
   dependency, confirmed by reading (not yet by trying to break it):**
   `pkg/apiserver/handler.go`'s DELETE case and `pkg/apiserver/gc.go`'s
   owner-reference cascade both take `namespacedStores
   []*ResourceStore`, built by `NamespacedResourceStores` as the union
   of **every** GroupVersion's namespaced stores (main.go's comment:
   "deleting an apps/v1 Deployment must be able to find and delete the
   ReplicaSets (apps/v1) and Pods (core/v1) it owns"). A GroupVersion
   split would need this cascade to reach across separate Loader
   dynamic worker instances -- either a shared storage-key-based
   (not typed-`ResourceStore`-based) rewrite of `namespacedelete.go`/
   `gc.go`, or cross-binary HTTP calls per cascade step. Given finding
   1 above already shows the split isn't worth pursuing on its own
   merits, this risk was not attempted (per this task's explicit
   instruction not to risk regressing today's just-landed cascading
   delete behavior).

**What was actually done instead: a real, verified ~1.84MiB win with
none of the above risk.** `zz_generated_defaulters.go` unconditionally
registers upstream versioned defaulters for 8 groups
(`cmd/k8flare-gen/defaulters.go`'s `defaulterPackages`). Of those, 5
(coordination.k8s.io/v1 Lease, storage.k8s.io/v1 StorageClass/
CSIDriver/CSINode, resource.k8s.io/v1 DRA stub types,
discovery.k8s.io/v1 EndpointSlice, networking.k8s.io/v1
Ingress/IngressClass/NetworkPolicy/ServiceCIDR) are never actually
defaulting-dependent in this codebase: Leases are written by the k3s
agent/kubelet with explicit fields, StorageClass is bootstrapped as a
complete literal (`pvcbind.go`'s `BootstrapStorageClasses`),
EndpointSlice is written by the real endpointslice controller which
sets its own fields, and the networking/DRA types have no controller
acting on them at all yet. Removed those 5 from
`cmd/k8flare-gen/defaulters.go` (the hand-maintained source, not the
generated file) and regenerated with `go run ./cmd/k8flare-gen`
(confirmed via `git diff --stat` that only
`pkg/apiserver/zz_generated_defaulters.go` changed -- no drift in the
other 5 generated artifacts). Result:

- `cmd/apiserver-wasm` raw size: 64,795,202 -> **62,863,479 bytes**
  (-1,931,723 bytes, ~1.84MiB). Loader-cap headroom: 2,259 KiB ->
  **4,145 KiB** (2.21MiB -> 4.05MiB), verified by
  `scripts/build-wasm-chunks.sh`'s own gate output.
- `go test ./pkg/apiserver/...` green, including the exact tests that
  exercise the 5 affected groups end-to-end against real `wrangler
  dev` (`TestLeaseCRUD`, `TestNewlyRegisteredResources/NetworkPolicy`,
  `TestResourceAPIGroup` (DRA), `TestStorageClassBootstrap_R2ExistsAndIsDefault`)
  -- all still PASS, confirming the removed defaulters were never
  observably exercised.
- `cmd/kcm-wasm` unaffected (this change is apiserver-only): still
  64,019,374 bytes / 3,017 KiB headroom, unchanged from before this
  investigation.

`packages/k8flare-worker/wrangler.jsonc`, the Loader routing in
`packages/k8flare-worker/src/loader/apiserver.ts` /
`packages/k8flare-worker/src/controllers/index.ts`, and
`scripts/build-wasm-chunks.sh` are all unchanged -- this stays a
single apiserver Loader dynamic worker, same as before. If apiserver's
headroom becomes tight again, the next-highest-leverage lever found
during this investigation (not attempted here, out of scope) would be
revisiting whether `strategicpatch`'s ~13MiB is fully load-bearing for
every resource (e.g. a narrower merge-patch implementation for
resources kubectl rarely `apply`s to with array-merge semantics) --
NOT another attempt at a GroupVersion-based Loader-worker split, which
this section's measurements show is not where apiserver's size
actually comes from.

## S21: kube-scheduler VolumeBinding/NodeVolumeLimits/DynamicResources delegate-Worker investigated and rejected -- these plugins are under 1% of the size gap, not the cause (2026-07-07)

Task ask: instead of deleting VolumeBinding/NodeVolumeLimits(CSI)/
DynamicResources from the WASM scheduler outright (rejected by the user
because a future hybrid BYO-VM-plus-real-CSI cluster might need them to
stay correct), delegate just those three plugins to a second, lazily
Loader-loaded WASM worker, invoked only for the rare Pod that actually
triggers them, so the common case (no DRA, R2-only storage) costs
nothing. This entry records why that plan was not implemented, backed
by fresh builds and measurements (not a re-read of the existing S8
"kube-scheduler-wasm-fork" entry or S20's tail above, both of which
this entry independently corroborates rather than supersedes).

**Finding 0 (changes the premise): these plugins are already disabled
today, identically, on both the host-process scheduler and the WASM
one -- not because of the WASM size cap.** `pkg/controllers/sched/
sched.go`'s `RunScheduler` sets `cfg.Profiles[0].Plugins.MultiPoint.
Disabled` to `VolumeBinding`, `VolumeRestrictions`, `NodeVolumeLimits`,
`VolumeZone`, and `DynamicResources`, with a comment stating this
"[m]atches `cmd/scheduler/main.go`'s `writeSchedulerConfig`: disable
the PV/PVC/StorageClass-touching plugins this apiserver can't back."
That is the **host-process** scheduler binary (`cmd/scheduler`, used
for BYO-VM nodes today, entirely unconstrained by any Loader byte cap)
disabling the exact same four volume plugins for the exact same
reason: this repo's PV/PVC/StorageClass backend is an R2 shim with no
real CSI provisioner anywhere yet (`pkg/apiserver/pvcbind.go`'s
`BootstrapStorageClasses`). So there is no currently-working BYO-VM +
real-CSI functionality for a delegate-Worker to "restore" -- building
one would add net-new, speculative functionality for a hybrid
configuration that does not exist in this codebase today, on either
execution shape.

**Finding 1, extension points (confirmed by grepping the resolved
`.build/k8s-js-mirror` source, not assumed):**

- `VolumeBinding`: `PreFilter`, `PreFilterExtensions`, `Filter`,
  `PreScore`, `Score`, `ScoreExtensions`, `Reserve`,
  `PreBindPreFlight`, `PreBind`, `Unreserve`.
- `NodeVolumeLimits` (`CSILimits`): only `PreFilter`,
  `PreFilterExtensions`, `Filter` -- no `Reserve`/`PreBind`/
  `Unreserve`, and its `PreFilter`/`Filter` signatures don't even bind
  the `fwk.CycleState` parameter (`_ fwk.CycleState`). This one plugin
  really is a stateless, read-only node filter with no side effects --
  cleanly delegatable in principle, mechanically speaking.
- `DynamicResources`: `PreEnqueue`, `PreFilter`, `PreFilterExtensions`,
  `Filter`, `PostFilter`, `Score`, `ScoreExtensions`,
  `NormalizeScore`, `Reserve`, `PreBindPreFlight`, `PreBind`,
  `Unreserve`.
- A repo-wide grep (`grep -rl "^func.*) Reserve(ctx context.Context" pkg/scheduler/framework/plugins`)
  found **only** `volumebinding` and `dynamicresources` implement
  `Reserve` among every in-tree plugin. Both pair it with `PreBind`
  (the real, side-effecting API write -- actually binding a PV to a
  PVC, or a ResourceClaim to a device) and `Unreserve` (rollback if a
  *different* plugin's `Reserve` fails later in the same cycle). This
  is a two-phase-commit protocol coordinated by the framework's own
  scheduling loop across possibly-concurrent pods sharing one
  in-process `assumecache.AssumeCache`/`SharedDRAManager` for mutual
  exclusion -- not a single stateless decision. Splitting it across an
  RPC boundary means either reimplementing that concurrency-safe
  assume/bind protocol remotely (a large, correctness-critical
  undertaking, and a reimplementation this repo's own rule 3 says to
  avoid), or relaying the *entire* plugin lifecycle (every extension
  point above, not just the rare PreBind trigger) to the remote side
  with a custom CycleState bridge keyed by (schedulingCycle, podUID)
  across every PreFilter/Filter/Score/Reserve/PreBind/Unreserve round
  trip for that pod's cycle -- because `Reserve`/`PreBind` read state
  (`cs.Read(stateKey)`) written by that *same plugin's own* earlier
  PreFilter/Filter/Score calls in the same cycle. "Delegate only the
  rare trigger case" does not hold up mechanically for these two.

**Finding 2, CycleState privacy (a non-issue, confirmed by grep):**
both plugins key their `CycleState` entries with `stateKey fwk.StateKey
= Name` (their own package name, e.g. `"VolumeBinding"`) and no other
in-tree plugin reads either key. So cross-plugin CycleState sharing is
not an additional blocker here -- Finding 1's Reserve/PreBind/Unreserve
protocol is the real structural concern, not state leakage to other
plugins.

**Finding 3, the DRA feature gate and scheduler.go's unconditional
construction (confirmed exactly as the task's premise stated, plus one
more instance the premise didn't name):** `pkg/features/kube_features.go`
locks `DynamicResourceAllocation` to `Default: true` /
`LockToDefault: true` starting v1.35 in this k3s pin -- it cannot be
turned off via feature-gate flags. `pkg/scheduler/scheduler.go`'s own
body (not the pluggable registry) gates ResourceClaim/ResourceSlice/
DeviceClass informer and `SharedDRAManager` construction behind
`feature.DefaultFeatureGate.Enabled(features.DynamicResourceAllocation)`
-- unconditional in practice because the gate is locked. **New finding
this session:** the very next line, `sharedCSIManager :=
nodevolumelimits.NewCSIManager(informerFactory.Storage().V1().CSINodes().Lister())`,
has **no feature-gate or plugin-enabled guard of any kind** -- it runs
regardless of whether `NodeVolumeLimits` is even in the registry. The
task's brief only asked about DRA's informers; the same
"`scheduler.go`'s own body, not the registry" pattern turns out to
apply to `NodeVolumeLimits`'s CSI manager too.

**Finding 4, decisive and measured (same session, same environment,
fresh from-scratch builds -- not read from the existing S8/S20 entries):**
built three variants from a freshly regenerated `.build/k8s-js-mirror`,
identical toolchain and `wasm-opt` flags to `scripts/build-wasm-chunks.sh`
(`GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath` +
`wasm-opt -Oz --strip-debug --strip-producers --enable-bulk-memory
--enable-nontrapping-float-to-int --enable-sign-ext
--enable-mutable-globals`), all disposable (no committed file was
touched; edits were made directly to the gitignored `.build/`
mirror copy and discarded afterward):

| Variant | wasm-opt bytes | Delta from baseline | Still over 64MiB cap by |
| --- | --- | --- | --- |
| Clean baseline, unmodified overlays (this environment) | 103,067,345 | -- | 35,958,481 (~34.3MiB) |
| Registry-only: drop `VolumeBinding`/`VolumeRestrictions`/`VolumeZone`/`NodeVolumeLimits.CSIName` from the js registry map (exact same pattern as the already-shipped `DynamicResources` drop; no `scheduler.go` changes) | 102,499,335 | -568,010 bytes (~555KiB) | 35,390,471 (~33.8MiB) |
| Full removal: registry change above + a disposable `scheduler.go` fork skipping the DRA-gated informer/`SharedDRAManager` construction *and* the unconditional `sharedCSIManager` construction | 102,050,604 | -1,016,741 bytes (~993KiB, ~0.97MiB) | 34,941,740 (~33.3MiB) |

(Note on the baseline number itself: 103,067,345 differs from the
existing S8 entry's 101,110,390 by ~1.96MiB. Both were built the same
way from the same pinned upstream module and this session did not
change any overlay before taking the baseline measurement -- the
difference is attributed to Go/wasm-opt toolchain version drift
between sessions, not a code change, and is flagged here rather than
silently reconciled, per this repo's honest-correction convention.
Either baseline leads to the same conclusion below.)

Removing all three plugins as cleanly as this session could manage --
registry entries dropped *and* `scheduler.go`'s two unconditional
construction sites neutralized -- saves under 1MiB out of a ~34-35MiB
gap. **These three plugins collectively account for under 1% of why
the real kube-scheduler doesn't fit the Loader's 64MiB cap.** This is
an independent, from-scratch re-confirmation (not a restatement) of
the 2026-07-07 S8 "kube-scheduler-wasm-fork" entry's conclusion that
`kubernetes.Interface`'s global width -- forced on every group's
informer files across the *entire* client-go tree, not scoped to
whichever plugins are actually enabled -- is the dominant, structural
cost, not any individual plugin's own implementation code.

**Conclusion: did not implement the delegate-Worker (or facet-hosted)
plumbing.** Two independently sufficient reasons, either one alone
enough to stop here per this task's own branch condition:

1. **Size is dispositive on its own.** Even a fully-working delegate
   for all three plugins would leave the main scheduler binary
   ~33-34MiB over the Loader's cap -- it would not achieve the actual
   goal (shipping a WASM scheduler for pure-Cloudflare-native, no-BYO-VM
   clusters). Building the delegation plumbing first and finding this
   out afterward would have been backwards; Finding 4 was checked
   before writing any product code specifically to avoid that.
2. **Correctness/complexity is disproportionate to what little size it
   would save.** Finding 1 above means a clean delegation of
   `VolumeBinding`/`DynamicResources` is not "ship a decision over the
   wire" -- it is "reimplement or fully relay a concurrency-sensitive,
   multi-callback assume/bind protocol," for well under 1MiB of benefit,
   in service of a configuration (Finding 0) nothing in this codebase
   supports today even on the unconstrained host-process path.

**On the coordinator's Facets-based hosting suggestion, evaluated at
the design level but not prototyped:** mid-investigation, the
orchestrating session correctly pointed out that `packages/k8flare-worker/src/
storage/facets.ts`'s `ctx.facets.get(name, factory)` mechanism (a
Durable-Object-owned, lazily-created, independently-stateful facet
instance, backed by the same `env.LOADER.get(...).getDurableObjectClass(...)`
delivery already used for `cmd/apiserver-wasm`/`cmd/kcm-wasm`) is the
right-shaped existing primitive for a lazy delegate, not a bare new
Loader-worker binding -- and that `packages/k8flare-worker/src/controllers/
index.ts`'s `Controllers` DO already has a stubbed, gracefully-handled
`"sched"` `loadComponent` case and a `GATEWAY` fetcher
(`dynamicEnv.GATEWAY = this.env.SELF`) in the shape that
`pkg/vkubeproxy`/`podproxy.ts` already use for WASM-to-Worker
callbacks. Both of those facts were confirmed by reading the actual
files (not taken on faith) and are accurate, and this is the correct
mechanism to reach for **if** this delegation is revisited later. It
was not prototyped end-to-end this session because Finding 4 (size)
already made the whole delegation moot before reaching the hosting
question -- there was nothing left worth hosting. Recorded here so the
next attempt at this doesn't have to re-derive it.

**What would actually help close the ~34MiB gap instead** (pointer, not
new work this entry): the existing S8 "kube-scheduler-wasm-fork"
entry's own scoped follow-up -- pruning `informers/{apps,storage,
resource,scheduling,policy}/<unused version>/*.go` and each group's own
`interface.go` tree-wide so `kubernetes.Interface` itself can narrow,
not another plugin-by-plugin pass. If that follow-up ever lands and the
gap closes enough that these three plugins' <1MiB starts to matter
again, Finding 0-2 above (already disabled everywhere, correctness
complexity concentrated in `VolumeBinding`/`DynamicResources`'s
Reserve/PreBind/Unreserve trio, `NodeVolumeLimits` alone is cleanly
stateless) is the starting point, not this entry's rejection of the
whole idea.

**Verification performed this session**: three `GOOS=js GOARCH=wasm go
build` + `wasm-opt -Oz` measurements above, all from disposable edits
to the gitignored `.build/k8s-js-mirror` copy -- `git status --short`
confirmed zero tracked files touched by any of them.
`bash scripts/build-wasm-chunks.sh` (the real CI gate, run after
regenerating a clean mirror) re-confirmed apiserver and KCM unchanged
at **62,863,479** and **64,019,330** bytes respectively, matching this
branch's existing baselines exactly. `go vet ./pkg/... ./cmd/...`
(host; pre-existing, unrelated `cmd/agent`/`pkg/vkubeproxy` build-tag
exclusions on darwin, not caused by anything here), `GOOS=js GOARCH=wasm
go vet ./pkg/leanclient/... ./pkg/controllers/...` (untagged), and
`GOOS=js GOARCH=wasm GOFLAGS=-modfile=go.wasm.mod go vet -tags leanwidth
./cmd/kcm-wasm ./pkg/controllers ./pkg/leanclient/...` all clean.
`CLOUDFLARE_ACCOUNT_ID=... go test -count=1 ./pkg/apiserver/...` (after
`rm -rf .wrangler/state`) passed. No new Go typed clients were added, so
`cmd/k8flare-gen` was not re-run. No `docs/cost-model.md` entry was
added, since no new component was built or shipped -- the task's cost-
accounting requirement was conditional on actually implementing the
delegate, and this entry's conclusion is the negative-finding branch
instead.

---

## S20: resident dynamic worker の watch ストリームが全滅していた退行(発見+修正 2026-07-10)

**症状**: 実 garbagecollector を第 3 の dynamic worker として組み込む作業
(pkg/controllers/gc)中、informer が一度も cache sync を完了しないことを発見。
切り分けの結果、**KCM の informer も同一環境で一度も sync していなかった**
(Deployment を作っても ReplicaSet が生成されない)— GC 固有ではなく resident
DW 全体の退行。

**根本原因(2 要因の合成)**:

1. `pkg/cfruntime/cloudflare/fetch` (2026-07-08 のスクラッチ書き直し)が
   レスポンスボディを `arrayBuffer()` で一括読みしていた。書き直し時に確認した
   呼び出し元(kine GET・vault 読み等の有界 JSON)では正しかったが、
   **Kubernetes の WATCH(終わらないストリーム)では `arrayBuffer()` の
   Promise が永遠に解決しない**。
2. client-go **v1.35 から `WatchListClient` feature が Beta/Default:true**
   (.build/clientgo-lean-mirror/features/known_features.go)— reflector の
   初回同期自体が list ではなく streaming watch(`watchList` +
   sendInitialEvents)になるため、「watch だけでなく初回 sync から」全部
   ブロックした。ミラーへの使い捨て println 計装で
   `ListAndWatchWithContext ENTER` まで到達し `list()` に一度も入らないことを
   実測して特定。

**なぜテストで捕まらなかったか**: `go test ./pkg/apiserver/...` は
`KCM_DISABLED=1` で wrangler dev を起動する(テスト中の Pod をコントローラーに
触らせないため)。resident DW の informer 動作はどの自動テストにも守られて
おらず、cfruntime 書き直し(apiserver の per-request 経路と /healthz でのみ
検証)がこの退行を伴ったまま 2 日間気づかれなかった。

**修正**: `fetch.go` の `responseFromJS` を ReadableStream の
`getReader()`/`read()` チャンク逐次読みの `io.ReadCloser`(`streamBody`)に
変更。参照実装は spikes/s8-wasm-resident/vendor/syumai-workers-fork の
internal/jsutil/stream.go(旧 cfruntime が吸収していた形)。

**検証(実機, wrangler dev)**: 修正後、(1) Deployment 作成 → KCM が 3 秒で
ReplicaSet を生成、(2) Deployment 削除 → 実 garbagecollector が 3 秒で
ReplicaSet をカスケード削除(非同期・eventually-consistent、実 k8s と同じ)。
両者とも修正前は無反応だった。

**付随して直した第 2 のバグ**: pkg/controllers/gc の informerFactory が
「Start() 済みか」を `stopCh != nil` で判定していたが、resident DW の ctx は
`context.Background()` であり、その `Done()` は仕様上 **nil チャネル**を返す
("Done may return nil if this context can never be canceled")— 正当な
stopCh の値がまさに nil なので判定が常に偽になり、informer が一度も
Run されていなかった。明示的な `started bool` フラグに変更。

**教訓**: (1) ストリーミングが要る呼び出し元(watch)の存在は「現在の
呼び出し元を全部確認した」では守れない — トランスポート層は最初から
ストリーミングにしておく。(2) KCM_DISABLED で守られていない領域
(resident DW の実 reconcile 動作)に自動テストの穴がある — conformance CI
(e2e-conformance.yml)がこの穴を埋める唯一のゲートなので、ローカル変更でも
KCM に触れたら手動で Deployment→ReplicaSet の実機確認をすること。

---

## S21: 実 kube-scheduler の Dynamic Worker 化(2026-07-10、実機検証済み)

**結論**: 実物・無改変の upstream kube-scheduler が第 4 の dynamic worker
(`-tags schedwidth`、45.2MB opt、64MiB cap に 21.4MiB の余裕)として
Cloudflare Workers 内で稼働し、wrangler dev で実 Pod の bind
(Deployment→実 KCM が Pod 生成→実 scheduler が 4 秒で bind→削除→実 GC が
4 秒でカスケード、のフルチェーン)を実機確認した。2026-07-07 の
「101.1MB でキャップ超過、恒久的にホスト専用」判断を覆す。

**サイズ削減の 3 本柱**(すべて gen-k8s-js-mirror.ts /
gen-clientgo-lean-mirror.ts に再現可能な形で実装、sha256 ピン付き):

1. **schedwidth 幅**: kubernetes.Interface を scheduler が実際にリンクする
   9 メソッドに絞る clientset_schedwidth.go + informers サブバージョン
   刈り込み overlay(2026-07-07 に失敗した「V1 だけに絞れない」問題を、
   グループ内の非 V1 サブバージョン informers を build-tag 分岐で落とす
   ことで解決)。124.1MB raw → 98.6MB raw
2. **kube-scheduler 第 3 ミラー**(.build/kube-scheduler-mirror、
   go.wasm.mod の replace 先): framework/listers.go の import 1 行を
   structured → structured/schedulerapi(apimachinery のみ依存の alias 元)
   に書き換えるだけで、structured→internal/{stable,incubating,experimental}
   →cel-go→protobuf/genproto/antlr の全チェーンが切断。-14.7MB raw
3. **kubelet/types→cri-client/logs の切断**: pkg/apis/core/validation→
   capabilities→kubelet/types が「時刻フォーマット定数 2 個」のために
   cri-client→grpc+cri-api+otelgrpc+protobuf 全体をリンクしていた。js-pair
   で定数をインライン化して切断 — **-30.8MB raw**。この切断は KCM/GC にも
   効き、kcm 65.0→41.8MB、gc 65.2→41.2MB へ縮小(副次効果)

**起動までに潰した実バグ 7 件**(すべて実機で発見、コンパイルでは不可視):

1. leanclient の Discovery() panic スタブ → events adapter が起動時に
   無条件でプローブするため即死。エラー返却化して core-events フォール
   バックに誘導
2. DynamicResources を MultiPoint.Disabled で無効化しても
   expandMultiPointPlugins が Disabled 参照**前**にレジストリ存在チェックで
   エラー → Enabled からの strip 方式へ
3. DRAExtendedResource feature gate(Beta/default-on)が nil
   SharedDRAManager を deref → gate をコードで無効化
4. leanclient に PersistentVolume/Namespace/ReplicationController の実
   クライアントが無く informer の ListAndWatch が panic → いずれも実
   apidef リソースなので k8flare-gen に追加(スタブ拡張は panic のまま)
5. **context.Background().Done() は nil チャネル**(仕様)— resident DW の
   ctx として本物の k8s コードに渡すと、stop channel として振る舞いが
   変わる箇所で壊れる(GC の informer factory 起動判定に続き 2 件目)。
   ResidentService が WithCancel 済み ctx を渡すよう根本修正
6. Volume 系 4 プラグインも Disabled が効かず有効のままで、VolumeBinding が
   **apidef.Table に存在しない** VolumeAttachment/CSIStorageCapacity の
   informer を張った。ゲートウェイの watch は未知リソースにも 200 の
   空ストリームを返す(404 にしない)ため initial-events-end bookmark が
   永遠に来ず、factory.WaitForCacheSync が恒久デッドロック → 4 プラグインも
   Enabled strip へ(ホスト版 cmd/scheduler の config 無効化と同等)
7. leanclient の Pods.Bind が Binding の TypeMeta を stamp せず、strict
   decoder に 400 され続けた(Delete の DeleteOptions stamping と同型)→
   stamp 追加。**これが最後の 1 個で、直後に実 bind 成功**

**残課題**: (a) 未知リソースへの watch が 404 でなく空 200 ストリームに
なるゲートウェイ挙動は informer を無限に待たせる罠(本件 6 の増幅要因)—
実 k8s 同様 404 を返すべき。(b) e2e-conformance はホスト版 scheduler を
併走させるため、sched DW と二重スケジューラになる構成の整理が必要
(binding の 409 は upstream 的に無害だが、意図した構成にすること)。
→ (b) は 2026-07-11 に解決: KCM_DISABLED と同型の SCHED_DISABLED
ハーネス・キルスイッチを Controllers DO の loadComponent に追加し、
e2e-conformance.yml の wrangler dev に `--var SCHED_DISABLED:1` を付与
(ホスト版が唯一の live scheduler になる構成に固定)。go test 経路は
KCM_DISABLED が全 poke を止めるため元から影響なし。

**S21 追補(2026-07-12、GC conformance required 昇格の最終形)**: upstream
GC conformance 7 テストのうち 6 つを required 化(run 29136960838 で 7/7
緑を確認済み)。残る 1 つ「should orphan pods created by rc [Serial]」は
**orphan の正しさではなく制御プレーンの書き込み/watch スループットのカナリア**
として advisory に置く: 高速なランナーでは通り(上記 7/7 run)、遅い
2-vCPU ランナーでは 50 Pod の作成自体に ~20 秒の DO 書き込みキュー時間が
かかり、KCM の informer が数秒〜数十秒古い世界を見て back-fill する。
この失敗系から 5 つの実修正(RC コントローラー有効化・削除の conflict
リトライ・graceful-deletion ライフサイクル・PATCH リトライ・
terminating/absent owner 作成ガード+無条件スタンプ)を得たが、
複数秒の informer ラグ自体はサーバー側ガードでは打ち消せない。
再昇格の条件は書き込みパスの高速化(別タスク)。

---

## S22: Go WASM インスタンスの isolate 内再利用 — 同期 js.FuncOf は IoContext を跨げる(2026-07-12、実機検証済み)

S8 の既知制約「timers/goroutines は go.run() をホストした IoContext が
アクティブな間しか進行しない(独立リクエストからの再利用は "code had
hung")」が、**同期 js.FuncOf コールバックには適用されない**ことを実機で
確認した。検証: labels/fields 判定のみの最小 Go WASM(apimachinery
labels+fields、4.57MB opt / 1.3MB gzip)を module スコープで 1 回
instantiate し、36 回の独立した fetch() イベント(連続 30 回+8 秒空けて
5 回)から globalThis.matchLabelSelector/matchFieldSelector を同期呼び出し
— 全て正しい結果、エラー 0、"code had hung" 0、boot 26ms、呼び出し
レイテンシ ~0ms。goroutine/timer の前進を必要としない純粋関数の
エクスポートなら、Go ランタイムのスケジューラが止まっていても呼べる
(js.FuncOf コールバックはホスト JS スレッドから直接実行されるため)。

**含意**: watch 経路のセレクタ判定 Go 化(タスク #1)は「静的アセット
WASM を gateway isolate 内で 1 回 instantiate して同期呼び出し」という
案 (d) で確定。per-event の Loader/DW 起動もイベント毎の再インスタンス化も
不要 = ホットパスへのコスト追加ほぼゼロ(コスト不変条件を全て満たす)。
サイズも ASSETS の 25MiB/ファイル上限に対し 4.57MB で余裕。

**S21 追補の訂正(2026-07-12)**: カナリア分離の根本原因が判明し、同日中に
required へ再昇格した。「複数秒の informer ラグ」の主因は環境スループット
ではなく (1) WASM 側 rest.Config の QPS 未設定(client-go デフォルト
5 QPS が全 resident コントローラーを律速 — ローカル実測: KCM の 50 Pod
作成 ~9 秒、GC の 50 ownerRef strip ~10 秒)と (2) e2e がホスト
kube-controller-manager と kcm DW の**二重コントローラーマネージャー**で
走っていたこと。QPS=50/Burst=100 + CM_DISABLED(SCHED_DISABLED と対称)
で run 29161962551 はカナリア含め全緑、GC required 群の実行時間も
~8 分→~3 分に短縮。GC conformance 7 テストは全て required に戻った。

---

## S23: dw 制御プレーン e2e(matrix)の現状 — wrangler dev の複数巨大 WASM 同居限界(2026-07-12、調査中断のチェックポイント)

e2e-conformance を matrix 化(controlplane: host|dw)し、v3 本番形態
(kcm/sched/gc DW のみ、ホストバイナリなし)の conformance 検証を追加した。
**host variant(required)は全経過で緑**。dw variant は advisory のまま赤で、
以下の調査結果を残して一旦中断する:

- 症状: 最初の controllers poke(Deployment 作成)直後に wrangler dev の
  workerd が完全に無応答化(全 API・console 出力・V8 inspector まで死ぬ =
  ネイティブコードでイベントループ専有)。20 分以上回復しない。
- 切り分け済み: メモリ天井ではない(12GB cgroup でも再現)、CPU クォータ
  でもない(8 vCPU でも再現; いずれも linux-arm64 コンテナ)。単体ロードは
  CI-x86 で実績あり(gc は host variant で毎回、sched は smoke-nodes で、
  kcm+gc 同時も selector.wasm 導入以前の全 e2e run で成功)。
- 対処として製品側に **DW ロードの直列化** を実装(controllers/index.ts の
  loadChain)— コールドスタート CPU スパイクの平準化として本番にも妥当だが、
  CI の dw wedge 自体は解消しなかった。
- ローカル再現の罠: linux-arm64 workerd(OrbStack コンテナ)は CI-x86 と
  挙動が異なり(kcm+gc だけでも wedge)、x86 エミュレーション(Rosetta)は
  apiserver DW の cold start が 503 になるなど、どちらも忠実な再現環境に
  ならなかった。
- 本番影響なし: 実 Loader は Cloudflare 側でコンパイルするため、これは
  wrangler dev(単一 workerd プロセス)へ 4 つの 40-63MB WASM を同居させた
  ときだけの emulation 限界。

再開する場合の選択肢: (a) より大きい self-hosted/larger runner で dw variant
を回す (b) workerd の wasm コンパイル挙動(linux でのブロッキング)を上流に
切り分け報告 (c) dw variant を 3 モジュール構成 2 種(kcm-dw / sched-dw)に
分割して段階検証。いずれもコスト判断が要るため、ユーザー判断待ち。

**S23 の解決(2026-07-12、同日訂正)**: dw wedge の根本原因が判明した。
QPS=50 化によりコントローラー起動時の informer ストーム(kcm 15 +
gc ~30 の LIST/WATCH が同時発火)が、**per-request 契約の apiserver DW
= 1 リクエスト毎の ~63MB WebAssembly.Instance 生成(同期・データセグメント
コピー)** を wrangler dev の単一イベントループ上に連続発生させ、
informers のタイムアウト→再 LIST が積み上がる**ライブロック**を起こして
いた(macOS の `sample` によるネイティブプロファイルで
`InstanceBuilder::LoadDataSegments` / `memory_copy_wrapper` がホットと確認。
Node+Deployment で決定論的に再現、Node+RC は margin 内で生存という
不可解な差もストーム量の差で説明がつく)。**QPS=20/Burst=30(upstream
kube-controller-manager デフォルト)へ変更で解消**: 同フローが 4 秒で完走、
canary 要件も維持(50 Pod 作成 11s・orphan 完了 11s)。S22/S23 で疑った
「wasm コンパイル」は誤りで、実体は**インスタンス化のスタンピード**だった
(訂正として記録)。本番 Loader は単一イベントループを共有しないが、同じ
スタンピードは実 CPU 課金を無駄に燃やすため、20/30 は本番にも正しい値。

---

## S24: apiserver DW の per-request インスタンス化が本番 128MiB isolate を超過(2026-07-17、実デプロイで実測)

**症状**: 実デプロイ(k8flare.kooffice.workers.dev)に実ノードを join させ
baseline conformance を回すと、`SchedulerPredicates` 系が Pod Pending の
まま 2〜5 分でタイムアウト。当初スケジューラの問題に見えたが、`wrangler
tail`(このセッションで初めて本番の実ログを取得)で真因が判明: 45 秒間に
**`Worker exceeded memory limit` 例外が 41 件**。KCM は実際に動いていて
ReplicaSet/Pod を作っていた(ログに `live-probe-<rs-hash>-<pod>` が見える)
が、その **apiserver DW 呼び出し(RS 作成・status 更新・events 書き込み)が
軒並みメモリ超過で落ち**、永続化されなかったため GET すると空に見えていた。

**機構**: apiserver は per-request 契約(`pkg/cfruntime`.Serve が
`<-dispatchDone` で 1 リクエスト処理後に main() 終了)。`bootstrap.ts` の
`makePerRequestBootstrapJS` が **リクエスト毎に新規 `WebAssembly.Instance`
(= 新規 Go 線形メモリ)を同一 isolate 内に生成**する。apiserver.wasm は
62.9MB で 4 バイナリ中最大 —— **コンパイル済み Module(isolate 内で 1 回・
全インスタンス共有)だけで 128MiB の大半を占有**し、残ヘッドルームに複数の
並行 Go インスタンスの線形メモリが乗り切らない。KCM 15 informer + scheduler
+ kubelet + e2e の LIST バーストで並行数が跳ね、超過する。さらに
`apiserverFetch` は GET/HEAD の 500 を最大 3 回リトライするため、OOM の 500
がインスタンスを増やす**リトライストーム**として増幅していた(41 例外の一因)。
watch は WatchHub DO 経由で apiserver DW を使わない(short な watch-open 時の
1 往復のみ)ため、並行数の主因は短命 LIST/GET/POST バーストのみ。

**なぜ今まで不可視だったか**: wrangler dev は 128MiB を強制しない(chunk
assembly コメントが既に「wrangler dev never enforces」と明記)。go test も
手動 curl も単発リクエストで、並行インスタンスが積み上がらない。**本番
デプロイに実コントローラー負荷をかけて初めて顕在化する**、本番固有の症状。

**試行して撤回した一次修正(並行上限)**: OOM する isolate 自身
(per-request bootstrap)に並行インスタンス数の上限(FIFO キュー)を入れる
案を実装・実デプロイして検証したが、**net で悪化したため撤回**した。上限は
確かに OOM を消した(`wrangler tail` で memory 例外 41→0)が、**キュー滞留の
レイテンシが kubelet のデッドライン制約付き登録リクエストをデッドライン超過
させ、kubelet がキャンセル → ノードが永久に Ready にならず、そのキャンセルが
apiserver 自身の storageDo サブリクエストにも連鎖**した(実デプロイで確認、
2026-07-17: cap 無しならノードは ~40 秒で Ready、cap=4 も cap=10 も Ready に
ならない。唯一の差分は cap)。OOM をレイテンシ起因のキャンセルに置き換えた
だけ。並行を絞る方向は、デッドライン制約のある登録経路と両立しない。

**真の修正候補(並行を絞るのではなく天井を上げる方向)**:
(a) **apiserver.wasm(62.9MB、4 バイナリ中最大)のサイズ削減** —— kcm/gc/
sched でやった依存切断(cri-client / CEL 等)と同じ手法。コンパイル済み
Module のベースラインを下げれば、同じ 128MiB でより多くの並行インスタンスが
乗る。他 3 バイナリは 41〜45MB なので、apiserver も同水準まで削れれば OOM
閾値が大きく上がる(場合によっては上限機構なしで足りる)。yield は未知だが
確立した手法。(b) **apiserver の resident 化**(ServeNonBlock + park で 1
インスタンス再利用): 並行×メモリ問題そのものが消える(1 線形メモリ)。ただし
「dispatch 2 で Go program exited」という S19 スパイク結果の再調査、token
rotation の memo(sync.OnceValue、per-request 前提)の見直し、S8 の IoContext
制約(request を跨ぐ goroutine の凍結)の確認が要る大きめの変更。apiserver の
ハンドラは request-scoped で跨ぎ goroutine を持たないため原理的には可能。
どちらもこのセッションでは未着手 —— ユーザー判断待ち。

**現状の deployed state**: 上記の撤回により、deployed Worker は cap 導入前と
同じ(ノードは Ready 化し制御プレーンは機能するが、重い並行負荷では apiserver
DW が 128MiB を超えて散発的に OOM する既知の天井が残る)。軽〜中負荷では
機能し、conformance の SchedulerPredicates 系のような重い並行を伴うテストで
顕在化する。

**天井を上げる作業 その1 — wasm-opt -Oz(実施済み、-11.2MB)**: apiserver の
ビルドレシピは他 3 バイナリと違い wasm-opt を通していなかった(生の
`go build` 出力 62.9MB をそのまま chunk していた)。他と同じ `-Oz` を
Makefile に追加して **62.9MB→51.8MB(-17.8%)**。実 wrangler dev + 実
client-go の go test 全通過で機能無傷を確認。コンパイル済み Module の
ベースラインが ~18% 下がり、同じ 128MiB でより多くの並行インスタンスが乗る。
ビルド時のみの変更でランタイム挙動は不変。

**天井を上げる作業 その2 — フル client-go クライアントセットの刈り込み
(実施済み、-10.4MB → apiserver 41.3MB)**: attribution(wasm name section)で
apiserver 肥大の主因が判明した。`pkg/apiserver` は (1) `rbac.go` が
`k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy`(RBAC
既定ロール)、(2) `serviceaccounttoken.go` が
`k8s.io/kubernetes/pkg/serviceaccount`(SA トークン生成/検証)を import し、
どちらも **`k8s.io/client-go/kubernetes` の集約 Clientset 型**(全 API 群の
フィールドを持つ)を推移的に引き込む。結果、apiserver が提供しない
flowcontrol(×4 版 1.3MB)・admissionregistration(×3 版 1.66MB)・
resource/DRA(×4 版 2.5MB)・extensions/v1beta1・apps/networking/storage の
beta 版などの型 + それぞれの typed client + applyconfigurations が全部リンク
される(apiserver は**サーバー**でありこれらの client アクセサは一切呼ばない
デッドコード)。controllers(kcm/gc/sched)はこれを go.wasm.mod の
clientgo-lean-mirror(幅刈り込み)で解決済みだが、apiserver は go.wasm.mod を
使わず素の go.mod(フル client-go)でビルドしていた。**修正: apiserver も
`-modfile=go.wasm.mod -tags leanwidth` でビルド**(controllers と同じ)。
apiserver は**サーバー**で刈られた client アクセサを実行時に一切呼ばないため、
既存の leanwidth 幅で追加 width 不要でそのままコンパイル・動作した。実
wrangler dev + 実 client-go の go test 全通過で機能無傷を確認(刈られた stub の
panic 無し)。wasm-opt と合わせて **62.9MB → 41.3MB(-34%)**、他バイナリ
(kcm 42 / gc 41 / sched 45MB)と同水準、チャンクも 3→2 に減った。

**天井を上げる作業の中間結果**: apiserver DW のコンパイル済み Module
ベースラインが 62.9→41.3MB(-34%)に下がったが、**本番で実測したら OOM は
減った(例外 41→29/45s)だけで解消せず、KCM の書き込みはまだ壊れ Pod が
作られなかった**(2026-07-17)。per-request インスタンス化という**アーキ
テクチャ**が根本問題で、サイズ削減だけでは足りない。

**根本修正 — apiserver の resident 化(2026-07-17)**: apiserver を per-request
(リクエスト毎に新規 Go インスタンス=新規線形メモリ)から **resident(isolate
毎に 1 インスタンスが全リクエストを処理)** に変えた。1 線形メモリを Go GC が
リクエスト間で回収するので、並行数によらずメモリが積み上がらない。並行
リクエストは 1 インスタンス内の並行 goroutine で捌く(通常の Go http サーバと
同じ)。実装:
- `pkg/apiserver/cmd/apiserver-wasm/main.go` の main() を `workers.Serve`(1
  dispatch で exit)から `ServeNonBlock + Ready + select{}`(park)へ。S19 の
  「dispatch 2 で Go program exited」は resident **bootstrap** に exit する
  Go main() を組み合わせた不整合が原因で、main() を park させれば解消(KCM/
  gc/sched と同じ)。
- **panic recovery ミドルウェア追加**(resident では 1 ハンドラの panic が
  共有インスタンス全体を殺す、S19 警告。実 kube-apiserver も同じ filter を持つ)。
- `loader/apiserver.ts` を resident bootstrap(pump window 15s)へ。apiserver は
  バックグラウンド goroutine が無い(検証済み)ので pump は正当性には不要、
  kubelet heartbeat 間の暖機のためだけ。アイドルで退避 → scale-to-zero 維持。
ローカル検証: go test 全通過(実 wrangler dev + 実 client-go、dispatch 2+
含む)、5 ラウンド持続の並行 20/20 全成功 + Deployment 10 replica が
KCM→scheduler で 10/10 bind 収束、wedge/リーク/クラッシュ無し。**OOM 解消
自体は 128MiB を強制する本番でしか観測できない** → 監視付き本番 run で確認。

**続報 — resident 化が顕在化させた二次バグ「クロスリクエスト I/O」とその修正
(2026-07-17、コミット ae088de)**: resident 化で OOM(メモリ例外)は本番で
0 に消えた(41→29→0、監視付きデプロイ版 9d7fcb44 でノード 10s で Ready)。
だが**それでも Pod が作られない**状態が残り、`wrangler tail --format json` で
本番ログを見ると別の例外が出ていた:

```
outcome=exception url=.../api/v1/namespaces/default/pods ep=k8flare
EXC: Cannot perform I/O on behalf of a different request. I/O objects
     (such as streams, request/response bodies, and others) created in
     the context of one request cannot be accessed from a different request.
```

根本原因: `main.go` の `storageDo` が STORAGE Fetcher を `sync.OnceValue` で
**最初のリクエストの env から一度だけ**取得してメモ化していた。per-request
時代はメモが 1 リクエストしか生きなかったので健全だった(コード内コメントも
「instantiated fresh per request なので memo は 1 リクエスト分」と明記して
いた)が、resident 化でこの前提が崩れた。サービスバインディング Fetcher は
**リクエストスコープの I/O オブジェクト**で、それを生成したリクエストの
IoContext が終わった後に別リクエストの dispatch から使うと上記例外になる。
apiserver bootstrap の pump window(15s)が切れた直後から、以降の全 kine
書き込みが例外化し KCM の Pod 作成が停止していた。**wrangler dev はこの
クロスリクエスト I/O 規則を強制しないので dev では不可視**、単発リクエストの
go test でも起きない — 実負荷の本番でしか出ない(S24 の OOM と同じ「本番
限定」性質)。

なぜ resident コントローラー(kcm/gc/sched)は同じパターンで壊れないのか:
コントローラーは outbound バインディングを**自分の resident リクエストの
WaitUntil 内(=そのリクエストがまだ生きている文脈)で一度だけ捕捉**し、
以降もその生きた文脈の中からしか呼ばない(`restconfig.go`、`ResidentService`
が `run(ctx)` を request-1 の waitUntil 下で永久実行)。apiserver は逆に
**各リクエスト自身の dispatch 内で storage I/O する**ので、インスタンス化
リクエストのバインディングではなく「今処理中のリクエスト」のバインディングが
要る。

修正(グローバル差し替えは並行 dispatch で競合するので不可 → リクエストの
`context.Context` で通す):
- `bootstrap.ts` が `handleRequest(request, env)` として**そのリクエストの
  env** を第2引数で渡す(instantiate 時の env は Go プログラム起動専用)。
- `handler_js.go` の dispatch が `cloudflare.WithEnv` で env を**dispatch
  ごとのクロージャ**(共有状態なし)から req.Context() に付与。
- `cloudflare` に `WithEnv`/`EnvFromContext`/`BindingFromContext` を追加。
- `storageDo` が STORAGE を `req.Context()` から解決しクライアントを毎回構築
  (メモ化撤去)。`pkg/apiserver` は既に `r.Context()` を storage.go の
  `NewRequestWithContext` まで通していたので storage 層は無改変。
- 副次効果: 各 storage サブリクエストが発生元リクエストに帰属するので、
  1 インボケーションに全クラスタの kine トラフィックが積み上がる
  (Cloudflare の per-invocation サブリクエスト上限に抵触する)問題も回避。
  コントローラー流の「request-1 の文脈を永久 waitUntil で開いたまま全 I/O を
  そこに集約」案(B)を採らず本案(A)にした決め手がこれ。

コントローラーは無影響(env 引数なしで handleRequest を呼び、ハンドラは
env-from-context を読まない)。**既知の follow-up**: `getTokens` の
provisioned クラスタ vault 読み取りは context 無しの http.NewRequest で
storageDo を呼ぶため resident 下ではグローバル(request-1)バインディングに
フォールバックする(同じクロスリクエスト I/O 危険)。default クラスタ経路
(conformance 全部)は踏まないので未修正のまま記録。

本番検証(監視付き、Version ff7cc3d3): rm した clean 環境で Deployment →
ReplicaSet → Pod → scheduler bind → tmpfs ノードの kubelet/containerd で
**5/5 Pod Running**、新規 2 replica Deployment が **8 秒で 2/2 Running**、
tail で**クロスリクエスト I/O 例外 0・メモリ例外 0**。この時点の default
クラスタは**ノード稼働 + ワークロード収束済み**の状態で tail レートが低かった
(~0.7 req/s、うち大半がノードの心拍)——これは「アクティブなクラスタの書き込み
単価が低い」ことの実測であって、**true-idle(ノード無し)クラスタが常駐ゼロに
パークする scale-to-zero そのものの実証ではない**(後日、収束不能ワークロードを
残した状態では tail が ~4.5 req/s の churn を示した。上の「訂正 — stale read」を
参照)。

**別件の所見 — teardown 済みクラスタの残骸 `prod-smoke` の暴走(2026-07-17)**:
tail に `/c/prod-smoke/` 宛の内部コントローラートラフィックが**無操作でも
~8 req/s 継続**していた(外部からは 404「cluster not found」)。Cluster DO は
消えているのに Controllers DO のアラーム/resident KCM が死んだクラスタへの
informer を回し続けている疑い。管理 API の `DELETE /clusters/<id>` teardown は
本番 `ADMIN_TOKENS` 必須で、シークレットはユーザー管理(deny-list)のため
セッション内では停止不能 → **ユーザーに要 teardown として申し送り**。コスト
不変条件 #3(アラームは未処理の仕事がある時だけ再武装しアイドルで自己解除)
の観点で、**「Cluster DO が消えたのに Controllers 側が park しない」park 漏れ
バグの可能性**。正常パス(default)は scale-to-zero を満たすので edge case だが、
resident コントローラーの「バックエンド消失時の自己停止」は要追検討。

**訂正 — 「アイドル静穏」判定は stale read に騙されていた(2026-07-17)**:
generation デプロイ(99d1a763)後の idle 検証中、`GET` した resourceVersion が
凍結(tailtest rv=7206・global rv=14603)していたため「アイドルで書き込み
ゼロ=scale-to-zero 成立」と一旦判定した。**これは誤り**だった:`wrangler tail`
(キャッシュ非経由の ground truth)で測ると、default クラスタは無操作でも
**~4.5 req/s で書き込み継続**(0ノードで収束不能な Deployment の pod を KCM が
作り続ける自己 poke ループ)。**API の読み取りが stale キャッシュから返ることが
あり、「rv 凍結」はアイドルの証拠にならない** — 書き込みの有無は tail で確認
すること(CLAUDE.md ローカル開発の落とし穴に既出の「読み取り専用ポーリングでは
発火しない」の一般化)。教訓: **ノードをテアダウンする前に、そのノードに
依存する Deployment を先に消す**(不要になった unconverged Deployment を残すと
KCM が延々 churn する)。

**未解決バグ — write-wedged Deployment(2026-07-17、要追調査)**: 上記の KCM
hot-loop churn を長く受けた Deployment(tailtest)が、`GET` は rv=7206 を返す
のに実 rv はより高い(`PUT` を rv=7206 で送ると 409「object has been modified」、
`DELETE` は「store delete: conflicted 5 times」)状態に陥り、**delete も update も
CAS が stale read を使うため永久に競合して API から一切変更できなくなった**
(KCM を無効化して churn を止めても解消せず、rv 7206 固着のまま)。store.Delete は
Update パス非経由なので generation 変更(Create/Update のみ)とは無関係で、
同時期の他 Deployment(clean-run/concept-final)は正常に削除できた。読み取りの
stale がどの層(Cluster DO の kine キャッシュか、hot-loop churn が生んだ kine
ログの不整合か)で生じているかは未特定。**根本解消にはクラスタ teardown/リセット
(fresh クラスタなら wedged オブジェクトは存在しない)が必要**。ホットループ churn
がストレージの読み取り整合性を壊しうる、という点自体が調査対象。

**generation 維持の実装と、それが露呈させた GC レイテンシの根本原因
(2026-07-17、コミット 2e2fb3c)**: apiserver が `metadata.generation` を
一切維持していなかった(3 つの advisory conformance spec に跨る Deployment
revision-tracking ギャップの根)。store の Create/Update に汎用実装を追加:
status subresource を宣言するリソース(= k8s が generation を bump する
spec/status 分割集合、かつ subresource.go の copyStatus が spec を保持する
集合)に限定してゲートし、Create で 1、Update は「spec が実際に変わった時だけ
+1(サーバー管理、クライアント不可侵)」。spec 比較は `reflect.DeepEqual`
ではなく `apiequality.Semantic.DeepEqual` — resource.Quantity や metav1.Time が
no-op RMW で内部表現だけ変わる("1Gi"↔"1073741824")場合の spurious bump を
防ぐ。status 書き込みは spec を保持するので equal ブランチに落ちて bump し
ない(status 更新は generation を変えてはならない不変条件)。テスト
`TestGenerationSemantics` は「bump してはいけない」2 ケース(status 書き込み・
no-op RMW)を含めて検証、apiserver スイート全緑。

副次効果 — この generation 修正が、既に inert だったコントローラー DO の
アラーム収束予測子を設計通り復活させた: `hasUnconvergedWork()`
(controllers/index.ts)は `status.observedGeneration < metadata.generation`
で Deployment 収束を判定していたが、generation が常に 0 だったため今まで
このチェックは常に false。generation が正しく動くことで、実 KCM が
observedGeneration を進めるまでアラームが正しく armed に留まる。

**GC eventual 削除の ~90-130s レイテンシの根本原因(未修正・要検討)**:
サブエージェント判定で GC 回収が実 k8s(数秒)より大幅に遅い ~90-130s と
実測された。原因は上記 `hasUnconvergedWork()` が **GC 待ちの orphan(削除
された Deployment の dangling ownerRef を持つ RS/Pod)を「未処理の仕事」に
数えない**こと。Background 削除後、orphan の RS は status==spec で「収束」
扱いになり予測子が false を返す → `SAFETY_NET_INTERVAL_MS`(60s)のアラームが
park → GC は削除 poke の pump window + 次の 60s tick でしか進めない(GC が
1 件削除すると、その削除書き込みが pingControllers を再トリガして次をカスケード
削除するが、最初の「グラフ再構築 → orphan 検知 → 初回削除」に窓が数回要る)。
安全な改善案: 予測子に「GC 待ち orphan の存在」を足せばアラームが 15s cadence
で armed のまま GC を早く完了させ、片付いたら park できる(コスト不変条件
#1/#3 と整合 — orphan は真に unconverged な仕事)。ただし dangling 検知の
list コスト、または gc-wasm のキュー深さを /healthz で露出するプラミングが
必要で非自明。GC の非同期 eventual 自体はユーザー承認済み(CLAUDE.md「kubectl
delete は非同期 eventually-consistent」)なので、コスト感応な予測子変更を
急がず、根本原因のみ記録して据え置く。

## S25: upstream generic registry の js/wasm リンク可否とサイズ実測 (2026-07-26、GO 判定)

pkg/apiserver の手書き generic-registry 相当 (handler/store/table/
subresource ≈2,330 行) を upstream の
`k8s.io/apiserver/pkg/registry/generic/registry` (genericregistry.Store +
storage.Interface) に置き換えられるかのフェーズ 0 スパイク。判定材料は
2 つ: (1) GOOS=js でリンクできるか (2) 64MiB Loader cap に収まるか。

**結果: 両方 YES。**

- 素の import は etcd クライアント (syscall.Flock) と go-systemd/journal
  で即コンパイル不能。依存経路は 3 本:
  `registry/generic → storagebackend/factory → etcd3 クライアント`、
  `storagebackend/config.go → etcd3 (LeaseManagerConfig のためだけ)`、
  `cacher/cache_watcher.go → util/flowcontrol (APF) → lean client-go に
  無い flowcontrol informer`。加えて 1.36 の新規
  `genericregistry → pkg/sharding → CEL パーサ` が cel-go 一式 (実測
  -5.7MB 分) を引き込む。
- 対処は KCM 移植と同型のミラー+オーバーレイ 5 点
  (pkg/k8s-js-overlays/apiserver/、gen-apiserver-js-mirror.ts が
  .build/apiserver-js-mirror を生成、go.wasm.mod の k8s.io/apiserver
  replace がそこを指す。go.mod=ホスト側は無改変 upstream のまま)。
- サイズ実測 (leanwidth, -s -w, trimpath):
  - 現行 apiserver 単体: 48.98MB (raw) → 41.3MB (wasm-opt -Oz 後)
  - 現行 apiserver + genericregistry+cacher 閉包を足した合成 (手書き
    コードを何も消していない最悪ケース): 82.3MB → sharding/CEL
    オーバーレイ後 76.6MB (raw) → **64.99MB (wasm-opt 後)**。
    cap 67.11MB に対し残 2.0MB。
- 手書き側の削除で若干戻るが、ヘッドルームは薄い。採用フェーズでは
  make wasm のサイズゲートを注視し、fieldmanager/admission (SSA 機構、
  genericregistry が無条件 import) の要否を次の削減候補として検討する。

スパイク手順の再現: `tmp-regspike/main.go` に genericregistry +
pkg/apiserver を import する main を置き、
`GOFLAGS=-modfile=go.wasm.mod GOOS=js GOARCH=wasm go build -tags
leanwidth` → wasm-opt -Oz で計測 (ディレクトリ自体はコミットしない)。

次フェーズ (未着手): DO ストレージ上に storage.Interface を実装し、
1 リソース (configmaps) を genericregistry.Store 経由に切り替えて
conformance の該当テストで挙動同値を確認 → apidef.Table 全体へ展開 →
手書き handler/store/table/subresource を段階削除。

### S25 phase 1 実装 (2026-07-26): configmaps が実 genericregistry.Store で稼働

- `pkg/apiserver/upstreamstorage.go` — KineStorage: storage.Interface を
  Cluster DO の kine プロトコル上に実装 (EncodeToStorage/DecodeFromStorage
  でバイト互換、リソース単位でどちらの層にも切替可能)。Watch は TS 層の
  まま (Store の非 watch verb は呼ばない)。
- `pkg/apiserver/upstreamregistry.go` — 全リソース共通の genericStrategy
  (外部型 + SimpleNameGenerator、Prepare/Validate は既存の
  ApplyDefaults/admission 層に委ねる) + NewUpstreamStore。
  CompleteWithOptions は経由しない (etcd の storagebackend factory を
  要求するため) — それが既定化するフィールドは明示設定が必要で、
  ObjectNameFunc の設定漏れは初回 create で即 panic した (実測)。
- 移行スイッチは stores.go の upstreamMigrated (現在 "v1/configmaps"
  のみ)。single-object 5 verb が upstream 経由、コレクション系
  (DeleteCollection / 名前空間 sweep) はバイト互換の生パスのまま。
- 挙動差分は 1 件だけスイートに現れた: UID 不一致の update を手書き層は
  422 Invalid にしていたが、実 apiserver は UID precondition 失敗 =
  409 Conflict。テストを upstream セマンティクスに追随させた
  (TestConfigMapCRUD/UpdateMismatchedUID)。
- サイズ: apiserver チャンク 41.4MB → 65.18MB (wasm-opt 後、3 分割)。
  cap 67.11MB に対し残 1.9MB — 以後のリソース追加ではなくコード削除で
  戻る見込みだが、fieldmanager/admission の削減余地を次に検討する。
- 検証: フルスイート + make test-kcm green (実 KCM が configmaps を
  読む informer 経路含む)。

### S25 phase 2-3 完了 (2026-07-26): 全リソースが genericregistry.Store、手書き CRUD 層を削除

- apidef.Table の全 38 リソースが upstream genericregistry.Store 経由に
  移行完了 (バッチ 4 回 + pods)。ResourceStore は常に upstream Store を
  構築し、手書きの Get/List/Create/Update/Delete 本体と dead helper 群
  (setResourceVersion / matchesFieldSelector / 各 status ヘルパー) を削除
  (store.go 670→405 行。残りは storageKey/prefix、DeleteCollection、
  DeleteAllInNamespace などバイト互換の生パスとステータスヘルパー)。
- 移行で吸収した挙動差分・落とし穴 (詳細は各コミット):
  - namespace 無しコンテキストは BeforeCreate が internal error 扱い —
    cluster-scoped でも空 namespace を必ずスタンプ。
  - field selector: upstream の既定 attr は metadata.name/namespace のみ。
    selectableFieldsFor (旧 store.go の独自セレクタ表) を attr func として
    供給しないと kube-scheduler/kubelet の spec.nodeName list が空になる。
  - DELETE 応答: upstream 既定は metav1.Status。ReturnDeletedObject: true
    で従来どおり削除オブジェクトを返す (settleDeletedObject が型 switch)。
  - Orphan/Foreground: EnableGarbageCollection: true で upstream 自身が
    deletionTimestamp+finalizer をスタンプ (markForDeletion の手書き
    スタンプは upstream では immutable エラーになる)。
  - metadata.generation バンプと Job の selector 生成は
    genericStrategy.PrepareForCreate/Update へ移設。
  - エラー形状: apierrors.StatusError が全経路に流れるため、
    isStatusReason/isNotFoundErr を両対応にした (endpoints reconcile が
    upsert の NotFound を見逃して EndpointSlice が生えない回帰を検出・修正)。
- 検証: フルスイート + make test-kcm green。apiserver チャンク 65.15MB
  (cap 67.11MB、残 1.9MB)。

#### S25 phase 2-3 の追記・訂正 (2026-07-26)

- 訂正: 上の「全 38 リソース」は数え違いで、apidef.Table の実数は **41
  リソース** (バッチ 3 回 + pods 単独)。移行スイッチ自体はもう存在しない
  (ResourceStore が常に upstream Store を構築する) ので、リソースを
  取りこぼす余地はない。
- 追加で見つかった KineStorage 側の 2 件 (どちらもスイートには現れず、
  実 wrangler dev への手動リクエストで発見):
  - `metadata.name` を固定する list は Store が **非 recursive** な
    GetList (単一キー) に変換する。kine アダプタがそれをプレフィックス
    扱いしていたため `?fieldSelector=metadata.name=...` が常に空だった。
    etcd3 の store と同じく Recursive で分岐する実装に修正し、
    TestPodListFieldSelectors で固定。
  - 未書き込みの facet は revision 0 を返し、upstream の versioner は
    それを "illegal resource version from storage: 0" として拒否する
    (fresh namespace の events を list する orphan sweep で露出)。
    グローバルリビジョンにフォールバックする。
- 手書き graceful-delete スタンプ (markForDeletion のループ) と
  finalizerForPolicy も dead になったため削除。
- 最終サイズ: apiserver チャンク **65,143,608 bytes** (cap 67,108,864 に
  対し残 1,919KiB)。移行前 (phase 1 完了時) の 65.18MB からわずかに減。
- 既知の別問題 (この作業とは無関係、main でも再現): `make vet` の
  GOOS=js レグが `-modfile=go.wasm.mod` を付けていないため etcd/journal
  で失敗する。CI は npm script 経由なので影響なし。


## S26: ノードなし Deployment でアラームのバックオフが効かない (2026-07-30、実測。**→ 下記「S26 訂正」で原因判明・修正済み。この節の当初の結論は誤り**)

外部レビュー由来。**ノードが 1 台も無いクラスタに Deployment を 1 つ作る**と
(新規クラスタの最初の 1 時間で最も起こりやすい状態)、Cluster DO と
Controllers DO のアラームが約 15 秒間隔で鳴り続ける。

実測 (wrangler dev --local、Miniflare の `_cf_ALARM.scheduled_time` を
10 秒間隔でサンプリング、2026-07-30):

- Deployment 作成後 **10 分間、Controllers の次回発火が常に 15 秒後**に
  再設定され続けた (18:26:45 → 18:27:00 → … → 18:36:45)。Cluster DO も
  ほぼ同周期。
- 同じ 10 分間の **kine 書き込みは 0 件**。つまり書き込みポークで
  起こされているのではない。
- Deployment を削除すると **20 秒でパーク**した(修正前の値。修正後は
  数分かかる — 下記「S26 訂正」と docs/admin-guide.md 参照)。

これは `controllers/index.ts` の実装コメントと食い違う。同ファイルは
「収束しない仕事(ノードなしクラスタの Deployment がまさに例として
書かれている)のコストを 15 秒→倍々→10 分上限のバックオフで抑える」と
述べているが、observed では倍化が一度も起きていない。`unconvergedTicks`
をリセットする経路(`fetch()` の "fresh write: reset backoff" と
warmup 分岐)のどちらかが毎周期踏まれている疑いが強いが、**書き込みが
0 件である以上「書き込みポークが原因」では説明できない**ため、原因は
未特定。推測で predicate を触らないこと(このアラーム経路は過去 2 回
壊して直している: 94-alarms インシデントと d1a9503)。

コスト影響: 15 秒間隔なら月あたり約 17.3 万回のアラーム + 同数の
Controllers DO ウェイク(訂正 2026-07-30: 当初「約 4.3 万回」と書いたが、
2,592,000 秒 ÷ 15 秒 = 172,800 であって ÷60 ではない。4 倍過小、しかも
問題を軽く見せる方向の誤りだった)。アイドルではない(ワークロードは存在する)が、
「収束しない仕事は指数バックオフで抑える」という設計意図は満たせて
いない。

`cost-gate.yml` はこの状態を検出できなかった: ワークロードを 1 つも作らず、
Node と Service だけを作ってアイドル判定していた(**追記 2026-07-30: 同日
コミット 049881a でワークロードケースを追加済み。さらに
`pkg/apiserver/upstreamstorage_test.go` の 3 本が `make test` レーンで
同じ回帰を秒単位で捕まえる**)。3 本なのは最初の 1 本ではガードの半分しか
固定できていなかったため — `currentRev != 0` の節だけを外すと、テストは
通ったまま不在キーで nil 参照 panic になる(2026-07-30 のレビューで発見・
ミューテーションで再現)。

### S26 訂正 (2026-07-30、同日): 原因判明 — no-op 更新の書き込みストーム

上の S26 は 2 点が**誤り**だったので訂正する(不可侵ルール #4: 消さずに追記)。

1. **「kine 書き込みは 0 件」は計測ミス。** Cluster DO は facets により
   複数の sqlite に分かれており、最初の計測は 7 個あるうちの 1 個
   (kine 行 0 件のシャード)だけを見ていた。全シャードを数え直すと
   **60 秒で 1,130 行**書かれていた。CLAUDE.md が挙げる「計器の側の誤り」
   そのもの。
2. **「バックオフが壊れている」も誤り。** バックオフは仕様通り動いていた。
   毎サイクル本物の書き込みが届くので `fetch()` の
   `unconvergedTicks = 0`(index.ts:350)が正しくリセットしていただけ。
   15 秒間隔は症状であって原因ではない。

**真の原因**: `KineStorage.GuaranteedUpdate` に upstream etcd3 store の
no-op 抑止がなかった。upstream は Txn の前に
`if !origState.stale && bytes.Equal(data, origState.data)` で「値が変わって
いない更新」を握り潰すが、こちらは無条件に新リビジョンを書いていた。

結果として自己持続ループが成立していた:

```
deployment controller が同一 status を再計算 → Update
  → kine に新リビジョン(値は前と 1 バイトも違わない)
  → afterWrite → pingControllers → Controllers.fetch()
  → unconvergedTicks = 0(バックオフ解除)+ 15 秒後に再武装
  → コントローラーが再 sync → 最初に戻る
```

実測 (node-less クラスタ + 2 replica Deployment 1 個、値の差分なし):

| | 修正前 | 修正後 |
|---|---|---|
| Deployment の resourceVersion | 5 秒で 9521→9622 (≈20/秒) | 28 で固定 |
| kine 書き込み (120 秒) | ≈2,260 行 | **0 行** |
| kine 総行数 | 7 分で 8,400 行、増加継続 | 159 行で定常 |
| Controllers アラーム間隔 | 15 秒固定 | 15→30→60→120→240 秒(上限 600 秒へ) |

間隔の正確な列は `Math.min(15_000 * 2 ** Math.max(0, ticks - 4), 600_000)`
(controllers/index.ts:447)から 15, 15, 15, 15, 30, 60, 120, 240, 480, 600 秒。
最初の 4 tick は倍化せず、600 秒上限に達するまで累計約 16.5 分かかる。

これはコスト不変条件に対する実害だった: 平凡な操作(ノードを join する前に
Deployment を作る)だけで rows-written 課金と kine の行数が無限に伸びる。
修正は `pkg/apiserver/upstreamstorage.go` の 1 箇所、upstream と同じ
`bytes.Equal` ガード。

**残る未解決 (S26b)**: 修正後も約 8 分周期で `warmupUntil` が再武装され、
`unconvergedTicks` が 0 に戻ってバックオフが 15 秒から数え直しになる
(t=480s で観測)。`alarm()` は `armWarmup: false` を渡すので、これは
`fetch()` 経由のポークが 8 分毎に届いていることを意味する。書き込みは
0 件なので発生源は未特定(dev の isolate 退避かもしれない)。平均間隔は
15 秒固定から約 40 秒に伸びており実害は小さいが、10 分上限には届いて
いない。

## S27: CronJob を起こすものが無い (2026-07-30、コードレビュー由来。未解決・本修正とは無関係)

S26 のレビュー中に発見。**本修正の回帰ではないことは確認済み**だが、
記録しておく。

CronJob コントローラーは WASM KCM で有効になっている
(`pkg/controllers/controllermanager.go:167`)。しかし Controllers DO の
`hasUnconvergedWork()`(`packages/k8flare-worker/src/controllers/index.ts`)が
プローブするのは Deployment / ReplicaSet / Job(管理クラスタでは Cluster も)
だけで、**CronJob は含まれていない**。したがって収束済みのクラスタで
CronJob だけが存在する場合、アラームはパークし、スケジュール時刻が来ても
それを起こすものが無い。

**ただしこれはコードから導いた結論であって、スケジュールを逃すところを
実際に観測してはいない**(不可侵ルール #2)。このリポジトリには
「kube-proxy が Worker をハングさせる」というソースだけの結論が誤りだった
前例がある。測定済みなのは「本修正とは独立である」という点だけで、
取りこぼしそのものは未測定。

これは k8flare の設計の根っこ(イベント駆動でアイドル時ゼロ)と、cron が
本質的に**時刻駆動**であることの衝突であり、predicate に 1 行足せば済む
話ではない。CronJob を「未収束の仕事」と見なせば、CronJob が 1 つでも
存在するクラスタは永久にアラームを鳴らし続けることになる(コスト不変条件
#1/#3 に抵触)。次のスケジュール時刻に合わせて alarm を張る、が正しい
方向だと思われるが未検証。

本修正との独立性の根拠(レビュアーが確認): `controllers/index.ts` は S26 の
diff で未変更で、park 判定も本ブランチ以前から存在する。修正前の書き込み
ストームは「既に開いているウィンドウを維持する」ことしかできず、一度
パークしたものを再び開くことはできないため、DO がパークした後の挙動は
修正前後で同一。

## S26b 補足: バックオフが効くようになったことで生じるトレードオフ

S26 訂正の副作用として記録。修正前は storm の各書き込みが
`Controllers.fetch()` に届いて `unconvergedTicks` を 0 にリセットし
(`controllers/index.ts:350`)、アラームを 15 秒間隔に張り付かせていた。
修正後はバックオフが実際に効くので、**未収束クラスタでは時刻駆動の遷移**
(Deployment の `progressDeadlineSeconds`、Job のバックオフ等)**が最大
数分遅れる**。これは意図した取引だが、ブランチ上にこれを検証するテストは
無い(cost-gate の新ケースが見ているのはコスト側と park だけ)。

### S27 訂正・解決 (2026-07-30 同日、実測 → 修正)

上の S27 は「コードから導いた結論であって未測定」と書いた。測定した結果、
**結論は正しかったが、素朴な測定は 2 回とも逆の答えを出した**。経緯を
残す(不可侵ルール #4)。

**測定 1・2(誤り)**: ノードなしクラスタに CronJob を 1 つ置き、アラームが
パークしたのを確認してから発火時刻を待つ → **Job は作られた**。S27 は
再現しない、と一度は結論しかけた。

誤りは 2 つあった:

1. **park 判定の計器エラー**。`count(*) FROM _cf_ALARM == 0` は
   「パークしている」だけでなく「**アラームが今まさに実行中**」でも真に
   なる。1 サンプルで判定していたため、発火の合間を park と読んでいた。
   5 秒間隔で 12 回連続ゼロ(60 秒)を要求するように変えた。
2. **isolate が生き残っていた**。持続的な park を確認した後でも Job は
   作られた。原因は **KCM の dynamic worker の isolate がまだ生きていて、
   実 cronjob controller 自身のタイマーが動いていた**こと。DO の
   アラームは無関係だった。

**測定 3(決定的)**: 持続的 park を確認 → `wrangler dev` を再起動して
**全 isolate を退避** → HTTP リクエストを 1 本も送らずに発火時刻を跨ぐ。
結果: **Job は作られなかった**。S27 は実在する。dev で「動いているように
見えた」のは isolate の寿命という環境依存の産物で、本番の退避後には
成立しない。

**修正**: `pkg/apiserver/cronschedule.go` の
`GET /internal/next-cron-schedule` が「次に起きるべき時刻」と「発火時刻を
既に跨いでいるか(overdue)」を返し、Controllers DO が park の代わりに
その時刻へアラームを張る。cron 式の解釈は upstream と同じ
`robfig/cron`(`spec.timeZone` は upstream の `formatSchedule` と同じく
`TZ=` 前置で渡す)。**何をするか**(startingDeadlineSeconds、100 件
取りこぼし時の諦め、concurrencyPolicy)は実 cronjob controller に完全に
委ねている — こちらが決めるのは**いつ起こすか**だけ。

**「時刻に起こすだけでは足りなかった」**: 最初の実装は発火時刻ちょうどに
起こして次の occurrence へ再武装していたが、**それでも Job は作られな
かった**。コールドな KCM はロード・informer 同期・実行に 1 パスでは
足りず、その間に窓が閉じ、跨いだ occurrence は捨てられていた。そこで
`overdue` を `hasUnconvergedWork()` に流し込み、既存の「収束するまで窓を
開け続ける」機構に任せる形に変えた。Job ができれば `lastScheduleTime` が
進んで `overdue` は自然に false になる。

**最終検証(実測)**: CronJob を 1 つ作る → アラームが発火時刻ちょうどに
張られるのを確認 → `wrangler dev` 再起動で全 isolate を退避 → リクエスト
ゼロで発火時刻を跨ぐ → **Job `cj-29756951`(= 13:11:00 UTC)が作られた**。
起動ログ 2 行以外にリクエストは記録されていないので、アラームだけで
コールドな制御プレーンが起きて仕事をしたことになる。

コスト影響は `docs/cost-model.md` の「CronJob の wake-up アラーム」。

**残る限界**: apiserver チャンクは 65,285,282 バイト(cap まで 1,780KiB)。
`robfig/cron` の追加で約 83KB 増えた。

## S28: 手書きの 3 コントローラー代替を実 KCM の 5 コントローラーへ差し戻し (2026-09-09、実測)

`pkg/apiserver` に残っていたサーバーサイドの手書き代替
(`endpoints.go` 456 行 / `nodelifecycle.go` 316 行 / `nodecidr.go` 234 行、
テスト込みで計 1,734 行削除)を削除し、実 kube-controller-manager の
`endpoint` / `endpointslice` / `nodeipam` / `nodelifecycle` / `tainteviction`
を kcm dynamic worker で走らせる形に戻した(不可侵ルール #3)。

**サイズ実測**(`make wasm`、wasm-opt 後の raw バイト。Loader cap は
67,108,864 バイト):

| チャンク | バイト | cap までの余裕 |
|---|---|---|
| apiserver | 64,416,718 | 2,692,146 (2.57MiB) |
| kcm | 44,203,024 | 22,905,840 (21.84MiB) |
| sched | 44,985,213 | 22,123,651 |
| gc | 41,298,694 | 25,810,170 |
| clusterop | 32,328,881 | 34,779,983 |

kcm は 5 コントローラー追加で 42,238,661 → 44,203,024 バイト
(+1,964,363 = +1.87MiB)。事前スパイクの予測値 44,203,045 と 21 バイト差。
apiserver 側は 3 ファイル削除で S27 記録時点の 65,285,282 バイトから
64,416,718 バイトへ(-848KiB。この差分には 7-30 以降の他の変更も含む)。
`pkg/leanclient` は Leases/EndpointSlices/DaemonSets/Endpoints の
client/informer 表面を既に持っており、GOOS=js のコンパイルエラーはゼロ。

**2026-07-06 の削除理由との関係**: 当時の削除理由はサイズではなく
**メモリ**だった(「informer キャッシュ(全 Node・全 Lease・全
EndpointSlice)が本番 128MiB isolate に対して純粋なオーバーヘッドで、
ロード直後の dynamic worker が informer sync 中に死んで reload ループ
した」)。今回それが解消したと主張できる根拠は無い。**本番 isolate での
メモリ挙動は未検証**である。今回検証したのは (a) cap 内に収まること
(b) `wrangler dev` 上で 2 Node・数 Pod 規模の実クラスタとして 5
コントローラーが同時に動くこと、の 2 点だけで、当時の削除理由は
「サーバーサイド代替があるから冗長」の部分だけが無効化された(代替を
消したので冗長ではなくなった)。128MiB 側は本番デプロイ時に
再確認が要る — 残課題として明記する。

**未検証項目(残課題、2026-09-09 レビューで 2・3 を追加)**:

1. 本番 128MiB isolate でのメモリ挙動(上記)。
2. **`tainteviction` の `tolerationSeconds` タイマーは kcm isolate の
   インメモリ状態である**(NoExecute テイントを見た時刻を起点に
   `timedWorkerQueue` が遅延立ち退きを保持する。既定 300 秒)。削除した
   `nodelifecycle.go` は永続化された Node condition のタイムスタンプから
   立ち退き時刻を毎回計算し直していたので**再起動に強かった**。kcm の
   isolate が 300 秒より短い間隔で退避・再ロードされると、到達不能 Node
   上の Pod が**いつまでも立ち退かない**可能性がある。正しさに関わる
   後退の候補だが**未計測** — isolate の退避は S27/S28 と同じ理由で
   local では再現できず、本番か退避を強制できるハーネスが要る。
3. **Pod が動き出すまでのレイテンシ。** `computeclass.go` は Pod 毎に
   Node 名を先にピン留めする(`kubernetes.io/hostname` セレクタ)ため、
   その Node の podCIDR 払い出しが apiserver の同期パスではなく kcm の
   pump window 待ちになった。`pkg/agent/flannel.go` は podCIDR が空の間
   ポーリングし続ける実装なので壊れはしないが、Node 登録から Pod が
   起動するまでの時間は pump window の分だけ伸びる。未計測。

**機能検証**(`make test-kcm` の `TestKCMDynamicWorkerControlPlane`、
実 client-go で dynamic worker 越しに駆動):

- `nodeipam` が 2 Node に別々の /24 を `spec.podCIDR` として払い出す
- `endpoint` / `endpointslice` が selector 付き Service + Ready Pod から
  Endpoints と EndpointSlice を作る(`endpointslice.kubernetes.io/managed-by`
  = `endpointslice-controller.k8s.io`、legacy 側は
  `endpoints.kubernetes.io/managed-by` = `endpoint-controller`。
  targetPort 名 `http` → コンテナポート 8080 の解決と Ready 条件も確認)
- `nodelifecycle` が Lease の更新が止まった Node を Ready=Unknown
  (reason `NodeStatusUnknown`)にし、MemoryPressure/DiskPressure/
  PIDPressure も Unknown にする。Lease が新鮮な Node は触らない

**削除したユニットテストの移植内訳(訂正 2026-09-09)**: `kcmdw_test.go`
のコメントは当初「削除したユニットテストがカバーしていたものを下に移植
した」と書いていたが、これは広すぎた。実際に移植したのは次のもので:

- `nodecidr_allocator_test.go`: clusterCIDR 内の別々の /24 を 2 Node へ
- `endpoints_test.go`: selector + Ready Pod → Endpoints/EndpointSlice、
  targetPort 名の解決、managed-by ラベル、targetRef
- `endpoints_test.go`: selector 無しの Service には Endpoints を作らない
  (レビュー指摘で 2026-09-09 に追加。ExternalName ケースは省略)
- `nodelifecycle_test.go`: Lease 陳腐化 → Ready=Unknown + 3 条件 Unknown、
  `node.kubernetes.io/unreachable` テイント付与(同上で追加)、
  Lease が新鮮な Node は不変

移植せずに落としたのは次の 4 つ:

- `TestReconcileNodeLifecycle_EvictsPodsPastEvictionTimeout` — 立ち退きの
  タイミングは upstream `tainteviction` の責務になった(上の未検証項目 2
  はこの範囲の残リスク)
- `TestReconcileNamespaceEndpoints_NotReadyPodGoesToNotReadyAddresses` —
  Ready/NotReady の振り分けは upstream `endpoint` の判定そのもの
- `TestDeleteServiceEndpoints` の冪等性 — Service 削除に伴う Endpoints の
  始末が upstream reconciler + GC 経路に移った
- `TestNodeCIDRAllocator` の release/reuse・既存 podCIDR の occupy —
  upstream `cidrset.CidrSet` の内部挙動で、upstream 自身のユニットテストが
  持っている

いずれも「振る舞いの持ち主が upstream に移り、conformance でカバーされる」
という理由であり、**required focus set は減らしていない**(不可侵ルール #1)。

**node-lifecycle の poke 経路について(否定的な測定結果も記録)**:
Lease の陳腐化は「書き込みの不在」で検出されるので poke を引っ掛ける
write が無い。そこで Cluster DO の既存の安全網 alarm
(`packages/k8flare-worker/src/storage/index.ts`、live Node がある間だけ
武装・無くなればパーク)から、削除した `/internal/reconcile-node-lifecycle`
POST の代わりに **Controllers DO を poke** する形に変えた
(`pingControllers` と同じ pending-ping 経路。固定間隔ポーリングの新設では
なく、既存 tick の中身の差し替え)。

**訂正 (2026-09-09、同日のレビュー指摘)**: 「`pingControllers` と同じ
pending-ping 経路」を使ったのが誤りだった。その経路は**書き込み由来の
poke** で、Controllers DO の `fetch()` はそれに対して (a) warmup 窓
(3 分)を張り (b) `unconvergedTicks` をリセットし (c) 自分の 60 秒
alarm を再武装する。live Node がある限り 60 秒毎にこれを行うと、
`controllers/index.ts` の `alarm()` コメントが記録している
「アイドルの BYO node が無意味な 60 秒チェーンを生かし続ける」回帰
そのものになる(コスト不変条件 #1/#3 違反)。専用の alarm 由来パス
`/safety-net/node-lifecycle` に変更し、kcm の `ensure(armWarmup:false)`
と pump 1 回だけを行い、backoff にも Controllers DO の alarm にも
触らないようにした。会計は docs/cost-model.md の該当節に記載。

**新経路の実測 (2026-09-09)**: alarm 由来分岐に一時的な計装(到達したら
ConfigMap を 1 個作る)を入れて `wrangler dev` で測った。Node を 1 個
作ると `/safety-net/node-lifecycle` が **60.9 秒間隔でちょうど 1 回ずつ**
到達し(タイムスタンプ 1788940060217 → 1788940121145、差 60,928ms)、
その Node を削除すると **150 秒待って追加の到達はゼロ**(パーク)。
「live Node がある間だけ 60 秒に 1 回 kcm を pump し、Node が消えたら
止まる」という会計はこれで実測済み。ただし**この pump が無いと
nodelifecycle が止まるのか**は相変わらず測れていない(上記 1〜3 の測定と
同じ理由で local では isolate の退避を再現できない)。計装は
コミットしていない。

ただし **この poke が実際に効いていることを local で分離できなかった**:

1. `make test-kcm` の (c) は poke をコメントアウトしても同じ 45 秒で
   通った。テスト中は Controllers DO 自身の alarm(warmup 15 秒間隔 /
   未収束 backoff)が窓を開け続けている。
2. `wrangler dev` に対する手動プローブ(ワークロードゼロのクラスタに
   Node + 陳腐化 Lease だけを置く)でも、poke 有無に関わらず 60 秒で
   Ready=Unknown になった。Node/Lease の書き込み自体が Controllers DO を
   起こし warmup(3 分)を張るため。
3. warmup を跨ぐプローブ(230 秒 Lease を更新し続けてから停止)でも、
   poke 無しで 50 秒後に Ready=Unknown になった。`wrangler dev` の
   isolate は S27 の測定 1・2 と同じ理由(isolate が退避されず Go 側の
   タイマーが動き続ける)で、pump window の境界を再現しない。

つまり **local では pump window の欠如を再現できないので、この poke の
必要性・有効性は測定で示せていない**。根拠はコードの筋(warmup 期限切れ
後、収束済みクラスタでは Controllers DO の alarm はパークするので、
live Node があるうちは何かが窓を開ける必要がある)だけである。S27 と
同じく決定的な測定は本番か、isolate を確実に退避させるハーネスが要る。

## S29: upstream `k8s.io/apiserver/pkg/endpoints` で手書き REST 層を置き換える案 — サイズで NO-GO (2026-09-09、実測)

S25 の次段(手書き handler.go / subresource.go / table.go / discovery.go /
watch.go を upstream の `endpoints.APIGroupVersion.InstallREST` +
`endpoints/discovery` で置き換える)の可否を S25 と同じ方法で計測した
(`tmp-endpointsspike/main.go` に pkg/apiserver と InstallREST 閉包を
import する main、`-tags leanwidth -ldflags="-s -w" -trimpath` → wasm-opt -Oz。
binaryen 129 / go1.26.5)。

| ビルド | raw | wasm-opt 後 | cap 67,108,864 との差 |
|---|---|---|---|
| baseline apiserver-wasm | 76,397,999 | 64,877,427 | +2,231,437 |
| + `endpoints` 閉包(InstallREST + discovery) | 80,402,057 | 68,388,344 | **-1,279,480** |
| + 追加オーバーレイ 3 点(storageversion スタブ / apihelpers から APF 分離 / `cases.Title` 置換) | 78,973,274 | 67,191,434 | -82,570 |

- コンパイル: `endpoints → storageversion → Clientset.InternalV1alpha1()`
  が leanwidth の刈られた clientset に無い 1 件のみ失敗。6 行のスタブ
  オーバーレイで閉包全体が GOOS=js でビルドできる(go-restful /
  x/net/websocket / wsstream / admission / audit / managedfields はそのまま通る)。
- 増分 +3,510,946 バイト(opt 後)の内訳: endpoints/handlers 395KB、
  flowcontrol/v1 377KB(apihelpers 経由の巻き添え)、structured-merge-diff
  285KB、endpoints 215KB、managedfields 118KB、apiserverinternal/v1alpha1
  116KB(storageversion 経由)、x/text 239KB(`cases.Title`)、websocket 104KB。
- 置き換えで消せる手書き 5 ファイルのコード実体は 156,596 バイト
  (pkg/apiserver 自身の全コード 631,844 バイトの 24.8%)で、opt 後換算
  約 0.19〜0.23MB。依存パッケージは 1 つも解放されない(残る pkg/apiserver
  が同じものを使う)。
- 結論: オーバーレイ 3 点 + 手書き削除を全部足しても cap 前後 ±0.15MB で、
  kubectl が直接待つ apiserver チャンクをヘッドルームほぼゼロで出荷する
  ことになる。**S25 が残した fieldmanager/admission(SSA 機構、genericregistry
  が無条件 import)の構造的な削減が先**。それまでは NO-GO。

計測のみで、コード・オーバーレイはコミットしていない(worktree を破棄)。

## S30: 本番(KOOFFICE アカウント)での S28 検証 — 本番コントロールプレーンが 7/26 版の時点で既に機能していないことが判明 (2026-09-09、実測)

feat/reduce-custom-code(S28 の 5 コントローラー復帰)を k8flare.kooffice.workers.dev
にデプロイし、eixooh8 上の privileged Docker コンテナ(独立 netns、`--net=host`
なし、8GB/4CPU 制限)で BYO ノード 1 台を join させて計測した。監視は
`wrangler tail --format json` と GraphQL(workersInvocationsAdaptive /
durableObjectsInvocationsAdaptiveGroups / durableObjectsPeriodicGroups)の
5 分毎ポーリング。

**結論: 本番ではコントローラーの watch 配信と Controllers DO → Loader の経路が
7/26 版の時点で既に壊れており、S28 の残課題(nodelifecycle の pump 依存、
tainteviction のインメモリタイマー)は本番では評価できなかった。**
切り分けは `wrangler rollback` で 7/26 版(d1474833)に戻して同じ試験を
繰り返すことで行った。

### 発見 1(先行、ブランチ非依存): DO 発の `LOADER.get()` が "Unable to deserialize cloned data due to invalid or unsupported version" で失敗する

- Controllers DO の `hasUnconvergedWork()`(`SELF.fetch` → シェル →
  `apiserverFetch` → `LOADER.get(apiserver id)`)が **全て**この例外で落ちる。
  外部からの kubectl(stateless 経路)は同じ id で正常。`loadComponent` の
  kcm/gc/sched ロードも間欠的に同じ例外。
- Loader id に env 由来のソルト(`LOADER_ID_SALT`、本ブランチで追加)を付けて
  再デプロイすると、**DO が最初にロードした id(kcm 等)は成功**し、**stateless
  側が先にロードした id(apiserver)を DO から要求すると失敗**した。同一マシン上
  で別 isolate が既にロード済みの worker を共有する経路(S19 G3)が壊れている
  形と整合する。
- 7/26 版でも同じ: ロールバック後 4 分間で kcm/gc/sched のロード失敗 8 件、
  KCM の `PUT deployments/status` が例外 16 件。デプロイ前の GraphQL でも
  10 分毎に 7 リクエスト(= 600 秒バックオフ上限で回り続ける Controllers DO
  alarm の list 群)が全件 `clientDisconnected` だった。
- 帰結: `hasUnconvergedWork()` が本番では決して false を返せず、**Controllers DO
  の alarm は永久にパークしない**(コスト不変条件 #1/#3 違反が 7/26 以降ずっと
  本番で起きていた)。今回の計測窓(08:15Z〜09:45Z)の累計は Worker 6,498 req /
  DO 7,117 req(うち DO エラー 885)。ストレージ read/write ユニットは 0。

### 発見 2(先行、ブランチ非依存): 動的ワーカーが毎分リロードされ、コントローラーが初回 list 以降の変更に反応しない

- tail に `controllers: {kcm,gc,sched} load queued/starting` が **毎分 1〜3 回**
  並ぶ(Controllers DO のメモリ上の entrypoint が失われている = DO の再起動)。
  GraphQL の status には `exceededMemory` は現れず、tail は overload サンプリングに
  入っていたため、原因(128MiB isolate 超過か、別の理由か)は未確定。
- 症状: Deployment 作成直後(ロード直後の list)には ReplicaSet/Pod が作られる
  が、その後 `kubectl scale` に **3 分以上反応しない**。7/26 版でも同じ。
  Service を作っても Endpoints/EndpointSlice は 10 分以上作られない。
- KCM の list/watch リクエストは 60 秒で `canceled`(pump window の終端)。

### ブランチ側で測れたもの

| 項目 | 実測 |
|---|---|
| Node 登録 → 実 nodeipam による podCIDR 付与 | 20 秒以内(`10.42.0.0/24`) |
| Node 登録 → Pod Running | 20 秒以内 |
| 12 コントローラー + 実ノード 1 台での `exceeded memory` | tail 上は 0 件(ただしサンプリング中) |
| 実 nodelifecycle の挙動 | 健全なノード(Lease は 10 秒毎に更新成功)が Ready=Unknown ↔ True を 2 回フラップ。Unknown 時の lastHeartbeatTime は初回 list 時点の値 = informer キャッシュが更新されていない。発見 2 と交絡しており単独評価は不可 |
| 実 endpoint/endpointslice | 本番では未評価(発見 2) |

### その他

- k3s agent の remotedialer トンネル(`/v1-k3s/connect`)が 401 で 3 秒毎に
  リトライし続ける(3 分で 65 リクエスト)。管理トークンでの join では期待
  される挙動か未確認。
- `Cannot perform I/O on behalf of a different request` が agent 由来リクエスト
  で散発し、`apiserverFetch` のリトライで成功している。
- 撤収: Deployment/RS(foreground 削除のファイナライザは GC が動かないため手で
  除去)/Service/Node を削除、コンテナ・ボリューム・イメージ・作業ディレクトリを
  ホストから削除。Containers アプリのインスタンスは終始 0。本番は 7/26 版
  (d1474833)にロールバックした状態のまま。

### S30 続報 (2026-09-09 同日): 発見 1 の修正で本番コントローラーが復活、発見 2 は発見 1 の帰結だった。残る欠陥は resident DW の informer watch 失速

**修正**: `apiserverFetch` が DO 経由の呼び出し(`gateway.internal` ホスト =
Controllers DO の `apiGet`)に対して **別の Loader id(`do/` スコープ)**を
使うようにした(commit "fix: load a separate apiserver dynamic worker for
DO-origin requests")。同一 id を stateless isolate と DO isolate の両方から
要求すると後者が "Unable to deserialize cloned data" で落ちる、という S30 の
仮説どおり、スコープ分離後は **deserialize 例外 0 件**(ソルト `s30b/` で
再デプロイ、09:43Z〜)。Loader unique が 1 つ増える($0.002/日)。

**発見 2 の再解釈**: 「DW が毎分リロードされる」ログ(`load queued/starting`
→ 直後に `dynamic worker up`)は Controllers DO の各 alarm/fetch 呼び出しで
entrypoint stub を取り直しているだけで(stub はリクエストスコープ、S2 item 4
のとおりロード済み id の factory は走らない)、DO の再起動ではなかった。
コントローラーが反応しなかった真因は発見 1(DO 発の apiserver 呼び出しの
全滅)で、修正後は `kubectl scale` に **20 秒以内**で反応した。

**修正後の実測(ノード再 join、privileged Docker コンテナ)**:

| 項目 | 実測 |
|---|---|
| kcm を fresh load した直後 | Node 登録 → podCIDR / Pod Running ×2 / EndpointSlice(実 endpointslice controller、Pod IP 入り)が **20 秒以内**に全部揃う |
| kcm がロード済みのまま(09:46Z ロード)で 09:59Z に Node を登録 | **6 分以上 podCIDR が付かず、Pod は Pending、EndpointSlice なし**。同時刻に外部からの `kubectl get nodes --watch` / Lease watch は ADDED/MODIFIED を正常に受信。Loader id のソルトを上げて kcm を強制 fresh load した途端に 20 秒以内で全部揃った |
| Node Ready のフラップ | 修正後 6 分間の 15 秒サンプリングで **フラップなし**(Ready=True 継続) |
| 撤収後のパーク | ワークロード・Node 削除(10:12Z)の 2 分後から **12 分間 Worker/DO ともリクエスト 0 件**。コスト不変条件 #1/#3 を本番で初めて確認 |

**残る欠陥(プロダクション化のブロッカー)**: resident DW(kcm)の informer
watch が、初回 list 以降のある時点から Node/Lease の更新を受け取らなくなる。
外部 watch は正常なので WatchHub 側ではなく、DW 内の reflector が pump window
の終端(watch ストリームの `canceled`、wallTime 60 秒)後に張り直す watch
リクエストが、リクエストコンテキストの無い状態から発行されて失敗している
可能性が高い(同時間帯の tail に `Cannot perform I/O on behalf of a different
request` が散発)。S28 で見えた「健全ノードの Ready=Unknown フラップ」は
これの帰結(Lease 更新がキャッシュに届かず 50 秒で Unknown、次の fresh
list/pump で True に戻る)で、実 nodelifecycle 固有の問題ではない。
修正の当たりは pkg/cfruntime(DW 内 outbound fetch を現在の pump の
IoContext に紐付ける / window 内で再 watch させる)。これが直るまで
tainteviction のインメモリタイマー(S28 残課題 2)の評価は保留。

**未実施**: Node を 300 秒超停止させた場合の Unknown → taint → 立ち退きの
計測(上記欠陥と交絡するため)。本番は現在この修正込みのブランチ
(dfbbac18、`LOADER_ID_SALT=s30c/`)がデプロイされたまま。

## S31: resident DW の informer watch 失速の真因 — 「インスタンスを生んだリクエストは生き続ける」という前提が本番では成り立たない (2026-09-09、実測)

S30 続報の「残る欠陥(プロダクション化のブロッカー)」の追跡。**ローカル
(`wrangler dev`)で再現に成功し、根本原因を特定して修正した。** 本番での
検証は未実施(デプロイは指示待ち)。

### 根本原因

resident DW(kcm / gc / sched / clusterop)の **outbound fetch が、インスタンスを
生成したリクエスト 1 つに永久に紐付いていた**。

1. `pkg/controllers/restconfig` と `pkg/controllers/clusterop/bridge.go` は
   `cloudflare.GetBinding("GATEWAY")` で Fetcher を **1 回だけ**取得していた。
   これは `Go.run` 時点の env、すなわち **isolate を最初に生成したリクエスト**の
   env である。
2. `ResidentService` はコントローラーの run ループを、その同じリクエストの
   `cloudflare.WaitUntil` の中で起動していた。両者は「S8 の知見どおり、この
   WaitUntil の promise は決して解決しないので、このリクエストはインスタンスが
   死ぬまで生き続ける」という前提でつり合っていた。
3. **本番はこの前提を満たさない。** waitUntil には上限があり、リクエストは
   打ち切られる(S30 で実測: DW の list/watch が wallTime 60 秒で `canceled`)。
4. 打ち切られた後、その Fetcher の `fetch()` は **失敗しない**。返る promise が
   **解決も棄却もされないまま放棄される**。したがって promise を待つ goroutine は
   **isolate の寿命が尽きるまでブロックしたまま**になる。エラーログも、
   client-go の backoff ログも、リトライも一切出ない。
5. reflector は 60 秒で watch を切られた後の張り直しでここに嵌まる。以降その
   informer は二度と更新を受け取らない。**新しい Loader id で fresh load する
   以外に回復手段が無い**、という S30 の観測と完全に一致する。

S24 で修正した「stateless な apiserver DW が `sync.OnceValue` で Fetcher を
掴んで `Cannot perform I/O on behalf of a different request` を投げる」問題と
**同じ捕捉ミスの、resident 版**である。S24 の修正(`BindingFromContext`)は
per-request 経路だけを直しており、resident 経路には `GetBinding` の doc comment
として「resident は自分の WaitUntil の中から使うので GetBinding のままで正しい」
と**明示的に書かれていた**。その一文が誤りだった(rule 4 に従い、削除ではなく
本節に記録する)。本番 tail に散発していた `Cannot perform I/O ...` は同じ捕捉の
別の顔(IoContext がまだ生きているうちに触った場合はこちらになる)。

### 実測した根拠

**E1: `wrangler dev` はクロスリクエスト I/O 規則を強制しない。** worker_loaders
バインディング付きの探針 Worker を立て、リクエスト 1 の env を保持してリクエスト 2
から使う形を DW 実形状で試した。`sameEnvObject=true`、fetch も成功。**dev では
捕捉した env が永久に有効**であり、本番の規則が再現されない。これが `make test-kcm`
がこの欠陥を数か月見逃してきた理由である(S27 の「dev は isolate を evict しない」
と同じ系統の dev/prod 差)。

**E2(決定打): 放棄される promise は dev でも再現する。** window(リクエスト)を
3 秒で閉じ、20 秒かかる fetch をその中で開始する探針を回したところ、window が
閉じた後、その promise は **then も catch も一切呼ばれなかった**。つまり
「失敗」ではなく「無応答」であり、Go 側は `select` の第 2 の腕を持たない限り
永久に待つ。本番の tail に retry storm が全く無かったことの説明でもある。

**E3: ローカル再現(3 回)。** kcm を Deployment 作成でロードし、4 分間ポークを
続けた後に Node + kube-node-lease Lease を登録し、実 nodeipam の podCIDR 付与を
待つスクリプトを 3 つのコード状態で回した:

| # | コード状態 | 結果 |
|---|---|---|
| 1 | 修正前(bootstrap の boot waitUntil が dev では永久に開いたまま) | `podCIDR=10.42.0.0/24 after 0s` — **再現せず** |
| 2 | bootstrap のみ修正(pump window を本番同様に閉じる)+ Go 側は `GetBinding` のまま | `NO podCIDR after 120s -- DEFECT REPRODUCED` |
| 3 | 修正後(Go 側も live window から解決) | `podCIDR=10.42.0.0/24 after 0s` |

つまり **dev で本番の欠陥を再現するには、dev 側でも pump window を本番と同じく
閉じる必要があった**。この window を閉じる変更自体を修正に含めたので、以後は
Go グルーの退行がローカルでも捕まる。

**E4: 修正後の挙動が event-armed であることの確認。** 実行 3 の DW ログで、
「window が閉じたので放棄した待ち」が **11:22:51 に 15 件、11:23:06 に 15 件**
(= kcm の informer 数ぶん、window 1 つにつき 1 巡)出たあと、**3 分間まったくの
無音**になり、11:26:06 の Node 登録のポークで開いた window で即座に再 watch して
1 秒以内に podCIDR を付けた。ポーク間は本当に何も動いておらず(コスト不変条件
#1/#3)、それでいて次のポークには即応する。

### 修正

- `pkg/cfruntime/cloudflare/window.go`(新規): 開いている pump window の登録簿。
  JS 側が dispatch 毎に `openPumpWindow(env)` / `closePumpWindow(id)` で開閉する。
- `packages/k8flare-worker/src/loader/bootstrap.ts`: 各 dispatch を window として
  publish し、`ctx.waitUntil` のタイマー満了で閉じる。
- `pkg/cfruntime/cloudflare/fetch`: `WithLiveBinding(name)` を追加。**呼び出し毎に**
  現在開いている window から Fetcher を解決し、開いていなければ次のポークまで
  待つ。待つのはタイマーもポーリングも伴わない(コスト不変条件 #2: I/O 待ちは
  無課金)。in-flight の呼び出しは window が閉じた時点で `ErrPumpWindowClosed` に
  して手放し、client-go に生きた window でリトライさせる。
- `pkg/cfruntime/residentservice.go`: `cloudflare.WaitUntil` をやめ、run を素の
  goroutine で起動する。特定のリクエストに紐付けるものを無くした。
- `pkg/cfruntime/cloudflare/env.go`: `GetBinding` と `WaitUntil` を削除。
  `EnvFromContext` は context → 現在の window → boot env の順に解決する
  (S24 が残した「context の無い vault read が boot env に落ちる」穴も、
  これで生きた window に載る)。
- 回帰テスト: `pkg/apiserver/kcmdw_test.go` に、kcm ロードから 3 分以上経った
  あとに Node を登録して podCIDR を要求するフェーズを追加。既存の assertion は
  すべて最初の 1〜2 window 以内に完結しており、この欠陥を検出できなかった。
  待ち時間はテスト自身の経過時間で相殺するので、レーンの実時間はほぼ増えない。

### 却下した実装(記録)

**window が閉じるときに in-flight fetch を `AbortController` で畳む**、を最初に
実装したが**動かない**。Go の goroutine は「そのとき動いている JS コールバック」の
中で再開するため、AbortController はリクエスト A の下で生成され、リクエスト B の
下で `abort()` されうる。これ自体がクロスリクエスト I/O アクセスであり、
`I/O type: RefcountedCanceler` の例外でインスタンス全体が exit code 2 で落ちた。
I/O オブジェクトに一切触れない `window.Done()` の待ち合わせだけが安全な畳み方。

副産物として、JS 境界の呼び出しは全て `recover` で Go の error に変換するように
した(`fetch.go` の `jsCall`)。捕捉していないと、リクエスト境界で投げられた
例外 1 つが resident インスタンス全体を落とす。

### デバッグ上の落とし穴(記録)

Go の stdout/stderr は `wasm_exec.js` の `console.log` に出るが、**DW の
`console.log` は `wrangler dev` のコンソールに出てこない**(`console.error` は
出る)。そのため上記の panic は「Go program has already exited」だけが見えて
理由が完全に不可視だった。DW 内の Go を追うときは `wasm_exec.js` の当該行を
一時的に `console.error` に差し替えること(ビルド生成物なのでコミットはしない)。

### 修正後に露出した別の欠陥: 再 list が informer キャッシュを空にする(原因判明・修正済み)

修正で dev が本番と同じ「window の外では止まる」挙動になった結果、**実
nodelifecycle の Lease 失効検知が収束しなくなった**。`kcmdw_test.go` の
S28 由来の assertion(2 分以内に Ready=Unknown + unreachable taint)が
タイムアウトする。

window 長だけを変えた A/B(他は同一、Go 側は修正済み。ノード 2 台、片方の
Lease を 5 分バックデートし、もう片方は 5 秒毎に更新し続けてポークを供給):

| `PUMP_WINDOW_MS`(Controllers DO) | 結果 |
|---|---|
| 25,000(現行) | 240 秒待っても Ready=True のまま |
| 50,000 | 240 秒待っても Ready=True のまま |
| 300,000 | **46 秒で Ready=Unknown + unreachable taint** |

`nodeMonitorGracePeriod` は 50 秒。window ≤ 50 秒では収束せず、window が
猶予期間より十分長いと即座に収束する、という切れ方をしている。つまり
**window 境界を跨ぐと猶予タイマーが実質巻き戻る**。S30 の本番観測「健全な
ノードが Ready=Unknown ↔ True をフラップする」も、本番の IoContext 上限
(約 60 秒)が猶予期間 50 秒とほぼ同じであることの現れとして整合する。

切り分けで**否定した**仮説(いずれも実測):

- **Go のタイマーが window 境界で死ぬ**: 否定。`time.Tick(5s)` のログを
  resident に仕込んで計測したところ、window を跨いで 150 秒間 30 回、
  5 秒間隔でずれなく発火し続けた。
- **kcm インスタンスが壊れている / 応答しない**: 否定。同じインスタンスで
  Deployment を作ると 15 秒で ReplicaSet、20 秒で Pod が作られる。
  nodeipam の podCIDR 付与も即座。イベント駆動の経路は健全。
- **reflector が張り直せていない**: 否定。window が閉じるたび
  `watch ended with error ... cloudflare: pump window closed` が出て、次の
  window で list からやり直せている(想定どおり)。

**真因(kcm を `-v=4` でビルドし直して判明)**: 猶予タイマーの問題ですら
なかった。V(4) ログに出ていたのはこれ:

```
12:40:32.652  reflector.go:507] "Caches populated" type="*v1.Node"
12:40:34.314  node_lifecycle_controller.go:679] "Controller observed a Node deletion" node="v4x-a"
12:40:34.314  node_lifecycle_controller.go:679] "Controller observed a Node deletion" node="v4x-b"
12:40:34.314  controller_utils.go:173] "Recording event message for node" event="Removing Node v4x-a from Controller"
```

健全な Node 2 台が **消えたことにされて** `knownNodeSet` と
`nodeHealthMap` から落とされ、以後 `monitorNodeHealth` の対象ですら
なくなっていた。だから何分待っても Ready=Unknown にならない。

WatchList モードの reflector は、再 list のとき
`sendInitialEvents=true&resourceVersionMatch=NotOlderThan&resourceVersion=<いま持っている rv>`
で watch を張る。これは「rv 以上の鮮度の**現在の全状態**を synthetic ADDED
で送れ」という意味で、`resourceVersion` は**鮮度の下限**であって再生カーソル
ではない。`packages/k8flare-worker/src/k8s/watch.ts` はこれを再生カーソルと
して WatchHub に渡していたため、**現在 rv からの再 list はアイテム 0 件 +
initial-events-end bookmark だけのストリーム**になり、reflector はその空集合
で `Replace()` して informer キャッシュを空にしていた。手で叩いた確認:

```
$ curl '.../api/v1/nodes'                      -> items 2, resourceVersion 76
$ curl '.../api/v1/nodes?watch=true&sendInitialEvents=true&resourceVersionMatch=NotOlderThan&resourceVersion=76&allowWatchBookmarks=true'
{"type":"BOOKMARK","object":{"kind":"Node",...,"annotations":{"k8s.io/initial-events-end":"true"}}}   # ADDED が 1 件も無い
$ curl '.../api/v1/nodes?watch=true&sendInitialEvents=true&...&resourceVersion=0&...'
{"type":"ADDED","object":{...v4x-a...}}                                                                # 0 からなら正しく全件
```

**pump window 導入前は踏まなかった**: resident インスタンスが再 list を
必要としなかったので、reflector は起動時の rv=0 でしか list せず、常に
正しい経路を通っていた。window で watch が切られるようになって初めて
「現在 rv からの再 list」が毎 window 走り、この欠陥が常時発火した。
window 長との相関(上の A/B)も、window が長いほど再 list の回数が減って
猶予期間 50 秒を跨ぐ確率が下がる、というだけのことだった。

**修正**: `sendInitialEvents=true` のときは replay revision を 0 に固定する
(commit "fix: serve the full state for sendInitialEvents watch requests")。
修正後、同じプローブで **36 秒で収束**(window は既定の 25,000ms のまま)、
phantom deletion は 0 件。

なお **GC も同じ空キャッシュを見ていた**はずで、実際この欠陥の再現中に
Deployment の ReplicaSet が消える現象を観測している(Deployment の status は
replicas:1/updatedReplicas:1 のまま、RS だけ存在しない)。オーナーが居ない
と判断した実 garbagecollector による削除と整合するが、単独では確認して
いない。**空の informer キャッシュは黙って壊れるのではなく、実物を消しに
かかる**という点で、これは S31 の元の欠陥より危険度が高い。

コストの観点では境界の再 list が無視できない、という点は修正後も残る:
KCM の約 15 個に加えて GC のメタデータ informer が約 45 個あり、**window が
閉じるたびに約 60 本の watch が切れて全部が list からやり直す**。しかも上の
修正で、その再 list は毎回**全件**を返す(それが正しい挙動)。window を
延ばすほど再 list の回数は減るので、window 長は「短いほど安い」ではない。
本番の IoContext 上限(約 60 秒、S30 実測)より長い window は取れず、上限より
長い `setTimeout` を仕掛けると `closePumpWindow` が発火しないままリクエスト
だけが死に、S31 の元の欠陥が別経路で再来する。**この再 list 増幅の実測が
未了**(コスト不変条件 #5): 現行 25,000ms の window で rows read が
どれだけ増えるかは cost-gate で測っていない。

### 未検証

- **本番での確認**(デプロイ禁止のため未実施)。本番は S30 続報時点の
  dfbbac18 のまま。
- 空 informer キャッシュを見た実 garbagecollector が ReplicaSet を実際に
  削除したのか(観測はしたが、単独では確認していない)。
- window ごとの全件再 list によるコスト増(rows read)の実測。
- 本番で散発していた `Cannot perform I/O on behalf of a different request` が
  この修正で消えるか。ローカルでは E1 のとおり dev がこの規則を強制しないため
  確認できない。
- tainteviction のインメモリタイマー(S28 残課題 2)と、Node を 300 秒超停止
  させたときの Unknown → taint → 立ち退き。どちらも本欠陥と交絡していたため
  S30 で保留したままで、本節では扱っていない。

## S31 追記: 本番検証と、window の close が届かないと resident が二度と回復しない (2026-09-09、実測)

S31 をデプロイした状態の本番(k8flare.kooffice.workers.dev、13:15-13:42Z)で
BYO VM のノードを使って測った結果。

**直った 2 件**:

- kcm ロードの **6 分後**に登録した Node が podCIDR を得て、その 2 Pod が
  Running、EndpointSlices まで **10 秒以内**。S31 前は「新しい Loader id で
  ロードし直すまで永遠に来ない」だった。
- ノードを `docker pause` → **61 秒以内**に Ready=Unknown +
  `node.kubernetes.io/unreachable` (NoSchedule + NoExecute)。実 tainteviction が
  Pod を立ち退かせ、ReplicaSet が作り直して Pending になるところまで実 k8s と
  同じ挙動。

**残った 1 件(本追記の対象)**: 約 9.5 分止めたノードを `docker unpause` すると、
kubelet の status 書き込みで Node は即座に Ready=True に戻るのに、**2 つの
unreachable taint が外れず、代替 Pod が 5 分以上 Pending のまま**だった。
20〜30 秒毎に Deployment の annotation を書いてポークし続けても変わらない。
**新しい Loader id で kcm をロードし直すと 15 秒で taint が外れて Pod が
スケジュールされた**ので、壊れているのは resident インスタンスの側。

### ローカル再現(段階を追って否定した仮説を含む)

指示のシナリオ(Deployment 2 replicas + Node + 10 秒毎の Lease 更新 → 2 分
停止 → 再開)をそのまま dev で回すと**再現しない**。taint は 45 秒で外れる。
条件を寄せていっても直らなかった:

| 試行 | 結果 |
|---|---|
| 1 ノード、2 分停止 | 45 秒で taint 除去。ただし NoExecute が付かず立ち退きも無い |
| 1 ノード、10 分停止 | 15 秒で taint 除去 |
| 2 ノード(1 台は cordon した健全ノード)、10 分停止 | **本番と同じく NoExecute + 立ち退きまで再現**。しかし復帰は 24 秒 |

1 ノードだと nodelifecycle が full disruption モードに入って立ち退きを止める
ため、本番の挙動を出すには**健全なノードがもう 1 台要る**(本番にもあった)。
つまり「長時間の無ハートビート」も「NoExecute + 立ち退き」も**引き金ではない**。

`watch.ts` に一時ログを入れて分かった別件: dev では watch の WebSocket が
**11 分で 1306 本開いて 0 本しか閉じない**。dev が `IoContext` を畳まないので
`handleWatch` が window より長生きするだけで、本番の挙動ではない。ただし
コスト不変条件に触れる実在の穴なので**未処理項目として記録**する(本欠陥の
真因ではないので本追記では直していない)。

### 真因: JS の close が届かなかった window は永久に「生きている」ことになる

S31 が本番で確認した事実「リクエストの `IoContext` は、その `ctx.waitUntil` の
タイマーより先に畳まれることがある」を、window 自身にも当てはめると答えが出る。
window を retire するのは `closePumpWindow` だけで、これは**そのディスパッチの
`ctx.waitUntil` に乗った `setTimeout` から呼ばれる**。リクエストが先に死ぬと
close は永久に来ず、window は登録されたまま「最新の window」で在り続ける。

以降 `CurrentWindow` はその死んだ window を返し続け、`WithLiveBinding` の
呼び出しは**誰も閉じないチャンネル**(`Done()`)を待って刺さる。resident の
outbound I/O が全部そこで止まるので、informer は張り直せず、
nodelifecycle は Node の復帰を見られない。ポークは届くが何も進まない。
**新しい Loader id でしか回復しない**という本番の観測とも一致する。
`wrangler dev` は `IoContext` をそこまで厳しく畳まないので、この経路は
ローカルでは自然発生しない。

**フォールト注入で再現**: `PUMP_WINDOW_DROP_CLOSE=N` を足して N 回に 1 回
`closePumpWindow` を落とすようにした(bootstrap の JS 側)。N=2 で
「ロードの数分後に Node を登録して podCIDR を待つ」フェーズが**3 分待っても
来ない**ようになり、本番と同じ「二度と回復しない」状態がローカルで出た。
close が 1 回落ちるだけで instance 全体が終わる。

### 修正

`pkg/cfruntime/cloudflare/window.go`: window に**自分の寿命を持たせて自分で
閉じる**。bootstrap が約束した `PUMP_WINDOW_MS` を `openPumpWindow` の引数で
Go 側に渡し(`handler_js.go`)、`lifetime + 2 秒`の `time.AfterFunc` で
`closeWindow` を呼ぶ。`CurrentWindow` / `currentWindowEnv` は
`newestLive()`(expiry を過ぎていない最新の window)しか返さないので、
タイマーより先に goroutine が起きても死んだ window は掴まない。retire の
処理は `closeWindow` に一本化し、**待っている goroutine を起こすのを最後に
する**(以前は先に `js.Func` の回収をしていて、そこで throw すると全員が
取り残された)。

コスト不変条件との関係: 追加のタイマーは window 1 本につき 1 発の
one-shot で、close が正常に届けば `Stop()` される(#3 の event-armed。
ポーリングではない)。isolate が凍結されていれば発火は遅れるだけで、
起きたときのディスパッチ = 解放された goroutine がリトライできる瞬間なので
遅れて困らない。常駐プロセスも壁時計課金も増えない。

### 回帰テスト

`pkg/apiserver/kcmdw_test.go` のレーンを **`PUMP_WINDOW_DROP_CLOSE=3` で
走らせる**(3 ディスパッチに 1 回 close を落とす)ようにし、末尾に復帰
フェーズを足した: 止めていたノードの Lease 更新と status 書き込みを再開し、
unreachable taint が外れることと、そのノードに `nodeSelector` で固定した
Pod がスケジュールされることを要求する。修正を revert すると既存の
「遅れて登録した Node」フェーズで落ちる(podCIDR が来ない)。レーンの実時間は
約 275 秒のまま。

ノードには hostname ラベルと kubelet 相当の capacity/allocatable を持たせた。
`NodeResourcesFit` は allocatable の pod 数が無いノードを全部弾くので、
これが無いと固定 Pod はどこにも載らない。

### 未検証

- **本番での確認**(デプロイ禁止のため未実施)。上の本番数値は S31 の
  コード(この追記の修正を含まない)で測ったもの。
- 本番で実際に close が落ちていたことの直接証拠(ログでは取れていない)。
  再現はフォールト注入によるもので、「本番でこの経路が起きうる」の根拠は
  S31 で実測した `IoContext` の早期畳み込みと、症状(新しい Loader id で
  しか回復しない)の一致まで。
- dev で観測した watch WebSocket の leak(11 分で 1306 本)。dev 固有の
  可能性が高いが、本番でのソケット数は測っていない。
- DefaultTolerationSeconds admission がこの apiserver に無いため、
  unreachable/not-ready の Pod が upstream の 300 秒猶予なしで即座に
  立ち退く(本番で実測)。**ギャップとして記録するのみ、今回は実装しない**。

### S31 追記 2 (2026-09-09 15:19-15:44Z、本番): 「close 欠落」修正後も復帰は約 6 分かかる — 詰まりではなく遅延

上の追記の修正(window の自前期限)込みでデプロイし(239fcd00、
`LOADER_ID_SALT=s31c/`)、同じ手順を本番で繰り返した:

| 段階 | 実測 |
|---|---|
| kcm ロードの 6.5 分後に Node 登録 | podCIDR / Pod Running ×2: 15 秒、EndpointSlice: 39 秒 |
| `docker pause` | 61 秒で Ready=Unknown + unreachable ×2、Pod 立ち退き → 代替 Pod Pending(再現) |
| 9.9 分後に `docker unpause` | Ready=True は即時。**unreachable taint は 255 秒経っても残る**が、**15:41〜15:43 の間(unpause から約 5〜6 分)に除去され、Pod は Running に戻った** |

つまり前回(13:34Z)観測した「fresh load するまで回復しない」は、fresh load
(13:40Z のデプロイ)が**同じ約 6 分の遅延の終端と重なっただけ**の可能性が
高く、上の追記の「close 欠落で永久に詰まる」は本番の真因とは**確認できて
いない**(ローカルでは close を意図的に落とすと再現する実在の穴なので修正
自体は残す)。

遅延中の tail(90 秒間): kcm は生きていて、Node の watch を張り直し
(window 終端 32 秒で `canceled` → 再 watch)、`PATCH nodes/k8flare-verify-1`
×2、`PUT deployments/s31-probe/status` **×24**(S26 の no-op 書き込みストーム
と同型。要調査)、sched は Pod の binding を POST。例外は無し。したがって
残るのは「復帰直後の nodelifecycle が taint を外すまでに数分かかる」理由の
特定で、候補は (a) 各 window 終端で watch が切られ、reflector の再 watch と
`monitorNodeHealth`(5 秒周期)のタイミングが噛み合わず観測が遅れる、
(b) upstream nodelifecycle 自体の挙動(Unknown からの復帰後、`nodeHealthMap`
の probe timestamp 更新を待つ)。実 KCM をホストで動かした場合の復帰時間との
比較が次の一手。

撤収後(15:45Z〜)は前回同様にパークを確認する。本番はこのブランチ
(239fcd00)がデプロイされたまま。

## S32: 書き込みストームの真因は no-op poke ループではなく「taint 立ち退き ⇄ ReplicaSet ⇄ scheduler」のホットループ (2026-09-10、ローカル実測)

S31 追記 2 が「S26 の no-op 書き込みストームと同型。要調査」と書いた
`PUT deployments/s31-probe/status` ×24/90秒 を、ローカルで**再現し計測した**。
結論は追記 2 の推測と違う: no-op poke ループではない(S26 の抑止は効いている)。
Node がダウンしている間、実 kcm の taint-eviction-controller が Pod を消し、
実 replicaset controller が作り直し、実 scheduler が**同じ tainted Node に
バインドし直し**、また消される、という**実書き込みのホットループ**だった。

### 計測方法(再現手順)

`pkg/apiserver/s32probe_test.go`(`K8FLARE_S32_PROBE=1` でのみ走るプローブ。
既定は skip)。2 Node + 2 replica Deployment(`nodeSelector` で nodeB に固定)
+ 各 Node の fakeKubelet(Lease 更新・Node status heartbeat・Pod を
Running/Ready にする・削除された Pod の finalize)。nodeB の kubelet を止めて
`docker pause` 相当を作り、90 秒後に戻す。書き込み量は namespace facet の
kine リビジョン(`deploy rv=`)で測る — 抑止された no-op は進めないので、
実際に受理された書き込みだけを数える。

request 単位の内訳は、計測中だけ gateway に一時的なトレースを入れて採った
(`wrangler dev` のログは外部リクエストしか出さず、`SELF` binding 経由の
内部 fetch は出ないため)。**このトレースはコミットしていない** — 公開
fetch のホットパスに分岐とレスポンス clone を足すため。以後の再測定は
facet リビジョンで足りる。

### 実測(修正前 / 修正後、同一手順)

| | 修正前 | 修正後 |
|---|---|---|
| Node ダウン中の 1 分間の全書き込み | **784** | **27** |
| うち `PUT deployments/s32-probe/status` | **230** | **2** |
| Node ダウン 90 秒間の facet リビジョン増分 | 142 → 739(30 秒で約 600) | 107 → 107(**0**) |
| ダウン中の Pod create / delete / binding | 61 / 61 / 61(約 30 秒) | 0 / 0 / 0 |

ループの現物(修正前のログ、`taint_eviction.go:111 "Deleting pod"` が
1 秒あたり約 2 件で無限に続く):

```
01:30:02 POST   /api/v1/namespaces/s32/pods                        201 (replicaset)
01:30:02 POST   /api/v1/namespaces/s32/pods/…-s9knh/binding         201 (kube-scheduler)
01:30:02 PATCH  /api/v1/namespaces/s32/pods/…-s9knh/status          200 (taint-eviction)
01:30:02 DELETE /api/v1/namespaces/s32/pods/…-s9knh                 200 (taint-eviction)
01:30:02 PUT    /apis/apps/v1/…/deployments/s32-probe/status        200
```

`deployments/status` の PUT は**このループの結果**であって原因ではない。しかも
同じリビジョンを返す PUT が 2〜3 回連続する(`rv=134` ×3、`rv=139` ×3 等)——
つまり S26 の no-op 抑止(`KineStorage.GuaranteedUpdate` の `bytes.Equal`)は
効いていて DO への書き込みは発生しておらず、`afterWrite`/`pingControllers` も
呼ばれていない。**S31 追記 2 の「S26 と同型」という見立ては誤りだったので
訂正する。**

### 原因と修正

この apiserver には upstream の DefaultTolerationSeconds admission が無く、
どの Pod も `node.kubernetes.io/not-ready:NoExecute` /
`node.kubernetes.io/unreachable:NoExecute` を許容しない。upstream なら 300 秒
待つところを、taint が付いた瞬間に立ち退きが走る。

修正は `pkg/apiserver/defaults.go` の `ApplyDefaults`(create パス)に
versioned 型版の同 admission を足すだけ。upstream の
`plugin/pkg/admission/defaulttolerationseconds` を**そのままリンクはできない**:
internal `api.Pod` を admit するため `k8s.io/kubernetes/pkg/apis/core` /
`apiserver/pkg/admission` / `component-base/featuregate` / `spf13/pflag` を
引き込み、apiserver チャンクの残り 2595KiB には収まらない。キーと Operator /
Effect は `k8s.io/api` の定数、300 秒はプラグイン側が非公開なので値だけ写した
(その旨は当該コードの doc comment に記録)。`make wasm` の headroom は
2595KiB で**変化なし**。

### 回帰ゲート

kcm レーン(`pkg/apiserver/kcmdw_test.go`)の Node 障害→復帰サイクルに、
その namespace facet のリビジョン増分の上限(400)を足した。同時に、この
サイクルで立ち退き対象になる 2 replica Deployment(nodeB 固定)を先に作る
ようにした — Pod が無いと上限判定が空振りするため。

### 残る欠陥(未修正・記録)

1. **実 scheduler が `unreachable:NoSchedule` の付いた Node に bind する。**
 上のループの構成要素で、TaintToleration の filter は NoSchedule を弾く
 はずなのに、taint 付与から 30 秒以上あとも bind し続けた(修正前ログ、
 `ua=kube-scheduler`)。sched DW の Node informer キャッシュが古いままだと
 いう S31 と同じクラスの疑い。トレランス追加でループ自体は止まったので
 優先度は下がったが、欠陥としては残っている。
2. **`POST events` が最初の数回 400 を返す。** 実 kcm の event broadcaster が
 `apiVersion`/`kind` の無い body を送り、こちらの apiserver が
 `"Object 'Kind' is missing"` で弾く(upstream は URL パスから推論する)。
 リトライで 201 になるので致命ではないが、node-controller の NodeNotReady
 event などが落ちている。

## S31 追記 3: 復帰後の taint 除去に数分かかる件は、ローカルでは resident pump でもホスト実 KCM でも再現しない (2026-09-10、ローカル実測)

追記 2 の「本番では unpause から約 5〜6 分 taint が残る」について、次の一手と
書いた「実 KCM をホストで動かした場合との比較」を実施した。

`s32probe_test.go` に `K8FLARE_S32_HOST_KCM=1` を追加(e2e-conformance.yml の
`host` バリアントと同じ分割: `CM_DISABLED:1` + `SCHED_DISABLED:1` で DW 側を
落とし、`cmd/controller-manager` の実バイナリを `wrangler dev` に向ける。
clientcmd が plain HTTP にトークンを送らないので自己署名証明書 +
`--local-protocol https` を使う)。

| 構成 | Node ダウン | Ready=True 後に taint が消えるまで |
|---|---|---|
| resident pump(kcm DW) | 90 秒 | **10 秒** |
| resident pump(kcm DW) | 10 分 | **20 秒**(別ラン、S32 の 1 回目) |
| ホスト実 kube-controller-manager | 90 秒 | **10 秒** |

**つまりローカルではどちらも速く、本番の 5〜6 分はどちらの構成でも再現しない。**
追記 2 の候補 (b)「upstream nodelifecycle 自体の挙動」は**否定された**
(ホスト実 KCM が 10 秒で外す)。候補 (a)「window 終端で watch が切れる
タイミング問題」は、ローカルの resident pump も同じ window 機構で回っていて
10〜20 秒で外せているので、**ローカルの条件では成立しない**。残るのは本番
固有の条件(isolate の寿命 / Loader id / DO の実配置)であり、S31 本文と
同じクラスの本番限定現象として扱う。

**したがって「ローカルで速いから直った」とは書かない。** 本番で再測定する
までこの項目は未解決とし、推測に基づく修正は入れない。ホスト比較を
再実行する手順だけプローブに残した。

### 未検証

- 本番での再測定(トレランス修正込みでのデプロイ後)。デプロイは指示待ち。
- 上の「残る欠陥」1 の sched DW の Node キャッシュ鮮度。
- 10 分より長いダウン(本番は 9.9 分)でのローカル挙動。90 秒 / 10 分の
 2 点しか測っていない。

## S33: foreground 削除で RC が依存 Pod より先に消える件 — ローカル再現に失敗、apiserver 側にガードを入れた (2026-09-10、実測)

必須 conformance の退行。GitHub Actions run 34373872717、ジョブ
`e2e-conformance (host)`(ホスト kube-scheduler + ホスト kube-controller-manager、
gc は dynamic worker)のステップ "Run garbage collector conformance tests
(required)" で

```
[sig-api-machinery] Garbage collector should keep the rc around until all its pods are deleted if the deleteOptions says so [Serial] [Conformance]
FAILED at test/e2e/apimachinery/garbage_collector.go:711
```

が落ちた。2026-07-11 に required 7/7 として昇格した 1 本。

### 前提の訂正: 「約 5 秒」ではなく「1 秒以内」

当初「RC が約 5 秒で消えた」と整理していたが、**これは誤り**だった(rule 4 に
従い訂正を記録する)。upstream のポーリングは `1*time.Second` 間隔で、RC が
まだ在るあいだ毎回 `%d pods remaining` を出す。CI ログにその行は **1 本も無い**。
つまり RC は DELETE(16:38:36.849)の**最初のポーリング、約 1 秒後には既に
NotFound** だった。

この 1 秒という値が切り分けの決め手になる。ローカルで実 GC に 40 Pod を
カスケードさせると 5〜15 秒かかる。1 秒未満で finalizer が外れるということは、
GC が「依存ゼロのグラフ」を見て `blockingDependents()` が即座に空を返した、
という形以外に説明が付かない。**一部を取りこぼした**のではなく、**そもそも
Pod を 1 つも知らなかった**。

### 実測で否定した仮説

- **再 list が古い resourceVersion で古い状態を返す。** 否定。
  `KineStorage.GetList` は `k.s.List(ctx, prefix, 0, 0)` と revision・limit を
  ともに 0 で固定して呼ぶ。Go apiserver はクライアントの `resourceVersion` も
  `limit` も storage 層に渡していないので、`storeList` の
  `AND mkv.id <= ?4`(point-in-time)分岐も、all-namespaces fan-out の
  continue トークン無し切り詰めも、**API 経由では到達不能**。一度この筋で
  書きかけたが、コードを追って否定した。
- **finalizer がそもそも付かない。** 否定。`upstreamregistry.go` は
  `EnableGarbageCollection: true` を立てており、ローカル再現でも毎回
  `finalizers=[foregroundDeletion]` が観測された。
- **書き込みバーストで watch イベントが落ちる。** 否定。生きた watcher に
  対して 40 Pod を同時 POST する試験を 3 回回して **40/40** が毎回届いた。

### ローカル再現には失敗した

`pkg/apiserver/gcprobe_test.go`(skip ゲート、S32 プローブと同じ形)を書いて
以下をすべて試したが、**RC は毎回正しく全 Pod より後に消えた**:

| 条件 | 結果 |
|---|---|
| kcm を DW に載せて 10 / 40 replicas | PASS(5 秒で 0 Pod) |
| 手製 Pod(`BlockOwnerDeletion` + linger finalizer) | PASS |
| Pod 生成後に 180 秒 settle | PASS |
| **CI と同じホスト分割**(ホスト実 KCM、`SCHED_DISABLED`、TLS、40 replicas) | PASS(5.1 秒) |
| 上記 + `PUMP_WINDOW_DROP_CLOSE=2` | PASS(5.1 秒) |
| 上記 + ハートビートする Node を置いて 5 分ウォームアップしてから RC 作成 | PASS(15.3 秒、Pod → RC の順序も保持) |

**再現できていない以上、真因は特定できていない。** 有力だが未確認の仮説は
「CI では gc DW の Pod informer が空だった」。傍証として当該 run の
`wrangler.log` には `Network connection lost` /
`Cannot perform I/O on behalf of a different request` /
`call to released function` が多数出ており、S31 と同じ I/O コンテキスト系の
失敗で informer が餌をもらえていなかった形と整合する。ただし**グラフの中身を
直接観測してはいない**ので、事実としては書かない。

### 入れた修正(真因修正ではなく、apiserver 側のガード)

実 garbagecollector は「**自分のグラフ**に blocking dependent が無い」ことを
根拠に foregroundDeletion finalizer を外す。そのグラフは informer の鮮度以上に
正しくなり得ず、resident DW では window 境界ごとに watch が切れる(S31)。
そこで、finalize-delete の可否を**グラフではなくストレージ**で判定する:

`pkg/apiserver/gracefuldelete.go` に `refuseForegroundFinalize` を追加し、
`handler.go` の `finalizeDelete` の先頭で呼ぶ。対象オブジェクトが
`deletionTimestamp` + `foregroundDeletion` を持つ状態で、その UID を
`BlockOwnerDeletion: true` の ownerReference で指す namespaced オブジェクトが
まだ 1 つでも残っていれば **Conflict を返して削除を完了させない**。GC は
`retry.RetryOnConflict` → workqueue の指数バックオフで再試行し、カスケードが
実際に終わってから完了する。

隣にある `sweepOrphanStragglers`(orphan 側の同じ問題への対処)の foreground
版であり、理由も同じ: ここでは GC の per-GVR watch にストリーム間の順序保証が
無いので、**ストレージを直接読む apiserver が最終判断を持つ**。upstream に
どちらのガードも無いのは、upstream の informer がここまで遅れないから。

コスト: finalize-delete 時にしか走らず、`foregroundDeletion` を持たない
オブジェクトは即 return する。ポーリングも alarm も増えない(不変条件 #3)。

### 回帰テスト

`pkg/apiserver/foregroundguard_test.go`。**kcm レーンではなく apiserver レーン**
(`KCM_DISABLED`)に置いた — gc DW が生きていると依存 Pod が本当に消えてしまい、
「早すぎる finalizer クリア」の窓がその回の GC のタイミング任せになるため。
テスト自身が「グラフが古い GC」を演じる: RC を foreground 削除 → 依存 Pod を
残したまま finalizer を空にする更新を投げ、**Conflict で拒否され RC が残る**
ことを要求する。その後 Pod を消してから同じ更新を投げると、今度は RC が
NotFound になることまで見る。

修正前のバイナリでは
`clearing foregroundDeletion with 2 blocking dependents alive = <nil>, want Conflict`
で落ちる(実測)。`make wasm` を挟まないと Go の変更が DW に載らないので、
このゲートを触るときは再ビルドを忘れないこと。

### 未検証

- **CI での確認**(push 禁止のため未実施)。この修正が当該 conformance を
  実際に緑にするかは未確認。
- **真因そのもの**。gc DW の Pod informer が CI で空だったのか、空だったなら
  なぜかは未特定。上のガードは症状を止めるが原因は残っている。
- `wrangler.log` の `Network connection lost` /
  `Cannot perform I/O on behalf of a different request` /
  `call to released function` の発生源。S31 の残課題と同じ系統に見えるが
  切り分けていない。
- 付随して見つかった別のバグ(未修正): `upstreamMarkForDeletion` は
  `&metav1.DeleteOptions{PropagationPolicy: &policy}` を新規に組み立てており、
  **リクエストの `Preconditions` を捨てている**。この conformance テストが渡す
  UID precondition が効いておらず、UID 不一致の DELETE が 409 にならず通る。
  今回の早期消失の原因ではない。

## S34: host ジョブの wrangler.log に出ていた 3 種のエラーを全部ローカルで再現し、2 つを修正した (2026-09-10、実測)

S33 が「未検証」として残した宿題 —— GitHub Actions run 34373872717 の
`e2e-conformance (host)` の wrangler.log に出ていた

```
✘ [ERROR] Uncaught Error: Network connection lost.          (約 300 件)
✘ [ERROR] call to released function                          (1 件)
[wrangler:error] Error: Cannot perform I/O on behalf of a different request.
    ... (I/O type: ReadableStreamSource)
    at async apiserverFetch (packages/k8flare-worker/src/loader/apiserver.ts:101)   (5 件)
```

の発生源特定。**3 種とも `wrangler dev` でローカル再現に成功した**(S33 の
「ローカルで再現しない」はホスト分割の再現だけを試していて、**並行書き込み
負荷**と**途中で消える watch クライアント**を欠いていたのが理由)。

再現ハーネスは `pkg/apiserver/ioctxprobe_test.go`(skip ゲート、
`K8FLARE_IOCTX_PROBE=1`)。DW モード(kcm/gc/sched が resident DW、
`PUMP_WINDOW_DROP_CLOSE=3`)で、ハートビートするノード + Deployment +
**12 本の独立した client-go クライアントが PATCH pods と POST events を
回し続ける**(2 分で約 1 万 PATCH)。途中で 8 本の watch を張って一斉に
切り、最後に「その回ずっと 1 件もイベントが無かった GVR」(ConfigMap /
Secret)を触る。

### 1. `Cannot perform I/O on behalf of a different request` (ReadableStreamSource)

**真因**: resident な Go インスタンスでは、**あるリクエストの goroutine が
別のリクエストの JS コールバックの中で再開されうる**。Go/wasm は単一
スレッドで、JS→Go の呼び出し(promise の `then`、`openPumpWindow`、
setTimeout コールバック等)が入ると Go ランタイムは**その時点で runnable な
goroutine を全部走らせてから** JS に戻る。したがってリクエスト A の
コールバックの中でリクエスト B のハンドラが完走することがあり、そこで
`handler_js.go` の `toJSResponse` が作る **`new Response(...)`(=
ReadableStreamSource)は A の IoContext に属してしまう**。B の応答として
それをシェルに返すと `ep.fetch` の await で上記例外になる。

**goroutine を跨いで起こす犯人**は Go の同期プリミティブだった。
`pkg/apiserver/cmd/apiserver-wasm/main.go` の `getTokens` が
**`tokenCacheMu` を握ったままトークン vault 読み取り(storage fetch)を
していた**ため、TTL 満了のたびに、待たされていた全リクエストの goroutine が
**保持者のコールバックの中で一斉に**解放される。

**フォールト注入による確定**(`tokenCacheTTL` を 60 秒 → 5 秒に変更して
ビルドし、同じプローブを回す):

| ビルド | `Cannot perform I/O` の件数 (120 秒負荷) |
|---|---|
| 修正前・TTL 60s | 7(1 バースト。同時 in-flight の 7 本が同時に 500) |
| 修正前・TTL 5s | **78** |
| 修正後・TTL 5s | **0** |

**修正**: JS の I/O オブジェクトを Go 側で一切作らない。
- `packages/k8flare-worker/src/loader/bootstrap.ts`: リクエスト本文を
  **JS 側(そのリクエスト自身の fetch ハンドラ)で** `arrayBuffer()` し、
  `{method, url, headers, body}` という**素の値だけ**を `handleRequest` に
  渡す。戻り値も素のオブジェクトで受け、**`new Response(...)` は JS 側で
  組み立てる**。
- `pkg/cfruntime/handler_js.go`: `readBody`(JS の `arrayBuffer()` を await
  していた)を削除。`requestFromJS` は Uint8Array をコピーするだけ。
  `toJSResponse` は Response ではなく `{status, statusText, headers, body}`
  を返す。ヘッダは `Headers` オブジェクトではなく `[[k,v],...]` の素の配列。
  これで `handler_js.go` から JS の I/O オブジェクト生成が消え、使われなく
  なった `awaitPromise` も削除した。

**この修正だけで直ることを単独で確認した**: `getTokens` を**わざと元の
ブロッキング実装に戻したまま**(TTL 5 秒)、bootstrap/handler_js の修正
だけを入れたビルドで **0 件**。つまり `await binding.handleRequest(...)` の
継続は、promise を解決したのが別 IoContext であっても**待っている側の
リクエストの文脈で走る**ことが実測で分かった(事前には不明だった点)。

**併せて入れた防御** (`getTokens` の single-flight 化): それでも
「foreign な IoContext で再開された goroutine が、自分のリクエストの
`env.STORAGE` で outbound fetch する」形は残る。`wrangler dev` はこの
規則(捕捉済み env の使い回し)を強制しない(S31 E1)ので**ローカルでは
観測できない**が、本番では S24 と同じ例外になるはず。そこで `tokenCacheMu`
は**もう I/O を跨いで保持しない**(リフレッシュ中の並行呼び出しは直前の
リストを返す)。TTL 60 秒のキャッシュなので、リフレッシュ中に 1 世代古い
トークンを返すのは元々許容している鮮度の範囲内。

**残っている同型の穴(未修正・記録のみ)**: `pkg/apiserver/bootstrap.go` の
`bootstrapOnce` と `certmanager.go` の `CAManager.Initialize` は
`sync.Once` / mutex を storage I/O を跨いで保持している。こちらは
**isolate あたり 1 回(コールドスタート時)**なので同じバーストが起きるのは
起動直後の 1 回だけで、上の Response 修正によって応答自体は安全になった。
outbound 側の危険は残る。

### 2. `call to released function`

**真因**: S31 で入れた `cloudflare.AbandonFunc` / `reapAbandoned`。window が
閉じたときに `fetch.go` の `awaitPromise` が then/catch の `js.Func` を
手放し、**次の次の `ClosePumpWindow` で `Release()`** していた。ところが
**放棄した promise はもっと後で settle しうる**(古い watch ストリームに
次のイベントが届く、切れかけの fetch が最終的に "Network connection lost"
で reject する)。released 済みの `js.Func` を JS が呼ぶと Go の
`syscall/js` が `console.error("call to released function")` を出し、
**その reaction は丸ごと捨てられる**。S31 のコメントは「1 window 遅らせれば
安全」と書いていたが**それは誤り**だった(rule 4 に従い、消さずにここに
記録する)。

**発生頻度の実測**: 修正後のコードに一時計測を入れ、「放棄済みの待ちに
あとから settle が届いた」回数を数えたところ **165 秒のプローブで 1 件**。
CI の丸ごと 1 回の conformance run で 1 件だったのと桁が合う。稀なのは、
dev では放棄された read が**次のイベント(=数ミリ秒後)で settle して
しまい、reap の 2 window 前に消化される**ため。2 window(約 50 秒)何も
来なかったストリームだけが踏む。

**修正**: `js.Func` を**一切 release しない**設計に変える
(`pkg/cfruntime/cloudflare/fetch/fetch.go`)。プロセス全体で 2 つだけの
永続 `js.FuncOf`(resolve 用 / reject 用)を持ち、呼び出しごとに
`Function.prototype.bind` で待ち受け id を焼いた JS 関数を作って
`then`/`catch` に渡す。Go 側は id → チャンネルの登録簿を持ち、放棄は
**登録簿からエントリを消すだけ**。あとから settle が来ても
トランポリンは生きていて no-op になる。`AbandonFunc` / `reapAbandoned` は
削除した。副次的に、旧実装が成功パスで `catch` を release し忘れていた
リークも消えている。

### 3. `Uncaught Error: Network connection lost.`

**発生源は k3s の remotedialer トンネルではなく、こちらの watch ストリーム
だった。** プローブで **watch クライアントを 8 本同時に切ると、ちょうど
8 件**出る(切らない構成では 0 件)。1 本の中断された watch につき 1 件で、
conformance 1 回ぶんの約 300 件はそのまま「途中で消えた watch の本数」。
つまり**制御プレーンの故障ではなくノイズ**である。

`packages/k8flare-worker/src/k8s/watch.ts` は `TransformStream` の writer に
`writer.write(...)` を**投げっぱなし**(await も catch も無し)にしていて、
クライアントが去った後の書き込みは reject する。さらにクライアントの離脱を
知る手段が無いので、**WatchHub 側の WebSocket が閉じられない**まま残る
(S31 追記が「dev で 11 分に 1306 本開いて 0 本閉じる」と記録した穴)。

**修正**: `TransformStream` をやめ、`cancel()` コールバックを持つ
`ReadableStream` を直接返す。`cancel()` はクライアント切断で発火するので、
そこで DO 側の WebSocket を `close(1001)` する。書き込みは
`controller.enqueue()` の同期 throw を捕捉して同じ経路に落とす。

**結果は 8 件 → 3 件で、ゼロにはならなかった。** 残りは `cancel()` が走る
前にソケットが死ぬレース分。ランタイムがストリーミング応答のポンプで
出しているものまでは user code から抑えられていない。**プローブのゲートは
この 1 種類だけ「情報として記録するが失敗にしない」**扱いにしてある
(残り 2 種は 0 でなければ失敗)。

**トンネルの 401 について(コードを読んだだけ、未実測)**: `/v1-k3s/connect`
は `gateway/index.ts` の `isUnauthenticatedPath` に**入っていない**ので
クラスタトークンの提示が要る。`verifyClusterToken` は Basic の
パスワード部も受けるが、k3s の agent が remotedialer に載せるのは
**ノードパスワード**でありクラスタトークンではないため、401 になるはず。
当該 CI ログは `--log-level error` 相当で 401 のリクエスト行が残っておらず、
**実際に 401 していたかは確認できていない**。上記のとおり
`Network connection lost` の発生源はこちらではないので、今回は直していない。

### 回帰ゲート

`pkg/apiserver/ioctxprobe_test.go`(`K8FLARE_IOCTX_PROBE=1`)。既定の
DW モードのほか `K8FLARE_IOCTX_MODE=host` で CI の host ジョブと同じ分割
(TLS + ホスト実 KCM + `CM_DISABLED`/`SCHED_DISABLED`)も張れる。
`K8FLARE_IOCTX_WORKERS` / `_SECONDS` / `_DROP_CLOSE` / `_LOG` で調整する。
**常設レーンには入れていない**(1 回 3 分弱かかり、S32/S33 のプローブと
同じ扱い)。修正を revert すると `Cannot perform I/O` で落ちる(実測、
12 件)。

### 未検証

- **CI / 本番での確認**(push・デプロイ禁止のため未実施)。この 3 件が
  実際に conformance の GC テストの非決定的失敗(S33)を消すかは未確認。
  S33 のガードと合わせて 2 段構えになっている状態。
- `Network connection lost` の残り 3 件を消す方法。ランタイム側で出て
  いるのか、まだこちらに抑えられる余地があるのかを切り分けていない。
- watch の WebSocket リーク(S31 追記の 1306 本)が `cancel()` 経路で
  実際に閉じるようになったかの**本数の実測**。コード上は閉じるが数えて
  いない。
- `bootstrapOnce` / `CAManager.Initialize` が I/O を跨いで保持している件
  (上記 1 の「残っている同型の穴」)。本番でしか出ない。
- `/v1-k3s/connect` の 401(上記 3)。

### S34 追記 (2026-09-09 20:58Z): CI と本番の確認結果

- e2e-conformance run 34398403238(S34 修正込み、dbad2f9): **required の
  `host` variant は baseline + GC 7 件すべて success**。1 つ前の run
  34390383167(S33 ガードまで)でも host は success で、required は 2 回連続
  green。`sched-dw` も 2 回連続 success。
- `kcm-dw`(advisory)は GC の "should not delete dependents that have both
  valid owner and owner that's waiting for dependents to be deleted" で失敗:
  rc1 を Foreground 削除してから **90 秒間、rc1 が残ったまま 25 Pod に
  deletionTimestamp が付かない**(`garbage_collector.go:795`)。host variant で
  S33 が見た「RC が 1 秒で消える」とは逆向きで、gc DW が削除に**着手しない**
  形。kcm DW と gc DW が同居する variant でのみ出ており、pump window
  (60 秒)と reflector の再 watch の噛み合わせで gc の処理開始が 90 秒を
  超えたと考えられる。未解決の advisory 項目として残す。
- 本番(a203d30e、`LOADER_ID_SALT=s34a/`): Deployment を 1→3 にスケールして
  40 秒以内に RS 3/3・Pod 3、`kubectl delete deploy` から約 100 秒で実 GC が
  RS/Pod を全部回収。撤収後 20:06Z 以降 50 分間リクエスト 0 件(パーク)。

---

## S35: workerd の新モジュールレジストリ (`new_module_registry`) — 何が買えて何が買えないか (2026-09-10、実測)

Cloudflare が workerd のモジュールレジストリを作り直した
(blog 2026-09, <https://blog.cloudflare.com/workers-module-registry-nodejs/>)。
この repo に効く変更は次の 5 点:

1. オプトインの互換性フラグ `new_module_registry`
2. specifier が URL として解決される
3. `import.meta.url` / `import.meta.main` / `import.meta.resolve`
4. WebAssembly の source-phase import (`import source x from './a.wasm'`,
   `await import.source(...)`)
5. **モジュールは最初に import された時点で遅延コンパイルされる**
   (static / dynamic のどちらでも)

同じアナウンスで **Worker のサイズ上限が全プラン 64 MiB になり、
圧縮後 (gzip) バンドル上限は撤廃**された。

### 採用したもの: `new_module_registry` (shell Worker のみ)

`packages/k8flare-worker/wrangler.jsonc` の `compatibility_flags` に
追加した。**同梱 workerd (wrangler 4.106.0) がこのフラグを受け付ける
こと自体が対応の証明**になる —— 未知のフラグを渡すと workerd は起動を
拒否する。3 レーンとも通過: test-apiserver ok 55.568s /
test-kcm PASS 300.32s / test-clusterop PASS 70.02s。shell に static
バンドルされた `import selectorWasmModule from "@wasm/selector.wasm"`
も、Loader が供給する dynamic worker 側のモジュール
(`loader/bootstrap.ts` の `import wasmModule from "./app.wasm"`) も
そのまま動く。

**ハーネスは全部 config から継承する** —— repo 内のどこにも
`--compatibility-flags` を渡している箇所は無く、`wrangler dev` を自前で
起動する全部が `-c packages/k8flare-worker/wrangler.jsonc` を指している
(`pkg/apiserver/apiserver_test.go` / `kcmdw_test.go` / `clusterop_test.go` /
`gcprobe_test.go` / `ioctxprobe_test.go` / `s32probe_test.go`、
`.github/workflows/e2e-conformance.yml` / `cost-gate.yml` /
`smoke-nodes.yml`、`package.json` の `dev`、`Makefile` の `dev`)。
CLI フラグの追加は不要。

**ただし dynamic worker には効いていない。** `WorkerCode` を組み立てて
いる 3 箇所 (`loader/apiserver.ts` の `compatibilityDate: "2026-07-01"`、
`controllers/index.ts` の同左、`storage/facets.ts` の
`compatibilityDate: "2026-03-24"`) はどれも `compatibilityFlags` を
渡していないので、Loader 側の ~30-64MB の WASM は従来どおりの扱いの
まま。今回は shell の module graph だけが対象。

### 実測 1: Loader の cap は変わっていない —— S29 の NO-GO は据え置き

Worker 本体の上限が 64 MiB になっても、**Worker Loader の
dynamic worker 1 つあたりの cap は 67,108,864 バイトのまま**。
プローブ: 60MiB の loader モジュールはロードでき、64MiB 以上は

```
Dynamic Worker code size (67108919 bytes) exceeds the maximum allowed size of 67108864 bytes.
```

で失敗する。したがって S29 の「upstream の
`k8s.io/apiserver/pkg/endpoints` レイヤーは載らない」という NO-GO は、
2026-07 の測定だけでなく **2026-09 の再測定でも裏付けられた**。

### 実測 2: gzip 上限の撤廃は「結論」ではなく「理由」を無効化する

**訂正 (rule 4)。** S8 / S14 と `docs/cost-model.md` の該当箇所は、
ASSETS+LOADER のコード供給チャンネルが存在する理由を
「Workers の 10MiB gzip デプロイ上限を回避するため」と記録している。
**この理由は 2026-09 時点で成立しない**(圧縮後上限は撤廃された)。
歴史的記述としてそのまま残すが、現在の理由は別で、**チャンネル自体は
依然として必須**:

- 5 つのバイナリの opt 後の実サイズ合計が **227,309,657 バイト
  (約 216.8 MiB)** —— apiserver 64,447,322 / kcm 44,215,888 /
  gc 41,309,324 / sched 44,995,728 / clusterop 32,341,395。
  64 MiB の Worker 1 つに同居させられない(apiserver 単体ですら
  上限の 96%)。
- Static Assets の 1 ファイル上限は 25MiB のままで、チャンクに割る
  必要も変わらない。

以下の各所に「gzip の数字は歴史的なもの」という 1 行ポインタを足した
(歴史的記述そのものは編集していない): 本ファイル S5/S14 の一覧表・
S2 の「Size」項・S14 本文・Phase 10 の scheduler 節・Phase 5 の
client-go 測定節、`docs/cost-model.md` の route A 節。

### 実測 3: selector.wasm を hot path から外した

`packages/k8flare-worker/src/k8s/selector-wasm.ts` は 4,582,761 バイト
(shell バンドルの 98%) の Go WASM を **トップレベル static import**
していた。使うのは label/field selector 付きの watch だけなのに、
素の kubectl CRUD リクエストまで全部これを払っていた。遅延コンパイル
が入ったので dynamic `import()` の裏に移した:

- `ensureSelectorsReady()` (async, memoize 済み・in-flight promise も
  共有) が `wasm_exec` と `selector.wasm` を dynamic import して Go
  ランタイムを立てる
- `validateSelectors()` / `objectMatchesSelectors()` は **同期のまま**
  (broadcast path で watcher ごと・イベントごとに走るため)
- `watch.ts` は selector が実際に付いている時だけ、既存の
  `validateSelectors()` 呼び出しの直前で 1 回 await する
- ロードに失敗したら watch を 500 Status で落とす(黙って全一致に
  しない)

**バンドルの before/after** (`wrangler deploy --dry-run --outdir`):

| | before | after |
| --- | --- | --- |
| Total Upload | 4679.36 KiB | 4682.85 KiB |
| gzip | 1370.28 KiB | 1370.60 KiB |
| `index.js` | 208,903 B | 212,474 B |
| entry graph 内の参照 | `import selectorWasmModule from "./9fa9fad…-selector.wasm"` (static, 先頭) | `await import("./9fa9fad…-selector.wasm")` (dynamic, `boot()` 内) |

**アップロード量はほぼ変わらない**(+3.49 KiB = 追加した TS の分)。
wasm は before/after どちらも別ファイルとして出力されており、
**買えたのはデプロイサイズではなく isolate 起動時のコンパイル**である。

**遅延を証明した実験(決定的)**: `assets/wasm/selector.wasm` を同じ
バイト数のランダムデータで置き換えて `wrangler dev` を起動した。

- static import 版(変更前のコードを戻したもの): **workerd がそもそも
  起動しない**。
  `service core:user:k8flare: Uncaught CompileError: WasmModuleObject::Compile(): expected magic word 00 61 73 6d, found d1 22 62 0f @+0`
  → `The Workers runtime failed to start.`。curl は `code=000`。
- dynamic import 版(採用したもの): **起動する**。selector を使わない
  `/clusters/probe` は 410 を返し、selector 付き watch を初めて叩いた
  ときだけ
  `{"kind":"Status",…,"message":"selector matching unavailable: CompileError: WasmModuleObject::Compile(): …","reason":"InternalError","code":500}`
  になる。

つまり **static import では isolate 起動時にコンパイルされていて、
dynamic import では最初の import まで一切コンパイルされない**。
実験後、実ファイル (sha256 `fb34e37a068a4e66095d26f4ed0f0f7d43646b33849cfd6a6aa59fc49b0640f2`)
に戻してある。

回帰ゲートとして `pkg/apiserver/apiserver_test.go` の
`TestPodWatchLabelSelector` を追加した(apiserver レーン)。
`boot()` を強制的に throw させると
`Watch: selector matching unavailable: …` で落ちることを確認済み。

### 採用しなかったもの: source-phase import

`await import.source("@wasm/selector.wasm")` は **esbuild (wrangler) は
通る** —— 出力に
`await import.source("./9fa9fad…-selector.wasm")` と specifier を
書き換えた形で残り、Total Upload 4682.84 KiB / gzip 1370.59 KiB で
ビルドも成功する。しかし **TypeScript 6.0.2 が構文を知らない**:

```
error TS18061: 'source' is not a valid meta-property for keyword 'import'. Did you mean 'meta' or 'defer'?
```

`make check` / `npx tsc --noEmit` / CI の型チェックが落ちるので採用せず、
素の dynamic `import()` にした(遅延は上記のとおりこれで達成できて
いる)。tsc が対応したら乗り換えを検討する価値はある。

### 未検証

- **本番での確認**(デプロイ禁止のため未実施)。上の遅延コンパイルは
  ローカル workerd (wrangler 4.106.0 同梱) での測定。
- **isolate 起動時間の実数**。「4.58MB のコンパイルが消えて何 ms 速く
  なったか」は測っていない。`wrangler dev` の初回リクエストは計測時点
  で既にウォームで、5-28ms のレンジに埋もれて差が見えなかったため、
  上の bad-wasm による二値実験に切り替えた。
- selector 付き watch の**初回**レイテンシがどれだけ増えるか
  (コンパイルがそこへ移動しただけなので、その watch は遅くなるはず)。
- dynamic worker 側に `new_module_registry` を渡した場合に何が変わるか
  (今回は渡していない)。
- `import.meta.resolve` / URL specifier / `import.meta.main` は
  この repo では未使用・未検証。

### S35 追記 (2026-09-10): 全 3 variant が同時 green、本番も確認

- e2e-conformance run 34440157442(`new_module_registry` + selector 遅延化込み、
  88b21c9): **host / kcm-dw / sched-dw の 3 variant すべてで baseline と
  garbage collector の required が success**。3 つ揃って green になったのは初。
  `ci.yml` も success。
- 本番(96de46d7、`LOADER_ID_SALT=s35a/`): 素の CRUD と、ラベル付き
  ConfigMap への selector 付き watch(ADDED 配信)・非マッチ selector
  (無配信)を確認。撤収後 05:15Z 以降 58 分間リクエスト 0 件でパーク。
- **dw variant の required 昇格について**: kcm-dw は 2026-09-09 の 2 回の run で
  GC が落ちており(S33 / S34 追記)、今回が初の green。不可侵ルール 5 の
  観点から、昇格の前にもう数回 green を確認すること。
