# S2 — Worker Loader / Dynamic Workers spike findings

Verified empirically against `wrangler dev` (wrangler 4.77.0, workerd compat max
2026-03-17) on 2026-07-02. Every claim below was exercised with real requests
(curl) against a running multi-worker dev session; nothing is doc-reading-only.
Spike code: `parent/` (all test endpoints) and `child-service/` (service-binding
target). The agent that ran this spike could not write files, so this document
was transcribed by the coordinating session from its full report.

Repro:

```
node_modules/.bin/wrangler dev -c spikes/s2-loader/parent/wrangler.jsonc \
  -c spikes/s2-loader/child-service/wrangler.jsonc \
  --port 8830 --inspector-port 9230 --persist-to spikes/s2-loader/.wrangler-state
# then curl /wasm  /env?fields=...  /outbound?mode=...  /size?mb=...  /counter?id=...
```

## Verdict summary

| # | Question | Verdict |
|---|---|---|
| 1 | WASM module inside a loader-loaded worker | **Yes.** `modules[k] = { wasm: ArrayBuffer }`; import yields an uninstantiated `WebAssembly.Module`. |
| 2 | Size limit of loaded code (local) | **No limit hit up to 500 MB.** Load time grows superlinearly (33 s at 500 MB). Non-issue at realistic sizes. |
| 3a | Passing bindings via `env` | **Plain values and `Fetcher` (service binding) pass. `DurableObjectNamespace` and `DurableObjectStub` both fail with `DataCloneError`.** |
| 3b | `globalOutbound` | `null` = blocked, `undefined` = real internet, real `Fetcher` = full outbound proxying; duck-typed plain object rejected with `TypeError`. |
| 4 | `get(id, factory)` caching | Confirmed: factory runs once per id (cache miss only); module state survives across requests; distinct ids fully isolated; loading a new id does not evict existing ones. |

## Details

### 1. WASM

A hand-assembled 41-byte WASM module (`add(a,b)`) was unit-tested in Node first
(`add(3,4) === 7`), then loaded:

```js
import wasmModule from "./add.wasm";
// modules: { "index.js": <source>, "add.wasm": { wasm: addWasmBytes() } }
new WebAssembly.Instance(wasmModule, {});
```

`curl /wasm` → `{"ok":true,"result":7,"moduleCtor":"Module"}` — the import is a
`WebAssembly.Module`, not auto-instantiated.

`@cloudflare/workers-types@4.20260317.1` matches observed behavior:

```ts
interface WorkerLoaderModule { js?: string; cjs?: string; text?: string; data?: ArrayBuffer; json?: any; py?: string; wasm?: ArrayBuffer; }
interface WorkerLoaderWorkerCode {
  compatibilityDate: string; compatibilityFlags?: string[]; allowExperimental?: boolean;
  mainModule: string; modules: Record<string, WorkerLoaderModule | string>;
  env?: any; globalOutbound?: Fetcher | null; tails?: Fetcher[]; streamingTails?: Fetcher[];
}
interface WorkerLoader { get(name: string|null, getCode: () => WorkerLoaderWorkerCode | Promise<...>): WorkerStub; load(code): WorkerStub; }
interface WorkerStub { getEntrypoint<T>(name?: string, options?: { props?: any }): Fetcher<T>; }
```

