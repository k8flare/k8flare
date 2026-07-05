# S14 — Loader + R2/ASSETS runtime code supply, and the real Loader size cap

Verified against `wrangler dev` (wrangler 4.106.0) on 2026-07-05. All claims below were exercised with real requests (curl) against a running dev session; nothing is doc-reading-only unless explicitly marked as such.

## Part 1: runtime code supply from R2/ASSETS works

Confirmed empirically (parent worker `env.LOADER.get(id, factory)` with an **async** factory that fetches from an R2 or ASSETS binding at request time, then builds the `modules` map from the fetched bytes):

- `/from-r2-js`: JS module text fetched from R2 at request time, passed as a plain string module → loads and runs correctly.
- `/from-r2-wasm`: WASM bytes fetched from R2 at request time, passed as `modules["add.wasm"] = { wasm: bytes }` → instantiates and runs correctly.
- `/from-assets`: same via the Static Assets (`ASSETS`) binding → works.
- `/direct-wasm-compile` (control): calling `WebAssembly.compile()` directly in a normal (non-Loader) Worker isolate on the same fetched bytes fails with `CompileError: WebAssembly.compile(): Wasm code generation disallowed by embedder`. Confirms Cloudflare's dynamic-code-generation restriction applies to the plain isolate; the Loader's `modules.wasm` field is the sanctioned bypass (it's treated as a real ES module import, not a runtime `compile()` call).

So: **the hypothesis "fetch code from ASSETS/R2 at runtime and hand it to the Loader to route around the 10MiB gzip deploy cap" is correct and demonstrated end to end.**

## Part 2: CORRECTION — the Loader has its own hard 64MiB cap

**Retracting a claim from the earlier s2-loader spike** (`spikes/s2-loader/FINDINGS.md` item 2, "no limit hit up to 500 MB"). That test used a **single** giant string module. Testing here with **real, full-size payloads across many modules** surfaced a hard ceiling that the single-module test never hit:

- Loading the real goscript-transpiled kube-scheduler dependency graph (`main.js`, 49.41 MB, + 65 satellite bundles, total 237,059,962 bytes, seeded into local R2 and fetched at request time) via a single `LOADER.get()` call:
  ```
  "error": "Dynamic Worker code size (237059993 bytes) exceeds the maximum allowed size of 67108864 bytes."
  ```
- Loading a real `GOOS=js GOARCH=wasm` build of `spikes/s13-kcm-lean-only` (69,067,692 bytes, built on the spot with `go build`, seeded into R2, passed via `modules["kcm.wasm"] = { wasm: bytes }`): **same error, same 67,108,864-byte ceiling.**

`67,108,864 = 64 × 1024 × 1024` exactly — this matches Cloudflare's own "64MiB raw" language for the Loader elsewhere in the docs. It is a **hard cap on the total bytes across all modules in a single `WorkerCode.modules` map**, not a per-module limit, and it is enforced before any evaluation starts (so neither payload got far enough to hit the scheduler's known pre-existing protobuf init issue or to reach WASM instantiation).

**Why did s2-loader's earlier "500MB, no problem" claim not catch this?** Two candidate explanations, not yet distinguished: (a) s2-loader used wrangler 4.77.0 vs. 4.106.0 here — the check may not have existed yet; (b) the check may only trigger once modules-map byte accounting crosses some threshold that a single-module test structurally can't exercise the same way. Whichever it is, the exact, documented-matching 64MiB figure found here should be treated as the real, production-relevant number going forward. **The s2-loader "no limit" conclusion is superseded by this finding; do not rely on it.**

### Net effect on the original hypothesis

Runtime code supply from ASSETS/R2 (Part 1) is real and still routes around the 10MiB **gzip deploy** cap. But it does not, by itself, get a 226MB scheduler or a 69MB WASM binary onto the platform as a single Dynamic Worker — the Loader imposes its own independent 64MiB ceiling that has to be satisfied by the payload itself (via splitting, deduplication, and/or minification), not just routed around by fetching at runtime.

## Part 3: how much headroom is there to get under 64MiB?

Working from the same scc-out46 (Run46) goscript-transpiled scheduler output:

