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

// makePerRequestBootstrapJS: fresh Go()+Instance per request. Every
// concurrent request in this isolate instantiates its OWN
// WebAssembly.Instance (its own Go linear memory) on top of the shared
// 63MB compiled Module, whose code alone already claims a large share
// of the 128MiB production isolate cap; a burst of concurrent requests
// (KCM/scheduler informer LISTs, kubectl discovery fan-out) stacks
// enough coexisting linear memories to blow it. Observed live against
// the real deployment (wrangler tail, 2026-07-17, S24): 41 "Worker
// exceeded memory limit" exceptions in 45s under real controller load.
//
// A concurrency cap here was tried and REVERTED (S24): bounding
// in-flight instances did eliminate the OOM, but the queueing it added
// pushed the kubelet's deadline-sensitive registration requests past
// their deadline -- the kubelet canceled them, the node never went
// Ready, and the cancellations cascaded into the apiserver's own
// storageDo subrequests. Trading OOM for latency-induced cancellation
// was a net loss. The real fixes raise the concurrency ceiling instead
// of throttling it: shrink apiserver.wasm (63MB, the biggest binary) so
// the compiled-module baseline leaves room for more concurrent
// instances, and/or make the apiserver resident (one instance, one
// linear memory, no per-request stacking). Both are larger efforts,
// tracked in S24.
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
