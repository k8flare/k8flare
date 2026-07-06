// S19 spike worker. Three gates in one config:
//   G1  containers + assets(run_worker_first) + worker_loaders + sqlite DOs
//       + self service bindings coexist in one wrangler.jsonc (/g1)
//   G2  a self-service-binding Fetcher (named entrypoint) survives the
//       Loader env clone and a loaded worker can reach a DO through it (/g2)
//   G3  the real 43MB Go apiserver WASM served from ASSETS chunks, loaded
//       via LOADER, dispatched on the hot path: cold/warm latency (/g3),
//       kubectl-discovery-like 30-parallel burst (/g3-burst), and whether a
//       DO-origin LOADER.get with the same id shares the isolate (/g3-from-do)
import { DurableObject, WorkerEntrypoint } from "cloudflare:workers";

type Env = {
  ASSETS: Fetcher;
  LOADER: any;
  SELF: Fetcher;
  STORAGE: Fetcher;
  TESTDO: DurableObjectNamespace;
  TINYVM: DurableObjectNamespace;
};

export class TestDO extends DurableObject<Env> {
  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/put") {
      await this.ctx.storage.put("k", url.searchParams.get("v") ?? "");
      return new Response("ok");
    }
    if (url.pathname === "/get") {
      return new Response(((await this.ctx.storage.get("k")) as string) ?? "(unset)");
    }
    // G3b: same LOADER.get id issued from a DO isolate -- if the dynamic
    // worker isolate is shared with the one the public handler created,
    // this dispatch is warm (ms); if per-caller, the factory reruns (s).
    if (url.pathname === "/g3-from-do") {
      const r = await dispatchApiserver(this.env, "/version");
      return Response.json(r);
    }
    return new Response("not found", { status: 404 });
  }
}

// G1 presence-only: config maps this class to a container image; the spike
// never starts an instance.
export class TinyVM extends DurableObject<Env> {
  async fetch(): Promise<Response> {
    return new Response("tinyvm");
  }
}

// The consolidation's ClusterLoopback shape: header-addressed DO routing
// behind a named-entrypoint self service binding, passable into Loader envs.
export class ClusterLoopback extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const name = request.headers.get("X-S19-Cluster") ?? "default";
    const ns = this.env.TESTDO;
    return ns.get(ns.idFromName(name)).fetch(request);
  }
}

const G2_CHILD_JS = `
export default {
  async fetch(request, env) {
    const put = await env.STORAGE.fetch("http://do.internal/put?v=hello-from-loader", {
      headers: { "X-S19-Cluster": "g2" },
    });
    const got = await env.STORAGE.fetch("http://do.internal/get", {
      headers: { "X-S19-Cluster": "g2" },
    });
    return Response.json({ putStatus: put.status, value: await got.text() });
  },
};
`;

// NOT the KCM instantiate-once shape: syumai/workers' generated
// worker.mjs (workers/apiserver/build/worker.mjs) creates a FRESH
// Go()+WebAssembly.Instance per request, caching only the compiled
// Module -- the apiserver Go program serves one request and exits
// ("Go program has already exited" on dispatch 2 when we tried the
// instantiate-once shape live). The Loader's `app.wasm` import IS the
// compiled Module, so warm dispatch cost = per-request instantiation,
// identical to today's deployed apiserver.
const APISERVER_BOOTSTRAP_JS = `
import "./wasm_exec.js";
import wasmModule from "./app.wasm";

globalThis.tryCatch = (fn) => {
  try {
    return { result: fn() };
  } catch (e) {
    return { error: e };
  }
};

async function instantiate(env, ctx) {
  const go = new Go();
  const binding = {};
  let resolveReady;
  const readyPromise = new Promise((resolve) => {
    resolveReady = resolve;
  });
  const runtimeCtx = { env, ctx, connect: undefined, binding };
  const instance = new WebAssembly.Instance(wasmModule, {
    ...go.importObject,
    workers: { ready: () => resolveReady() },
  });
  go.run(instance, runtimeCtx);
  await readyPromise;
  return binding;
}

export default {
  async fetch(request, env, ctx) {
    const binding = await instantiate(env, ctx);
    return binding.handleRequest(request);
  },
};
`;

interface Manifest {
  sha256: string;
  size: number;
  parts: string[];
}

async function fetchAsset(env: Env, path: string): Promise<Response> {
  const resp = await env.ASSETS.fetch(`https://assets.internal/${path}`);
  if (!resp.ok) throw new Error(`asset ${path}: HTTP ${resp.status} (run ./gen-assets.sh first)`);
  return resp;
}