- **Size breakdown**: `main.js` = 49.41 MB; the 65 satellite files sum to 176.67 MB; total 226.08 MB (237,059,962 bytes).
- **Per-file minify** (`esbuild --minify --format=esm`, one file at a time, no bundling, import/export specifiers untouched): 226.08 MB → 162.71 MB (28% smaller). Loaded via plain Node `import()`: reaches the **exact same pre-existing** `panic: runtime error: slice bounds out of range [-4:251]` protobuf-init failure, at the same file/line, as the unminified original. No new breakage, and — notably — the identifier-collision problem that had previously made the scc-splitter disable `minifyIdentifiers` (collision with injected repair-path names) **did not reoccur** in this per-file (not bundled) minify pass. Still far short of 64MiB alone.
- **Massive cross-file duplication, not just per-file bulk.** Several of the largest satellite files are near-total duplicates of each other, even across different top-level packages:
  - `core-v1-index.js` (15.02 MB) vs. `core-v1-register.gs.js` (14.96 MB): 68 diff lines.
  - `apimachinery-meta-v1-index.js` (9.11MB) vs. `...-register.gs.js` (9.09MB): 54 diff lines.
  - `apimachinery-meta-v1-conversion.gs.js` (9.21MB) vs. `...-zz_generated.conversion.gs.js` (9.24MB): 132 diff lines.
  - Even **cross-package**, `core-v1-index.js` vs. `apimachinery-meta-v1-index.js` (different top-level Go packages entirely): only 772 diff lines despite being ~9-15MB files.
  - `zstd --ultra -19 --long=27` (long-distance matching, 128MB window) over the full 226MB concatenated corpus compresses it to **3.98 MB** — a ~60x ratio. Plain gzip -9 (32KB window, can't see cross-file duplication) only gets to ~31MB, confirming the redundancy is a cross-file structural property, not just repetitive local syntax.

  This is consistent with the scc-splitter's design: to preserve circular-import evaluation order per dependency cluster (the reason it avoids one flat esbuild `--bundle`, per `spikes/s2-loader/FINDINGS.md` item 6), each satellite entry appears to carry its **entire reachable transitive closure** inlined, rather than importing a shared common chunk. That inlining is very likely the dominant cause of the 226MB total.

- **Verdict on "can this get under 64MiB?"**: not proven directly (that needs changing how the splitter chunks the dependency graph, which is goscript-fixer's Run47 territory, not this spike's), but the 60x zstd-compressibility of the raw corpus is strong quantitative evidence that the true unique content is a small fraction of 226MB — plausibly comfortably under 64MiB — **if** the splitter is changed to extract shared subgraphs into one (or a few) common satellite module(s) that are imported by reference instead of re-inlined per entry, combined with the per-file minify pass validated above (safe, no collision, ~28% free reduction on top of dedup).

## Not done in this pass (time-boxed; flagging rather than guessing)

- Actual restructuring of the splitter's chunking strategy to test the dedup hypothesis for real (requires understanding/modifying the scc-splitter tool itself — out of scope here, and goscript-fixer's Run47 is concurrently iterating on that).
- A concrete <64MiB real-payload smoke test through the Loader (item 5 of the ask) — every real subset small enough to fit under 64MiB either omits a module `main.js` directly imports (breaks resolution) or requires the splitter-level dedup above to produce one first. Deferred rather than faked with a non-representative synthetic payload.
- Production-vs-local confirmation of the exact 67,108,864-byte figure (account operations are out of scope for this spike per standing constraints); treated as authoritative because it matches Cloudflare's documented "raw 64MiB" language exactly.

## Part 4: real kube-controller-manager binary running inside a Loader-loaded Worker

`spikes/s13-kcm-lean-only`'s `GOOS=js GOARCH=wasm` build, at `-ldflags="-s -w"`, is 66,980,340 bytes — already under the 64MiB Loader cap (128,524 bytes margin), but tight. Running it through `wasm-opt -Oz --enable-bulk-memory --enable-nontrapping-float-to-int --enable-sign-ext --enable-mutable-globals` (the four feature flags are required — Go's wasm output uses `memory.copy`/`memory.fill` and `i64.trunc_sat_f64_s`, which `wasm-opt` rejects without them) brings it to 55,605,496 bytes (53.0 MiB), giving 11.5MB of headroom.

