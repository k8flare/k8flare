// DO-hosted Go WASM pattern verified in spikes/s8-wasm-resident/do-hosted
// (local) and production-verified in spikes/s8-wasm-resident/prod-resident
// (docs/platform-verification.md's S8 section), now combined with the
// ASSETS+LOADER code-supply path verified in
// spikes/s14-loader-external-fetch: the compiled kube-controller-manager
// WASM is far too large to bundle into this script (gzip ~13MB vs
// Workers' 10MiB deploy cap), so it is served from this Worker's own
// Static Assets in <=24MiB chunks (25MiB ASSETS per-file cap), assembled
// at Loader-factory time, and run as a Dynamic Worker via the
// `modules.wasm` field -- the platform's sanctioned route around both the
// deploy cap and the dynamic-code-generation restriction (S14 Part 1).
// The wasm-opt'd binary must stay under the Loader's own hard 64MiB
// total-module-bytes cap (S14 Part 2) -- scripts/build-controllers-wasm.sh
// enforces that at build time.
//
// The Controllers DO below keeps its previous role (single resident
// instance, event-armed alarm safety net); it just dispatches into the
// Loader-loaded dynamic worker instead of instantiating the WASM in its
// own isolate. Two S2/S14-verified constraints shape the wiring:
//   - `Fetcher`s (service bindings) pass through WorkerCode.env; DO
//     namespaces/stubs do NOT (S2 item 3a). pkg/controllers.RestConfig
//     only needs the GATEWAY Fetcher by name, so the dynamic worker gets
//     exactly { GATEWAY, K3S_TOKEN }.
//   - The Loader binding itself cannot be forwarded (S14 Part 6), so all
//     LOADER.get() calls happen here, never inside loaded code.
import type { Env } from "./env.ts";

// Safety-net alarm interval: mirrors workers/storage's Cluster DO
// SAFETY_NET_INTERVAL_MS (workers/storage/src/index.ts) -- this is purely
// a liveness/resurrection check (is the resident controller-manager
// still alive after a redeploy/panic/eviction?), not a reconciliation
// trigger: the real controllers, once running, watch continuously via
// their own client-go informers and need no periodic nudging from here.
const SAFETY_NET_INTERVAL_MS = 60_000;

interface KcmManifest {
  size: number;
  sha256: string;
  parts: string[];
}

// Main module of the dynamic worker. Same instantiate-once-per-isolate
// shape this DO used when it hosted the WASM itself (and the same
// runtimeCtx contract as syumai/workers' generated worker.mjs): the
// Loader keeps the isolate (and therefore the module-scope bindingPromise
// and the running Go program) alive across requests for the same id
// (S2 item 4); if the isolate is evicted, the next dispatch simply
// re-instantiates and the controllers resync via their informers.
// `import "./wasm_exec.js"` is the syumai/workers variant (side-effect:
// defines globalThis.Go with runtimeCtx support), served from ASSETS.
// The `app.wasm` import arrives pre-compiled as a WebAssembly.Module --
// that is how the Loader's `modules.wasm` field works (S2 item 1).
const BOOTSTRAP_JS = `
import "./wasm_exec.js";
import wasmModule from "./app.wasm";

globalThis.tryCatch = (fn) => {
  try {
    return { result: fn() };
  } catch (e) {
    return { error: e };
  }
};

let bindingPromise = null;

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
    workers: {
      ready: () => resolveReady(),
    },
  });
  go.run(instance, runtimeCtx); // not awaited: main() starts the resident controller-manager then parks forever (select{})
  await readyPromise;
  return binding;
}

// How long each poke keeps this isolate's event loop serviced after its
// own response returns. Without this, the resident KCM goroutines freeze
// the moment a dispatch completes (verified: a Deployment scale PATCH
// after the initial startup window was never reconciled until the next
// dispatch) -- unlike the previous DO-hosted shape, a dynamic worker has
// no DurableObjectState.waitUntil keeping it pumped. Event-armed and
// bounded: a window opens only when something pokes us (storage's
// pingControllers on relevant writes, or the parent DO's safety-net
// alarm), and KCM's own resulting writes (e.g. Pod creates) re-ping and
// chain further windows while there is real work, then go quiet. I/O
// waits inside the window are not CPU-billed (cost invariant #2).
const PUMP_WINDOW_MS = 25000;

export default {
  async fetch(request, env, ctx) {
    if (!bindingPromise) bindingPromise = instantiate(env, ctx);
    const binding = await bindingPromise;
    ctx.waitUntil(new Promise((resolve) => setTimeout(resolve, PUMP_WINDOW_MS)));
    return binding.handleRequest(request);
  },
};
`;