// FOUND LIVE (first version of this spike): caching the getEntrypoint()
// Fetcher at module scope fails on the next request with "Cannot perform
// I/O on behalf of a different request ... (I/O type: SubrequestChannel)"
// -- loader entrypoint stubs are request-scoped I/O objects, exactly like
// DO stubs (workers/storage/src/facets.ts:243-253). The correct stateless-
// handler shape is LOADER.get() PER REQUEST: for an already-loaded id the
// factory is skipped (S2 item 4) so this is cheap; only plain data (the
// manifest) may be cached at module scope. (A Durable Object MAY cache the
// entrypoint across its own requests -- one IoContext per DO lifetime --
// which is why workers/controllers' Controllers DO gets away with it.)
let manifestCache: Manifest | null = null;

async function loadApiserver(env: Env, idSuffix = ""): Promise<{ ep: Fetcher; coldMs: number | null }> {
  if (!manifestCache) {
    manifestCache = (await fetchAsset(env, "apiserver.manifest.json").then((r) => r.json())) as Manifest;
  }
  const manifest = manifestCache;
  let factoryRan = false;
  const t0 = Date.now();
  const worker = env.LOADER.get(`apiserver@${manifest.sha256}${idSuffix}`, async () => {
    factoryRan = true;
    const wasm = new Uint8Array(manifest.size);
    let off = 0;
    for (const part of manifest.parts) {
      const resp = await fetchAsset(env, part);
      const reader = resp.body!.getReader();
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        wasm.set(value, off);
        off += value.byteLength;
      }
    }
    if (off !== manifest.size) throw new Error(`reassembly: ${off} != ${manifest.size}`);
    const wasmExec = await fetchAsset(env, "wasm_exec.js").then((r) => r.text());
    return {
      compatibilityDate: "2026-07-01",
      mainModule: "index.js",
      modules: {
        "index.js": APISERVER_BOOTSTRAP_JS,
        "wasm_exec.js": wasmExec,
        "app.wasm": { wasm: wasm.buffer },
      },
      env: { STORAGE: env.STORAGE, K3S_TOKEN: "k8flare-dev-token" },
    };
  });
  const ep = worker.getEntrypoint();
  await ep.fetch("http://apiserver.internal/version"); // force factory + first dispatch
  return { ep, coldMs: factoryRan ? Date.now() - t0 : null };
}

async function dispatchApiserver(env: Env, path: string) {
  const { ep, coldMs } = await loadApiserver(env);
  const t0 = Date.now();
  const resp = await ep.fetch(`http://apiserver.internal${path}`);
  const body = await resp.text();
  return { coldMs, dispatchMs: Date.now() - t0, status: resp.status, body: body.slice(0, 200) };
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    try {
      if (url.pathname === "/g1") {
        return Response.json({
          bindings: {
            ASSETS: !!env.ASSETS,
            LOADER: !!env.LOADER,
            SELF: !!env.SELF,
            STORAGE: !!env.STORAGE,
            TESTDO: !!env.TESTDO,
            TINYVM: !!env.TINYVM,
          },
          do: await env.TESTDO.get(env.TESTDO.idFromName("g1"))
            .fetch("http://do.internal/put?v=g1")
            .then((r) => r.status),
        });
      }
      if (url.pathname === "/g2") {
        const worker = env.LOADER.get("g2-child", () => ({
          compatibilityDate: "2026-07-01",
          mainModule: "index.js",
          modules: { "index.js": G2_CHILD_JS },
          env: { STORAGE: env.STORAGE },
        }));
        const resp = await worker.getEntrypoint().fetch("http://child.internal/");
        return Response.json({ childStatus: resp.status, child: await resp.json() });
      }
      if (url.pathname === "/g3") {
        return Response.json(await dispatchApiserver(env, url.searchParams.get("path") ?? "/version"));
      }
      if (url.pathname === "/g3-burst") {
        const n = Number(url.searchParams.get("n") ?? "30");
        await dispatchApiserver(env, "/version"); // ensure warm
        const t0 = Date.now();
        const results = await Promise.all(
          Array.from({ length: n }, () => dispatchApiserver(env, "/version")),
        );
        const times = results.map((r) => r.dispatchMs).sort((a, b) => a - b);
        return Response.json({
          n,
          totalMs: Date.now() - t0,
          okCount: results.filter((r) => r.status === 200).length,
          p50: times[Math.floor(n / 2)],
          max: times[n - 1],
        });
      }
      if (url.pathname === "/g3-cold") {
        // Force a brand-new loader id -> fresh isolate, to measure cold
        // repeatedly without restarting dev.
        const r = await loadApiserver(env, `#cold-${url.searchParams.get("seq") ?? "0"}`);
        const t0 = Date.now();
        const resp = await r.ep.fetch("http://apiserver.internal/version");
        return Response.json({ coldMs: r.coldMs, dispatchMs: Date.now() - t0, status: resp.status });
      }
      if (url.pathname === "/g3-from-do") {
        return env.TESTDO.get(env.TESTDO.idFromName("g3")).fetch(
          "http://do.internal/g3-from-do",
        );
      }
      return new Response("s19: /g1 /g2 /g3 /g3-burst /g3-cold /g3-from-do", { status: 404 });
    } catch (err) {
      return new Response(`error: ${err}`, { status: 500 });
    }
  },
};