Seeded this + Go's stock `wasm_exec.js` (GOROOT/lib/wasm/wasm_exec.js, 16,992 bytes, unmodified) into local R2, fetched both at Loader-factory time, and ran via `new globalThis.Go(); WebAssembly.instantiate(kcmWasm, go.importObject); go.run(instance)` inside a Loader-loaded worker.

**Gotcha**: when the WASM module arrives via the Loader's `modules["x.wasm"] = { wasm: ArrayBuffer }` field, the import already yields a compiled `WebAssembly.Module` (per s2-loader item 1). `WebAssembly.instantiate(module, importObject)` with a `Module` first argument resolves directly to an `Instance` — **not** `{module, instance}` (that pairing shape only happens when the first argument is raw bytes). Destructuring `{ instance }` here silently gives `undefined` and `go.run(undefined)` throws `Go.run: WebAssembly.Instance expected`.

Once fixed: instantiation took 8-16ms. `go.run(instance)` does not return (real controller-manager main loop, as expected) — raced against a 15s timeout, which fired (`raceResult: "timeout"`), meaning the process kept running the whole time with no crash. Captured real klog output via a `console.log` override:

```
E0705 08:20:27.298000  cidr_allocator.go:125] "Failed to list all nodes" err="Get \"https://example.invalid/api/v1/nodes\": net/http: fetch() failed: Error: internal error; reference = ds3hct8u4qdul9r9j3ddqiui"
E0705 08:20:37.277000  cidr_allocator.go:125] "Failed to list all nodes" err="Get \"https://example.invalid/api/v1/nodes\": net/http: fetch() failed: Error: internal error; reference = ftneof12b2630hh80npcrgh9"
```

This is the real `cidr_allocator` controller's resync loop (~10s cadence, matches two firings in 15s), making real outbound calls through Go's `net/http` → `syscall/js` → Workers' native `fetch()` bridge. The error is expected/correct (the dummy target URL `https://example.invalid` baked into `spikes/s13-kcm-lean-only/main.go` isn't a real API server) — the meaningful result is that the request round-trip through the bridge works end to end and the Go error type comes back intact.

**First real proof that a genuine, unmodified `k8s.io/kubernetes/pkg/controller/*` controller loop executes inside a Cloudflare Worker**, not just a hand-written TS reimplementation.

**Not verified**: production's 128MiB per-isolate memory limit — wrangler dev doesn't enforce it, and controller-manager's actual heap/goroutine-stack footprint under this WASM build is unmeasured. Recommend measuring before treating this as production-viable, since running all 10 leaned-in controllers together on real cluster data will use meaningfully more memory than this idle/erroring smoke run did.

## Part 5: real apiserver integration attempt — systemic Events() panic blocks all 10 controllers

Pointed `spikes/s13-kcm-lean-only/main.go`'s `restclient.Config` at a real local `k8flare` stack (`npm run dev`, `http://localhost:8787`, `BearerToken: "k8flare-dev-token"`) instead of the dummy `https://example.invalid` used in Part 4, and added a `runtime.ReadMemStats`-based logger (3s tick) to measure heap usage under real load.

Running all 10 controllers: `replicaset.(*ReplicaSetController).Run` panics immediately with `leanclient: Events not implemented (unused by this repo's controllers/scheduler)` (a `pkg/leanclient/gen/corev1/corev1.go` generated stub, `DO NOT EDIT`). The panic is unrecovered at the goroutine boundary and kills the entire GOOS=js process.

Disabling replicaset and retrying: `deployment.(*DeploymentController).Run` hits the identical panic. Disabling deployment, daemon, job, cronjob, endpoint, and endpointslice, leaving only the 3 controllers that had logged real startup lines in Part 4 (nodeipam/range_allocator, node_lifecycle_controller, taint_eviction): `nodeipam.(*Controller).Run` (`node_ipam_controller.go:140`) hits the same panic.

