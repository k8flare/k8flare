# cfruntime

The Go/WASM &lt;-&gt; Cloudflare Workers runtime bridge (fetch event dispatch,
env binding access, outbound `fetch()`, and the `wasm_exec.js` bootstrap
that loads a Go WASM module into the Workers `js`/`wasm` runtime). Used by
every WASM entrypoint in this repo (`cmd/apiserver-wasm`, `cmd/kcm-wasm`,
`cmd/kcm-wasm/scheduler`) and by `pkg/controllers/restconfig.go`.

## Origin

Absorbed 2026-07-08 from [`github.com/syumai/workers`](https://github.com/syumai/workers)
v0.32.0 (MIT), previously vendored as a patched fork at
`third_party/syumai-workers-fork/` and referenced via a `go.mod` `replace`.

**Correction, same day:** the absorption above copied syumai/workers'
code structure (its `handler_js.go` dispatch shape, `wasm_exec.js`
variant, and internal `jshttp`/`jsutil`/`runtimecontext` support
packages) essentially unchanged -- which didn't actually reduce anything,
it just relocated the same design. Rewritten from scratch instead: a
minimal, purpose-built implementation sized to exactly what this repo's
four call sites need (`Serve`/`ServeNonBlock`/`Ready`, `Getenv`/
`GetBinding`/`WaitUntil`, one outbound-fetch `RoundTripper`), fully
buffered (no `io.Pipe`/`ReadableStream` bridging -- verified by reading
every handler in `pkg/apiserver`: none of them stream), with
`wasm_exec.js` no longer committed at all -- see "wasm_exec.js" below.
No code from syumai/workers remains; `LICENSE.md` was removed
accordingly.

This was never an upstream-k8s/k3s dependency question (CLAUDE.md rule
3, "prefer real upstream over reimplementation," is about the Kubernetes
ecosystem specifically) -- it's Cloudflare Workers platform glue, which
is squarely this project's own domain, and Worker-runtime-specific fixes
are exactly the kind of change that belongs here rather than upstream.

## wasm_exec.js

Not committed. `scripts/patch-wasm-exec.mjs` patches a fresh copy of
`$(go env GOROOT)/lib/wasm/wasm_exec.js` at build time
(`scripts/build-wasm-chunks.sh`), instead of hand-maintaining a full copy
that silently drifts from whatever Go version actually compiled the WASM
binary. Normalizing the stock file and the old committed variant through
prettier and diffing them showed the only semantically necessary change
is threading a `context` argument into `Go.run` and exposing it through
`globalThis` via a `Proxy` -- everything else was reformatting noise.

The patch binds exactly two functions retrieved through that Proxy back
to the real `globalThis` (`fetch`, `setTimeout`) -- deliberately narrow,
not a blanket bind:

- **`fetch`**: native `fetch()` does a receiver/brand check and throws
  "Illegal invocation" when called with the Proxy as `this` (which
  happens whenever Go code calls `js.Global().Call("fetch", ...)`, since
  `js.Global()` now resolves to the Proxy). See
  `docs/platform-verification.md`'s S8 section (failures #3/#4) for the
  original discovery, including why binding _every_ function returned
  through the trap is wrong (it breaks static-method access like
  `Array.from`).
- **`setTimeout`**: same receiver/brand check, needed by this package's
  `yieldToEventLoop` (see `handler_js.go`) -- found live while debugging
  the correction above: `resolve`/`reject.Invoke` only _schedules_
  delivery of a dispatch's result as a microtask, it doesn't run that
  delivery synchronously, so `Serve()` letting `main()` return right
  after `Invoke` races the still-pending delivery. For a handler with no
  other async work (e.g. `GET /version`), that race was lost 100% of the
  time, surfacing as a JS-side "Cannot read properties of undefined
  (reading 'exports')" crash on every retry -- the exact signature of
  failure #4 in the S8 table, but with an unrelated root cause found
  this time. `yieldToEventLoop` forces a real macrotask boundary
  (`setTimeout(fn, 0)`), which the JS spec guarantees only fires after
  the entire microtask queue -- including however many `.then()` hops it
  takes to actually deliver the value up through `bootstrap.ts`'s and
  `apiserver.ts`'s awaits -- has drained, regardless of how many hops
  that turns out to be. Verified fixed live (`wrangler dev` +
  `go test ./pkg/apiserver/...`, both previously flaky/failing,
  stable across repeated runs after the fix).

## Scope

Only the subset this repo's binaries actually import: `Serve`/
`ServeNonBlock`/`Ready` (`handler.go`, `handler_js.go`), `Getenv`/
`GetBinding`/`WaitUntil` (`cloudflare/`), and the outbound-fetch client
(`cloudflare/fetch/`, imported as `cffetch`). No `cloudflare/d1`,
`queues`, `kv`, `r2` (this repo signs R2 requests itself, see
`pkg/apiserver/r2.go`), `cache`, `sockets`, `cron`, or streaming/
`ReadableStream` bridging -- see the Origin correction above for why.

## Updating

Plain, hand-maintained Go files -- there is no generator or upstream
diff to re-apply. A future Cloudflare Workers runtime change should be
fixed here directly.
