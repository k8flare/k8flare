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
| S1 | Facets (limits, storage accounting, parallelism, alarms, delete) | verified (the 10GB shared-vs-independent boundary test alone was intentionally not run — non-blocking) | Phase 4 (storage v2) |
| S2 | Dynamic Workers Loader (bundling WASM, size limits, env bindings) | verified (local wrangler dev; production limits unconfirmed) | Phase 2 / Phase 4 |
| S3 | Containers (startup, onActivityExpired, cold start, arbitrary images, UDP, wrangler dev) | not started | Phase 5 route B / Phase 7 |
| S4 | Cloudflare Mesh (billing scope, flannel prototype, Cluster DNS replacement) | verified (desk research) | Phase 9 |
| S5 | WASM isolate singleton-ization (syumai fork) | not started | Phase 2 (apiserver) |
| S6 | R2 (PVC access isolation, S3 access from Containers) | verified (desk research + one read-only check) | Phase 8 |
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
  through the parent DO's single thread for facet *dispatch* still
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
  assumes the 10GB is *shared* (reasoning: the downside of wrongly
  assuming independent is worse). This plan (v2 rewrite), by contrast,
  assumes *not shared* (rationale above). Both are different risk
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
   `cloudflare/containers`' `docs/egress.md` claimed egress is *blocked*
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

**2026-07-02 — S6 research-time correction on Containers egress
defaults.** While researching S3 access from Containers for the S6
section above, an initial LLM-summarized read of `cloudflare/containers`'
`docs/egress.md` claimed internet access from a Container is *blocked*
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