Important: `wasm` accepts a raw `ArrayBuffer` only — neither a base64 string nor
a precompiled `WebAssembly.Module`. The comment in
`packages/dynamic-worker/src/run.ts` ("data (ArrayBuffer) is not supported via
JSON API") applies to `wasm` as well: if the DynamicWorker CRD ever accepts
WASM, the JSON layer must carry base64 and the server must decode to
ArrayBuffer. Not tested: WASM as `mainModule` itself (only "JS main imports
wasm" was exercised).

### 2. Size (local)

JS text modules built from generated string literals, staged 1→500 MB:

| Requested | Bytes | Time | Result |
|---|---|---|---|
| 1 MB | 1,048,576 | 7 ms | ok |
| 5 MB | 5,242,880 | 31 ms | ok |
| 10 MB | 10,485,760 | 62 ms | ok |
| 20 MB | 20,971,520 | 288 ms | ok |
| 50 MB | 52,428,800 | 833 ms | ok |
| 100 MB | 104,857,600 | 2,174 ms | ok |
| 200 MB | 209,715,200 | 7,748 ms | ok |
| 500 MB | 524,288,000 | 33,468 ms | ok |

Stopped at 500 MB (local-OOM risk vs. information value). Caveat: the
superlinear curve is not attributable to the Loader itself — the test method
(one giant string literal that the JS engine must parse/compile) is an equally
likely cost driver; the two were not separated. Treat the curve shape as
indicative only. Production limits (e.g. whether the regular 10 MiB gzip
script cap applies to loaded code) remain unverified.

### 3a. `env` binding forwarding

Tested individually and in combination via `/env?fields=simple|do|svc|stub`:

- Plain string (`vars`): passes.
- Service binding (`Fetcher`, resolved via a second worker in the same
  multi-config dev session): passes, real RPC round-trip confirmed.
- `DurableObjectNamespace`: **fails before the child even runs**, at
  `.get()` / `getEntrypoint().fetch()` on the parent side:
  `DataCloneError: Could not serialize object of type "DurableObjectNamespace". This type does not support serialization.`
- `DurableObjectStub` (pre-resolved `COUNTER_DO.get(id)`): same-shaped failure:
  `DataCloneError: Could not serialize object of type "DurableObject". This type does not support serialization.`
- If any DO-typed key is present, the whole `env` clone fails (offending keys
  are not silently skipped).

Conclusion: `WorkerCode.env` crosses a structured-clone-like boundary. Plain
values and `Fetcher` RPC stubs pass; DO namespaces/stubs do not, in either
form. KV/R2/D1/Queues/AI/Vectorize/Workflows binding types were not tested.

**Design implication (the most important finding of this spike):** any
loader-loaded module (facet class, DynamicWorker) that needs to reach a DO must
be handed a `Fetcher` to a fronting Worker/RPC entrypoint instead — DO
namespaces/stubs cannot be passed through `env`.

### 3b. `globalOutbound`

| mode | value | result |
|---|---|---|
| none | `null` | Blocked: "This worker is not permitted to access the internet via global functions like fetch(). It must use capabilities (such as bindings in 'env') to talk to the outside world." |
| inherit | undefined | Real internet reachable (verified against `https://cloudflare.com/robots.txt`, HTTP 200). |
| custom-svc | real service-binding `Fetcher` | ALL child outbound fetches are diverted to the Fetcher regardless of URL (verified: child fetched an arbitrary URL, response came from child-service). |
| custom-plain | duck-typed `{ fetch: async … }` | Rejected: `TypeError: Incorrect type for the 'globalOutbound' field on 'WorkerCode': the provided value is not of type 'Fetcher'.` |

Conclusion: three real modes — block / inherit / full proxy via a genuine
`Fetcher`. A plain object is not accepted, so outbound filtering must be
implemented as a real Worker bound via `services`, not an inline JS object.

### 4. `get(id, factory)` cache & isolation

`/counter?id=X` calls `env.LOADER.get(id, factory)` freshly on every request.
Parent-side module-scope counter tracks factory invocations; child-side
module-scope counter tracks state survival:

```
GET /counter?id=A → factoryInvokedThisCall=true,  factoryCallCountTotal=1, counter=1
GET /counter?id=A → factoryInvokedThisCall=false, factoryCallCountTotal=1, counter=2
GET /counter?id=A → factoryInvokedThisCall=false, factoryCallCountTotal=1, counter=3
GET /counter?id=B → factoryInvokedThisCall=true,  factoryCallCountTotal=2, counter=1
GET /counter?id=A → factoryInvokedThisCall=false, factoryCallCountTotal=2, counter=4
```

Same id → factory only on first load, worker reused with module state intact
(even though `.get()` is called every request). Distinct ids fully independent.
Loading B did not evict A. Not tested (prod-only): idle eviction timing —
directly relevant to the content-hash-key cache-hit assumptions and to the
cost invariants (never assume residency).

### Additional observations (local dev)

- Multi-config dev (`-c parent -c child-service`) resolves service bindings in
  one process, but the startup banner's `[not connected]` status is unreliable:
  it read `local [not connected]` for the whole session while dozens of RPC
  calls succeeded. Never use the banner as a readiness signal.
- `compatibility_date: "2026-03-24"` (repo convention) falls back with a
  warning to workerd's max supported date 2026-03-17 under wrangler 4.77.0.
  Pre-existing repo-wide quirk, not introduced by this spike.

## Implications for the v2 design

1. Facet-class delivery (build-time bundled string + content-hash key) matches
   the `.get(id, …)` id model exactly (item 4).
2. The `cacheId = ${namespace}/${name}:${uid}:${resourceVersion}` pattern in
   `packages/dynamic-worker/src/run.ts` is validated by item 4 (spec changes
   bust the cache naturally).
3. Never design for DO namespace/stub in a loaded worker's `env` (item 3a) —
   use Fetcher indirection.
4. `globalOutbound: <Fetcher>` is a proven mechanism for a future
   DynamicWorker `networkAccess: "restricted"` mode (allowlist/audit/rate
   limiting in a proxy Worker).
5. Size is not a local blocker; production caps unknown.

## Remaining production-only items

- Actual module-bytes limit/quota for the Loader in production.
- Idle-eviction timing of the `.get(id, …)` cache.
- Behavior of non-Fetcher, non-DO binding types in `env` (KV/R2/D1/Queues/…).
- Untested API surface (out of scope, not broken): `WorkerLoader.load()`,
  `allowExperimental`, `compatibilityFlags`, `tails`/`streamingTails`,
  `getEntrypoint(name, {props})`.