export class Controllers {
  state: DurableObjectState;
  env: Env;
  manifestPromise: Promise<KcmManifest> | null = null;
  // Resolved entrypoint once the dynamic worker is loaded, and the
  // in-flight load. Loading ~60MB through the Loader factory takes long
  // enough in production that awaiting it inline from a poke is wrong:
  // storage's pingControllers runs inside the write path, the canceled
  // ping tears down the load with it, and the next write starts the
  // whole load over -- observed live as an endless "GET / - Canceled"
  // storm with KCM never coming up. Pokes therefore return immediately
  // and the load runs detached (DO lifetime is not tied to any one
  // request); the load's own completion handler delivers the first poke.
  kcmEntrypoint: Fetcher | null = null;
  kcmLoading: Promise<Fetcher> | null = null;

  constructor(state: DurableObjectState, env: Env) {
    this.state = state;
    this.env = env;
  }

  private async manifest(): Promise<KcmManifest> {
    if (!this.manifestPromise) {
      this.manifestPromise = this.fetchManifest();
      // Don't cache failures (e.g. assets not built yet).
      this.manifestPromise.catch(() => {
        this.manifestPromise = null;
      });
    }
    return this.manifestPromise;
  }

  private async fetchManifest(): Promise<KcmManifest> {
    const resp = await this.env.ASSETS.fetch("https://assets.internal/kcm.manifest.json");
    if (!resp.ok) {
      throw new Error(
        `kcm.manifest.json: HTTP ${resp.status} -- workers/controllers/assets/ is missing; run npm run build:wasm first`,
      );
    }
    return resp.json<KcmManifest>();
  }

  private async fetchAsset(path: string): Promise<Response> {
    const resp = await this.env.ASSETS.fetch(`https://assets.internal/${path}`);
    if (!resp.ok) throw new Error(`asset ${path}: HTTP ${resp.status}`);
    return resp;
  }

  // Returns a Fetcher into the Loader-loaded kube-controller-manager
  // dynamic worker. Cheap on the warm path: LOADER.get() with an
  // already-loaded id skips the factory entirely (S2 item 4), so the
  // chunk fetching/assembly below only runs on isolate cold start. The
  // binary's own sha256 is the cache id, so a rebuilt binary is a new id
  // and the stale isolate is simply never addressed again.
  //
  // Chunks are fetched sequentially and streamed straight into one
  // preallocated buffer: the earlier fetch-all-then-concat version held
  // both every part and the assembled copy alive at once (~2x binary
  // size ≈ 125MB), which flirts with production's 128MiB isolate memory
  // limit that wrangler dev never enforces.
  private ensureKcm(): Promise<Fetcher> {
    if (!this.kcmLoading) {
      this.kcmLoading = this.loadKcm().then((f) => {
        this.kcmEntrypoint = f;
        return f;
      });
      // Don't cache failures -- the next poke retries the load.
      this.kcmLoading.catch((err) => {
        console.log(`controllers: kcm load failed: ${err}`);
        this.kcmLoading = null;
      });
    }
    return this.kcmLoading;
  }

