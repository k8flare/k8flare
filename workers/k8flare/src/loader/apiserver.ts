import type { Env } from "../env.ts";
import { CLUSTER_HEADER } from "../entrypoints.ts";
import { makePerRequestBootstrapJS } from "./bootstrap.ts";
import { assembleWasm, fetchWasmAsset, fetchWasmManifest, type WasmManifest } from "./chunks.ts";

// The Go apiserver as a Loader dynamic worker (S19 G3). Contract notes,
// all verified live by spikes/s19-single-worker:
//  - LOADER.get() runs PER REQUEST here: entrypoint stubs are
//    request-scoped I/O, so only plain data (the manifest) may be cached
//    at module scope. A loaded id skips the factory (S2 item 4), and the
//    dynamic-worker isolate is shared across caller contexts, so warm
//    dispatches cost ~12-24ms including the per-request Go boot the old
//    deployed worker.mjs did anyway.
//  - env passes only plain values + Fetchers: STORAGE (ClusterLoopback,
//    the Cluster DO route) and the K3S_TOKEN/R2_* vars main.go reads.
let manifestCache: WasmManifest | null = null;

async function apiserverEntrypoint(env: Env): Promise<Fetcher> {
  if (!manifestCache) {
    manifestCache = await fetchWasmManifest(env.ASSETS, "apiserver");
    if (!manifestCache) {
      throw new Error(
        "apiserver.manifest.json not shipped -- run npm run build:wasm before dev/deploy",
      );
    }
  }
  const manifest = manifestCache;
  const worker = env.LOADER.get(`apiserver@${manifest.sha256}`, async () => {
    const wasm = await assembleWasm(env.ASSETS, manifest);
    const wasmExec = await fetchWasmAsset(env.ASSETS, "wasm_exec.js").then((r) => r.text());
    const dynamicEnv: Record<string, unknown> = { STORAGE: env.STORAGE };
    for (const k of [
      "K3S_TOKEN",
      "R2_ACCOUNT_ID",
      "R2_ACCESS_KEY_ID",
      "R2_SECRET_ACCESS_KEY",
      "R2_BUCKET",
    ] as const) {
      if (env[k]) dynamicEnv[k] = env[k];
    }
    return {
      compatibilityDate: "2026-07-01",
      mainModule: "index.js",
      modules: {
        "index.js": makePerRequestBootstrapJS(),
        "wasm_exec.js": wasmExec,
        "app.wasm": { wasm: wasm.buffer as ArrayBuffer },
      },
      env: dynamicEnv,
    };
  });
  return worker.getEntrypoint();
}

// apiserverFetch replaces the former gateway->APISERVER service binding.
// Keeps the cold-start absorber verbatim from the old gateway
// (workers/gateway history: kubectl discovery fans out ~30 concurrent
// requests; a cold isolate can blow its startup CPU budget and the
// binding fetch throws -- retrying lands warm; GET/HEAD only so no
// mutation is ever replayed). Loader factory failures are absorbed by
// the same retry.
export async function apiserverFetch(env: Env, req: Request): Promise<Response> {
  const stamped = new Request(req);
  if (!stamped.headers.has(CLUSTER_HEADER)) {
    stamped.headers.set(CLUSTER_HEADER, "default");
  }
  const attempts = req.method === "GET" || req.method === "HEAD" ? 3 : 1;
  let lastErr: unknown;
  for (let i = 0; i < attempts; i++) {
    if (i > 0) await new Promise((r) => setTimeout(r, 250 * 2 ** (i - 1)));
    try {
      const ep = await apiserverEntrypoint(env);
      const resp = await ep.fetch((i === attempts - 1 ? stamped : stamped.clone()) as Request);
      if (resp.status < 500 || i === attempts - 1) return resp;
    } catch (err) {
      lastErr = err;
    }
  }
  throw lastErr instanceof Error ? lastErr : new Error(String(lastErr));
}
