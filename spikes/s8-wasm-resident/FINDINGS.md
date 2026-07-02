# S8 — WASM-resident controllers spike findings

Verified 2026-07-02 in `wrangler dev` (workerd), syumai/workers v0.32.0.
Raw evidence for every claim is under `logs/`. The agent that ran this spike
could not write report files, so this document was transcribed by the
coordinating session from its full report. Context: per the 2026-07-02 user
decision, S8 selects WHICH WASM execution shape `workers/controllers` uses —
Containers is not a fallback.

## Verdict summary

| # | Question | Verdict |
|---|---|---|
| (a) | Open response stream + background goroutines, 10+ min | **Works** — 11-min soak, 329 heartbeats, zero gaps, unmodified library |
| (c) | 10–15 concurrent long-lived streams on one isolate | **Works** — 12 concurrent (14 at peak incl. soaks), all uniform, zero drops |
| (d) | Same through a TS→Go service binding | **Works** — 11-min soak through the binding, zero gaps |
| S5 | Reuse one WASM instance across independent incoming requests | **Partial with a hard limit** — state persists (1-line fork); goroutine/timer progress does not survive request boundaries |
| bonus | Outbound `net/http` from Go WASM (what informers actually do) | **Broken by default in syumai/workers v0.32.0; fixable via binding-backed `cloudflare/fetch`** |

## Background facts (verified from source)

- `workers.Serve` = `ServeNonBlock(); Ready(); <-Done()`; `Done()` closes when
  the single handled request finishes — the library assumes one request per
  program instance.
- Streaming responses are pull-based (JS `pull()` → blocking Go `io.Pipe`
  read) with no wall-clock cap (confirmed by the soaks).
- **`r.Context().Done()` is never wired to client disconnect** — only a
  failed `Write()` (`io.ErrClosedPipe`) reveals it. Real footgun for porting
  k8s controller code.
- The generated glue (`packages/worker/build/worker.mjs` — same as the real
  build) creates a fresh `WebAssembly.Instance` + `go.run()` per `fetch()`;
  only compiled module bytes are cached.
- `js.Global()` in Go resolves to a `Proxy` around `globalThis` (wasm_exec.js
  injects per-request `context` through it) — root cause of the outbound bug.

## (a) Stream-resident program — works on the unmodified library

`stock/main.go`: `GET /stream` ticks every 2 s plus an independent
informer-like goroutine ticking every 5 s. `curl -m 660`: 329 heartbeats,
`ticker_count == seq` throughout, one instance the whole way,
`informer_tick=132 ≈ 660/5`. Zero gaps.

Pitfall: `wrangler dev` silently reloads (killing in-flight streams) on ANY
watched-file change, including directory renames and wrangler.jsonc edits —
never touch watched files mid-soak.

## (c) Concurrency — 12 simultaneous streams, clean

12 concurrent 90-s streams launched together while both 11-min soaks were
running (14 open streams at peak): identical start/end, 44 heartbeats each,
distinct instance IDs, `ticker_count == seq` in every log. One caveat: a
60–90 s apparent stall correlated with host load average 10–15 (two other
spikes' wrangler dev sessions were running on the same machine); all streams
recovered with zero heartbeat loss (`ticker_count` never desynced), so it
reads as OS contention, not a workerd limit — worth one quiet-environment
re-check.

## (d) Service binding hop — streaming semantics unchanged

`relay` (TS: `env.GOWORKER.fetch(request)`) → `stock`. Same 11-min soak
through the binding: 330 heartbeats, zero gaps. Directly validates the
gateway→apiserver and controllers→apiserver shapes.

Dev pitfall: `wrangler dev -c A -c B` exposes only the FIRST config's port on
localhost; later configs are reachable only via bindings. List the
curl-target config first.

## S5 — instance reuse across independent requests: state yes, progress no

`resident/` uses a hand-written `out/worker.mjs` that instantiates WASM and
calls `go.run()` once per isolate; `main()` uses `ServeNonBlock+Ready` then
`select{}`.

1. **Upstream crash, reproduced then fixed with a 1-line fork.** Unpatched,
   request #3 dies: `panic: close of closed channel` at `handler_js.go:56`
   (package-level `doneCh` closed on every request completion). Fix in
   `vendor/syumai-workers-fork/handler_js.go`: guard with `sync.Once`
   (~10 lines). After the fix: same instance across 4 sequential requests,
   `requests_served` 1→4, uptime monotonic, no panic. `diff -rq` vs the
   module cache confirms exactly one file differs.
2. **Hard limit: timers/goroutines only progress while the request whose
   IoContext hosted `go.run()` is active.** A `main()`-level 2 s ticker
   advanced at most +1 tick per incoming request regardless of idle gap
   (9 s/12 s/31 s/57 s gaps tested). A fresh instance whose first request is
   `/stream` works perfectly — and unrelated goroutines advance while that
   stream is open, freezing the instant it closes.
3. **A non-hosting request that needs new timer progress fails
   structurally**: with zero idle gap, request #2 (`/stream`) after request
   #1 (`/status`, the instantiating one) → workerd "code had hung" error
   resolved in 2 ms (structural detection, not a timeout), leaking 2
   goroutines permanently per failure (handler on `ticker.C` + stream-pull on
   a never-satisfied pipe read).
4. Counter-intuitive: `cloudflare.WaitUntil` on a stale ExecutionContext did
   NOT throw (3/3 ok) — likely because the Go call is synchronous and the
   task never blocks. Not fully understood; possibly dev-only; do not rely on
   it without production re-testing.

**Practical reframe:** the viable resident shape is NOT "instantiate once,
serve N independent requests over time" — it is **"instantiate once and keep
one long-lived stream open for the program's entire lifetime"** (proven by
(a)). A controllers program's own always-open session is the natural host.
Consequence for the apiserver: naively applying an isolate singleton to
`workers/apiserver` (independent short requests) would hit the timer/promise
limit — keep per-request instantiation there (current, known-working
behavior) and revisit only with a design that respects this constraint.

