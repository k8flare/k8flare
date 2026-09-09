// Bootstrap module source for Loader dynamic workers hosting syumai/
// workers Go WASM binaries. All four binaries (apiserver, kcm, sched,
// gc) now use the RESIDENT shape: instantiate once per isolate, the Go
// program parks forever (select{}), and one instance serves every
// dispatch over the isolate's lifetime. The Go side signals readiness
// via `workers: { ready }` so the bootstrap knows the js.FuncOf exports
// are installed before dispatching. The per-request shape the apiserver
// used to run (fresh Go()+Instance per request) was retired 2026-07-17:
// under concurrent controller load its stacked per-request linear
// memories blew the 128MiB production isolate cap (S24), while the
// resident instance's single GC'd heap does not. S19 had recorded that
// the apiserver "dies on dispatch 2" under the resident shape -- that
// was the resident BOOTSTRAP paired with a Go main() that still exited
// after one dispatch (workers.Serve); the fix was to make main() park
// (workers.ServeNonBlock + Ready + select{}) like the controllers, plus
// per-request panic recovery so one handler panic can't take down the
// shared instance (see pkg/apiserver/cmd/apiserver-wasm/main.go).
//
// The pump window keeps a resident instance warm between dispatches:
// KCM/gc/sched NEED it (their controllers run background goroutines that
// must progress between pokes -- S14/S8, they freeze when the dispatch
// response completes otherwise); the apiserver does NOT need it for
// correctness (no background goroutines -- each request is fully handled
// within its own dispatch's IoContext) and uses a shorter window purely
// to stay warm across the kubelet's heartbeats (loader/apiserver.ts).

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

export function makeResidentBootstrapJS(pumpWindowMs: number, dropCloseEvery = 0): string {
  return `${COMMON}
const PUMP_WINDOW_MS = ${pumpWindowMs};
const DROP_CLOSE_EVERY = ${dropCloseEvery};
let dispatchCount = 0;
let bindingPromise = null;

export default {
  async fetch(request, env, ctx) {
    if (!bindingPromise) bindingPromise = instantiate(env, ctx);
    const binding = await bindingPromise;
    // Publish this request as an I/O anchor for the Go instance's
    // background goroutines, and retire it when the window closes -- they
    // outlive any single dispatch, but the platform only lets them perform
    // I/O on behalf of a request that is still open (S31).
    const pumpWindow = binding.openPumpWindow(env, PUMP_WINDOW_MS);
    const dropClose = DROP_CLOSE_EVERY > 0 && ++dispatchCount % DROP_CLOSE_EVERY === 0;
    ctx.waitUntil(
      new Promise((resolve) =>
        setTimeout(() => {
          if (!dropClose) binding.closePumpWindow(pumpWindow);
          resolve();
        }, PUMP_WINDOW_MS),
      ),
    );
    // Forward THIS request's env so a resident Go instance resolves
    // request-scoped bindings from the current request instead of the one
    // that first instantiated the isolate (handler_js.go dispatch). The
    // env captured in instantiate() above is only used to run the Go
    // program; per-request I/O bindings must come from here.
    return binding.handleRequest(request, env);
  },
};
`;
}
