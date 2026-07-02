// S8 production re-verification worker. Instantiates the Go WASM program
// once inside a Durable Object instance (not module scope) and reuses it
// for every request routed to that DO id -- same pattern confirmed working
// locally in spikes/s8-wasm-resident/do-hosted/. See go/main.go for what's
// being tested (ctx.WaitUntil duration limit, wasm_exec.js fetch-bind patch).
import "../wasm/wasm_exec.js";
// @ts-expect-error -- .wasm import, handled by wrangler's bundler
import wasmModule from "../wasm/app.wasm";

declare const Go: any;

export interface Env {
  WASM_DO: DurableObjectNamespace;
}

export class WasmDO {
  state: DurableObjectState;
  env: Env;
  instancePromise: Promise<any> | null = null;

  constructor(state: DurableObjectState, env: Env) {
    this.state = state;
    this.env = env;
  }

  async ensureBinding(): Promise<any> {
    if (!this.instancePromise) {
      this.instancePromise = this.instantiate();
    }
    return this.instancePromise;
  }

  async instantiate(): Promise<any> {
    const go = new Go();
    const binding: any = {};
    let resolveReady: () => void;
    const readyPromise = new Promise<void>((resolve) => {
      resolveReady = resolve;
    });
    const runtimeCtx = { env: this.env, ctx: this.state, connect: undefined, binding };
    const instance = new WebAssembly.Instance(wasmModule as WebAssembly.Module, {
      ...go.importObject,
      workers: {
        ready: () => {
          resolveReady();
        },
      },
    });
    go.run(instance, runtimeCtx);
    await readyPromise;
    return binding;
  }

  async fetch(request: Request): Promise<Response> {
    const binding = await this.ensureBinding();
    return binding.handleRequest(request);
  }
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    // ?trial=<name> selects a distinct DO instance, so a second waitUntil
    // trial can run fully independently of one already in flight on
    // "default" without redeploying mid-trial (which could disrupt the
    // already-running DO instance).
    const trial = new URL(request.url).searchParams.get("trial") ?? "default";
    const id = env.WASM_DO.idFromName(trial);
    const stub = env.WASM_DO.get(id);
    return stub.fetch(request);
  },
};