**Conclusion: this is systemic, not specific to any one controller.** All 10 upstream controllers use client-go's standard `record.EventBroadcaster` pattern, which calls `Events().Create(...)` (or similar) as one of the first actions inside `Run()`, before any List/Watch reconciliation happens. `pkg/leanclient/gen`'s `Events()` stub assumed (incorrectly) that no controller in this set needed it. As a result, **none of the 10 controllers can survive `Run()` at all against a real (or fake) apiserver** without a working `Events()` implementation — this has nothing to do with Workers, the Loader, or WASM; it's a pre-existing `pkg/leanclient` completeness gap that this integration test surfaced for the first time.

Practical effect on this investigation: real List/Watch reconciliation against the live k8flare apiserver was never actually exercised (every combination of controllers crashes in well under 1 second), and the `runtime.ReadMemStats` logger (3s tick) never fired even once. **Retracting the implied framing of Part 4's "cidr_allocator resync loop, 2 firings in 15s" as steady-state proof** — that run (against the dummy URL) happened to have a different goroutine log a couple of lines before some other goroutine's Events() panic killed the process; it was not evidence of a stable, continuously-running reconciliation loop, just of one goroutine winning a race before the crash.

**What *is* confirmed**: client construction and controller construction succeed against a real k8flare apiserver over plain HTTP with a bearer token (informer factories build, controllers are constructed without error, `Run()` is entered and produces real per-controller startup log lines) — i.e., the wiring up to the point of `Run()` works. What's blocked is everything after that first `Run()`-time event-recording call.

**Next step (out of scope for this spike)**: `pkg/leanclient/gen` (driven by `cmd/k8flare-gen`) needs a real (or explicitly no-op/non-panicking) `Events()` implementation before any of these 10 controllers can be evaluated end-to-end against a real cluster. Memory measurement under real load remains completely unverified until that's fixed.

## Part 6: Loader nesting is not possible; the dynamic-compile restriction follows the loaded worker too

Two follow-up questions, both answered with a single `/loader-nesting` endpoint (s14 parent, `wrangler dev`, 2026-07-05):

1. **Can a `WorkerLoader` binding itself be passed through a loaded worker's `env`** (mirroring s2-loader item 3a's DO namespace/stub test)? No: `env.LOADER.get(id, () => ({ ..., env: { LOADER: env.LOADER } }))` fails at the `.get()` call itself (before the loaded worker's `fetch()` ever runs) with `DataCloneError: Could not serialize object of type "WorkerLoader". This type does not support serialization.` Same failure mode and same message shape as the DO namespace/stub case in s2-loader — `WorkerLoader` belongs to the same non-cloneable-binding-type list.
2. **Is `WebAssembly.compile()` also disallowed inside a Loader-loaded worker**, or does being dynamically loaded relax that restriction? Tested with no `env.LOADER` passthrough (isolating this from question 1): still disallowed, identical error to the plain-parent-isolate case — `CompileError: WebAssembly.compile(): Wasm code generation disallowed by embedder`.

**Implication for a multi-component design**: a single top-level Worker (one with a real, statically-declared `worker_loaders` binding in its `wrangler.jsonc`) can load several independent ≤64MiB components and wire them together via `Fetcher`s obtained from each `getEntrypoint()` call — that pattern is sound. But **the Loader capability itself cannot be delegated further down** — a loaded worker cannot become a loader of other workers, and it gets no special dynamic-WASM-compile privilege from being loaded. Any "split into N sub-64MiB Loader-loaded pieces" design has to have all N `LOADER.get()` calls made directly by the same top-level orchestrating Worker, not by one loaded piece on behalf of another.

## Repro

```
node_modules/.bin/wrangler dev -c spikes/s14-loader-external-fetch/parent/wrangler.jsonc \
  --port 8841 --persist-to spikes/s14-loader-external-fetch/.wrangler-state
# seed local R2 first (see shell history / team notes for the loop that
# populates s14-sched/sched/*.js and s14-sched/kcm.wasm from scratch)
curl localhost:8841/scheduler-load
curl localhost:8841/kcm-wasm-load
```

(Transcribed by the coordinating session on behalf of the spike agent, which cannot write report files.)