A "complete" N-request reuse story would require rewriting wasm_exec.js's
timer scheduling so callbacks aren't bound to one request's IoContext —
large, risky, no prototype, and possibly an intentional workerd boundary.
Recommendation: don't pursue it.

## Bonus (gate-level): outbound `net/http` is broken by default

Building the informer simulation crashed a fresh instance on its first
request:

```
panic: JavaScript error: Illegal invocation: function called with incorrect `this` reference.
net/http.(*Transport).RoundTrip(...) roundtrip_js.go:129
```

- Reproduced on the completely unmodified stock variant with a one-shot
  `http.Get` — independent of instance reuse, present in syumai/workers
  v0.32.0 as this repo ships it (unhit so far because the apiserver is
  inbound-only; its DO fetches are binding-shaped).
- Also broken: `cloudflare/fetch.NewClient()` at its default
  `namespace: js.Global()` — the library's documented alternative fails with
  defaults too.
- Root cause: native `fetch` brand-checks its `this`; the wasm_exec.js
  `Proxy` fails that check.
- **Verified fix**: route outbound through a real binding —
  `cloudflare.GetBinding("SELF")` +
  `cffetch.NewClient(cffetch.WithBinding(binding))` → clean 200.
- Consequences: controllers→apiserver over a service binding works with this
  pattern (needs re-verification against the real apiserver Worker, not the
  self-binding toy). Arbitrary open-internet outbound from Go WASM has NO
  demonstrated fix (bindings only target Workers). **Before Phase 5: audit
  the embedded kube-scheduler/KCM dependency graph for any
  `http.Get`/`http.DefaultClient`/custom `*http.Transport` that bypasses the
  injected rest.Config transport — each one is an instance-killing panic.**
- Broader suspicion: ANY native JS API invoked via `js.Global().Call(...)`
  may fail the same Proxy-as-`this` check, not just `fetch`.

## Fork requirements (concrete)

`vendor/syumai-workers-fork/` = copy of syumai/workers v0.32.0, module
path unchanged, `replace` scoped to `resident/go.mod` only (root go.mod
untouched). Exactly one changed Go file: `handler_js.go` (`sync.Once`
guard); later the workers-assets-gen glue asset gained the fetch-bind
patch. Upstream's `_templates/` scaffolding was pruned from the copy
(irrelevant to the fork's purpose, and its malformed sample HTML broke
the repo-wide `vp check` formatting gate).
That single change is sufficient for the stream-resident shape; the
wasm_exec.js timer rewrite is NOT needed for it and is not recommended.

## Production-only residual items

- Real CPU-ms billing for a long-open, mostly-I/O-idle stream (S8(b) — needs
  a deploy + `wrangler tail`/dashboard; the key cost-model number).
- Real connection/subrequest limits and isolate eviction under concurrency.
- Whether WaitUntil-on-stale-context holds in production workerd.
- The binding-backed outbound fix against the real apiserver Worker at scale.
- Production idle-isolate eviction vs the "one open stream keeps it alive"
  assumption.

## Other pitfalls worth remembering

- Unrecovered goroutine panics kill the whole Go program; any shared/reused
  instance needs `recover()` wrapping every goroutine.
- Bash `&` combined with a harness background flag orphans processes (SIGHUP)
  — use one or the other.

## Files

```
stock/     unmodified-library variant (main.go, go.mod/sum, wrangler.jsonc, out/ glue)
resident/  singleton variant (hand-written out/worker.mjs, forked lib via replace)
relay/     TS service-binding relay
vendor/syumai-workers-fork/   full copy; only handler_js.go differs
logs/      raw soak/concurrency/crash evidence
```

Repro commands are in the logs directory alongside the evidence; the key
ones: build with `workers-assets-gen -mode=go -o out` +
`GOOS=js GOARCH=wasm go build -o ./out/app.wasm .`, then
`wrangler dev --config <variant>/wrangler.jsonc` and curl
`/stream`, `/status`, `/waituntil`, `/outbound-test`, `/cf-fetch-test`,
`/binding-fetch-test`.
