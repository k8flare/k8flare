// DO-hosted Go WASM pattern verified in spikes/s8-wasm-resident/do-hosted
// (local) and production-verified in spikes/s8-wasm-resident/prod-resident
// (docs/platform-verification.md's S8 section): instantiate the Go
// program once per DO instance, dispatch every fetch()/alarm() into the
// same running instance via binding.handleRequest. main.go
// (workers/controllers/main.go) starts the resident kube-controller-manager
// goroutine (pkg/controllers.RunControllerManager) on first dispatch, kept
// alive past the triggering request via cloudflare.WaitUntil.
//
// IMPORTANT (see main.go's and pkg/controllers/controllermanager.go's doc
// comments for the full writeup): the compiled workers/controllers/build/app.wasm
// this file imports is, as of this writing, too large to actually deploy
// (gzip size well over Cloudflare Workers' 10MiB limit, driven by
// k8s.io/client-go's generated typed clientset/informers alone -- see
// docs/platform-verification.md's S8 section for exact measurements).
// This file and main.go are kept as verified-buildable, DO-hosted glue
// for whichever narrower binary shape resolves that -- do not `wrangler
// deploy` this Worker as-is.
import "../build/wasm_exec.js";
// @ts-expect-error -- .wasm import, handled by wrangler's bundler
import wasmModule from "../build/app.wasm";
import type { Env } from "./env.ts";

declare const Go: any;

// Safety-net alarm interval: mirrors workers/storage's Cluster DO
// SAFETY_NET_INTERVAL_MS (workers/storage/src/index.ts) -- this is purely
// a liveness/resurrection check (is the resident controller-manager
// goroutine still alive after a redeploy/panic/eviction reset this DO
// instance?), not a reconciliation trigger: the real controllers, once
// running, watch continuously via their own client-go informers and need
// no periodic nudging from here.
const SAFETY_NET_INTERVAL_MS = 60_000;

export class Controllers {
  state: DurableObjectState;
  env: Env;
  instancePromise: Promise<any> | null = null;

  constructor(state: DurableObjectState, env: Env) {
    this.state = state;
    this.env = env;
  }

  private async ensureBinding(): Promise<any> {
    if (!this.instancePromise) {
      this.instancePromise = this.instantiate();
    }
    return this.instancePromise;
  }

  private async instantiate(): Promise<any> {
    const go = new Go();
    const binding: any = {};
    let resolveReady: () => void;
    const readyPromise = new Promise<void>((resolve) => {
      resolveReady = resolve;
    });
    // Same shape as the do-hosted/prod-resident spikes: ctx here is this
    // DO's own DurableObjectState (has its own .waitUntil, used by
    // cloudflare.WaitUntil on the Go side), not a Worker ExecutionContext.
    const runtimeCtx = { env: this.env, ctx: this.state, connect: undefined, binding };
    const instance = new WebAssembly.Instance(wasmModule as WebAssembly.Module, {
      ...go.importObject,
      workers: {
        ready: () => {
          resolveReady();
        },
      },
    });
    go.run(instance, runtimeCtx); // not awaited: main() starts the resident controller-manager then parks forever (select{})
    await readyPromise;
    return binding;
  }

  async fetch(request: Request): Promise<Response> {
    const binding = await this.ensureBinding();
    const response = await binding.handleRequest(request);
    // Arm the safety net if it isn't already, so a redeploy/panic/
    // eviction that resets this DO instance still gets noticed and
    // restarted even if no further relevant write happens to re-trigger
    // storage's pingControllers (workers/storage/src/index.ts). Cheap
    // local check -- no cross-DO call on this hot path.
    const current = await this.state.storage.getAlarm();
    if (current === null) {
      this.state.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }
    return response;
  }

  async alarm(): Promise<void> {
    // Dispatching into the Go program from alarm() (not just fetch())
    // works via the same binding.handleRequest call -- verified in
    // spikes/s8-wasm-resident/do-hosted. Hitting /healthz both confirms
    // liveness and (re-)triggers the Go side's ensureStarted() if this DO
    // instance was reset since the last request.
    const binding = await this.ensureBinding();
    await binding.handleRequest(new Request("http://controllers.internal/healthz"));

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
