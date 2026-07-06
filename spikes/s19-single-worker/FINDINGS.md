# S19: single-Worker consolidation feasibility — findings (2026-07-06)

Gates for merging all 6 Workers into one deploy (plan:
6→1 consolidation + multi-cluster). All measured live against
`wrangler dev` 4.106.0 on this repo's real 43MB apiserver WASM
(`gen-assets.sh` chunks it from `workers/apiserver/build/app.wasm`).
Repro: `npx wrangler dev -c spikes/s19-single-worker/wrangler.jsonc`
then curl `/g1 /g2 /g3 /g3-burst /g3-cold /g3-from-do` on :8941.

## G1 — one config holds everything: PASS

containers[] (3-tier shape mirrored by one TinyVM entry) + assets
(`run_worker_first: true`) + `worker_loaders` + sqlite DOs + TWO self
service bindings (default export + named entrypoint `ClusterLoopback`)
all coexist; dev boots, every binding resolves, DO writes work.

- Container images must `EXPOSE` a port or `wrangler dev` refuses to
  start (hard error, found live).
- **Docker-less behavior**: default is a hard startup failure, but
  `--enable-containers=false` (CLI flag; also `dev.enable_containers`
  in config) starts dev cleanly with no Docker binary at all
  (verified with `WRANGLER_DOCKER_BIN=/nonexistent`), and G2/G3 pass
  in that mode. → CI harnesses (go test, e2e-conformance, cost-gate)
  just add the flag; **no "nodes stays a second Worker" fallback
  needed.**

## G2 — self-entrypoint Fetcher through the Loader env: PASS

A named-entrypoint self service binding (`ClusterLoopback`,
header-addressed DO routing) passed as `env.STORAGE` into
`LOADER.get()`'s WorkerCode survives the env clone, and the loaded
worker's fetch reaches the DO through it (write+read round trip
verified). This is the mechanism that replaces the Go apiserver's
`CLUSTER` DO-namespace binding (DO namespaces cannot cross the clone,
S2 item 3a).

## G3 — Loader-hosted 43MB apiserver on the hot path: PASS

| Measurement | Result |
|---|---|
| Cold (chunk fetch+assemble+compile+instantiate+dispatch) | 135–195 ms |
| Warm dispatch (`/version`) | 12–24 ms |
| 30-parallel burst (kubectl-discovery shape), warm | 30/30 OK, 842 ms total, p50 449 ms |
| Same loader id from a DIFFERENT caller context (DO vs handler) | factory skipped, 12 ms — **isolate is shared** |

Two contract findings that shape the consolidation code:

1. **Loader entrypoint stubs are request-scoped I/O** — caching
   `getEntrypoint()`'s Fetcher at module scope fails on the next
   request with "Cannot perform I/O on behalf of a different request
   (SubrequestChannel)", same class as DO stubs
   (workers/storage/src/facets.ts:243-253). Correct stateless-handler
   shape: `LOADER.get(id, factory)` **per request** (factory skipped
   for a loaded id, S2 item 4); cache only plain data (the manifest).
   A Durable Object may cache the entrypoint across its own requests
   (one IoContext per DO lifetime) — which is why the Controllers DO's
   existing caching is fine.
2. **The apiserver is NOT the KCM instantiate-once shape.**
   syumai/workers' generated `worker.mjs` creates a fresh
   `Go()`+`WebAssembly.Instance` per request (module compile cached);
   the KCM-style resident bootstrap fails on dispatch 2 with "Go
   program has already exited" (observed live). The dynamic-worker
   bootstrap must mirror `worker.mjs` (per-request instantiation, no
   pump window). Warm 12–24 ms above INCLUDES that per-request Go
   boot, i.e. it is the same work today's deployed apiserver does per
   request.

Caveats: numbers are local dev (M-series). Production cold start will
be larger, but S14 already proved the Loader path in production with
the bigger 62.5MB KCM, and the existing gateway 3x GET/HEAD retry
absorber carries over unchanged. Production isolate-eviction cadence
(→ cold frequency) remains the S14 open item; unchanged by this spike.

Not exercised here: a resource path through Go → STORAGE Fetcher (the
current binary still calls `NewDurableObjectNamespace("CLUSTER")`,
which the consolidation replaces with the G2 mechanism — that switch
is implementation work, its two halves are individually proven by
G2 and by pkg/controllers/restconfig.go:50-58 in production).