  private async loadKcm(): Promise<Fetcher> {
    const manifest = await this.manifest();
    const worker = this.env.LOADER.get(`kcm@${manifest.sha256}`, async () => {
      const wasm = new Uint8Array(manifest.size);
      let off = 0;
      for (const part of manifest.parts) {
        const resp = await this.fetchAsset(part);
        if (!resp.body) throw new Error(`asset ${part}: empty body`);
        const reader = resp.body.getReader();
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          wasm.set(value, off);
          off += value.byteLength;
        }
      }
      if (off !== manifest.size) {
        throw new Error(`kcm wasm reassembly: got ${off} bytes, manifest says ${manifest.size}`);
      }
      console.log(`controllers: kcm chunks assembled (${off} bytes)`);
      const wasmExec = await this.fetchAsset("wasm_exec.js").then((r) => r.text());
      // Only plain values and Fetchers survive the env clone (S2 item
      // 3a) -- this is exactly what pkg/controllers.RestConfig("GATEWAY")
      // and getToken() read on the Go side.
      const dynamicEnv: Record<string, unknown> = { GATEWAY: this.env.GATEWAY };
      if (this.env.K3S_TOKEN) dynamicEnv.K3S_TOKEN = this.env.K3S_TOKEN;
      return {
        compatibilityDate: "2026-03-24",
        mainModule: "index.js",
        modules: {
          "index.js": BOOTSTRAP_JS,
          "wasm_exec.js": wasmExec,
          "app.wasm": { wasm: wasm.buffer },
        },
        env: dynamicEnv,
      };
    });
    const entrypoint = worker.getEntrypoint();
    // Force the factory to actually run now (getEntrypoint alone is lazy)
    // and prove the dynamic worker is dispatchable before declaring it
    // ready -- this doubles as the "first poke" that starts the resident
    // controller-manager.
    await entrypoint.fetch("http://controllers.internal/healthz");
    console.log("controllers: kcm dynamic worker up");
    return entrypoint;
  }

  async fetch(request: Request): Promise<Response> {
    // Arm the safety net if it isn't already, so a redeploy/panic/
    // eviction that resets the dynamic worker still gets noticed and
    // restarted even if no further relevant write happens to re-trigger
    // storage's pingControllers (workers/storage/src/index.ts). Cheap
    // local check -- no cross-DO call on this hot path. Armed before the
    // dispatch so a still-loading KCM gets a completion poke even if no
    // further write ever arrives.
    const current = await this.state.storage.getAlarm();
    if (current === null) {
      this.state.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }

    const kcm = this.kcmEntrypoint;
    if (!kcm) {
      // Never await the load from a poke: pokes come from storage's
      // write path and get canceled when that write finishes, and a
      // canceled poke must not tear down (or serially restart) the load.
      // The detached promise outlives this request -- DO lifetime is not
      // request-scoped -- and its completion dispatch starts KCM.
      void this.ensureKcm()
        .then((f) => f.fetch("http://controllers.internal/healthz"))
        .catch(() => {}); // already logged in ensureKcm
      return Response.json({ controllerManager: "loading" }, { status: 202 });
    }
    return kcm.fetch(request);
  }

  async alarm(): Promise<void> {
    // Hitting /healthz both confirms liveness and (re-)triggers the Go
    // side's ensureStarted() if the dynamic worker's isolate was evicted
    // since the last request (the Loader factory then reruns too). The
    // alarm context has no client to cancel it, so awaiting the load
    // here is safe (and is what completes a load whose pokes all died).
    const kcm = await this.ensureKcm();
    await kcm.fetch("http://controllers.internal/healthz");

    // Re-arm only while the cluster still has a live Node worth having
    // controllers running for; otherwise park (cost invariants #1/#3 --
    // no alarm chain on an idle cluster). Queries CLUSTER's raw KV
    // endpoint directly (cheap, no WASM dispatch) rather than the full
    // REST/JSON apiserver path, mirroring Cluster DO's own
    // hasPendingSafetyNetWork check (workers/storage/src/index.ts).
    if (await this.hasLiveNodes()) {
      this.state.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }
  }

  private async hasLiveNodes(): Promise<boolean> {
    const ns = this.env.CLUSTER;
    if (!ns) return false;
    try {
      const stub = ns.get(ns.idFromName("default"));
      const resp = await stub.fetch("http://cluster.internal/list/registry/nodes/?limit=1");
      if (!resp.ok) return false;
      const data = await resp.json<{ count?: number }>();
      return (data.count ?? 0) > 0;
    } catch {
      return false;
    }
  }
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const ns = env.CONTROLLERS;
    const stub = ns.get(ns.idFromName("default"));
    return stub.fetch(request);
  },
};
