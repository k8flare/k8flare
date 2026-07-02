// S8 spike (team-lead follow-up, "if time allows"): minimal feasibility
// check for instantiating and dispatching into a Go WASM program from
// *inside* a Durable Object's own fetch()/alarm() methods, instead of a
// top-level Worker fetch() handler. This is the prerequisite for a
// "DO-hosted event-driven execution" shape (wake via alarm/WS hibernation,
// reconcile, go back to sleep) as an alternative to keeping a stream open.
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
  // Instance fields (not module-scope): persist for as long as this DO
  // instance stays resident, the same way module-scope `let` vars did in
  // the plain-Worker resident spike -- but now scoped to one DO id instead
  // of one isolate.
  instancePromise: Promise<any> | null = null;
  requestCount = 0;

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
    // Same shape as runtime.mjs's createRuntimeContext, but ctx here is
    // this DO's own DurableObjectState (has its own .waitUntil), not a
    // Worker ExecutionContext.
    const runtimeCtx = { env: this.env, ctx: this.state, connect: undefined, binding };
    const instance = new WebAssembly.Instance(wasmModule as WebAssembly.Module, {
      ...go.importObject,
      workers: {
        ready: () => {
          resolveReady();
        },
      },
    });
    go.run(instance, runtimeCtx); // not awaited: main() parks forever (select{})
    await readyPromise;
    return binding;
  }

  async fetch(request: Request): Promise<Response> {
    this.requestCount++;
    const url = new URL(request.url);
    if (url.pathname === "/do-meta") {
      // Answered directly by the DO class itself (no WASM involved), to
      // prove the DO instance itself persists across requests independent
      // of whether WASM instantiation succeeds.
      return Response.json({
        doRequestCount: this.requestCount,
        wasmInstantiated: this.instancePromise !== null,
      });
    }
    if (url.pathname === "/arm-alarm") {
      await this.state.storage.setAlarm(Date.now() + 2000);
      return Response.json({ armed: true });
    }
    if (url.pathname === "/alarm-result") {
      return Response.json({
        alarmFires: (await this.state.storage.get<number>("alarmFires")) ?? 0,
        lastAlarmWasmBody: await this.state.storage.get<string>("lastAlarmWasmBody"),
        lastAlarmError: await this.state.storage.get<string>("lastAlarmError"),
      });
    }
    const binding = await this.ensureBinding();
    return binding.handleRequest(request);
  }

  async alarm(): Promise<void> {
    // Feasibility check only: does calling into the Go program from the
    // DO's *alarm* handler (not fetch) work at all? Records into DO
    // storage (survives DO eviction) rather than an in-memory Go var, so
    // a later fetch() can observe it without needing the same live
    // instance.
    const binding = await this.ensureBinding();
    let alarmFires = ((await this.state.storage.get<number>("alarmFires")) ?? 0) + 1;
    await this.state.storage.put("alarmFires", alarmFires);
    // Touch the Go program from the alarm handler too, to see whether
    // dispatching into it from a non-fetch entry point works at all.
    try {
      const req = new Request("http://do/status");
      const resp = await binding.handleRequest(req);
      const body = await resp.text();
      await this.state.storage.put("lastAlarmWasmBody", body);
    } catch (e: any) {
      await this.state.storage.put("lastAlarmError", String(e));
    }
  }
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    const id = env.WASM_DO.idFromName(url.searchParams.get("id") ?? "default");
    const stub = env.WASM_DO.get(id);
    return stub.fetch(request);
  },
};
