// Bootstrap module sources for Loader dynamic workers hosting syumai/
// workers Go WASM binaries. Two shapes, matching how the binary itself
// behaves (S19 finding -- they are NOT interchangeable):
//
//  - resident (KCM): instantiate once per isolate; the Go program parks
//    forever (select{}) and needs a bounded pump window per poke or its
//    goroutines freeze when the dispatch response completes (S14).
//    Trying the per-request shape would restart every informer per poke.
//  - per-request (apiserver): syumai's generated worker.mjs contract --
//    fresh Go()+Instance per request over the cached compiled Module;
//    the Go program serves one request and exits. Trying the resident
//    shape dies on dispatch 2 with "Go program has already exited"
//    (observed live, spikes/s19-single-worker).

const COMMON = `
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
`;

export function makeResidentBootstrapJS(pumpWindowMs: number): string {
  return `${COMMON}
const PUMP_WINDOW_MS = ${pumpWindowMs};
let bindingPromise = null;

export default {
  async fetch(request, env, ctx) {
    if (!bindingPromise) bindingPromise = instantiate(env, ctx);
    const binding = await bindingPromise;
    ctx.waitUntil(new Promise((resolve) => setTimeout(resolve, PUMP_WINDOW_MS)));
    return binding.handleRequest(request);
  },
};
`;
}

export function makePerRequestBootstrapJS(): string {
  return `${COMMON}
export default {
  async fetch(request, env, ctx) {
    const binding = await instantiate(env, ctx);
    return binding.handleRequest(request);
  },
};
`;
}
