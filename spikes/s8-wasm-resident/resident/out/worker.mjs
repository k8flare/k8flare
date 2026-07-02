// Hand-written for the S8 spike (S5: isolate-level WASM instance reuse).
// Unlike the generated worker.mjs (see stock/out/worker.mjs, or
// packages/worker/build/worker.mjs in the main repo), this glue creates the
// WebAssembly.Instance and starts the Go program (`go.run`) at most ONCE per
// isolate. Every subsequent fetch() reuses the same `binding` object that
// Go's package-init wired `handleRequest` onto, dispatching into the
// already-running Go program/goroutines instead of instantiating fresh.
import "./wasm_exec.js";
import { createRuntimeContext, loadModule } from "./runtime.mjs";

globalThis.tryCatch = (fn) => {
  try {
    return { result: fn() };
  } catch (e) {
    return { error: e };
  }
};

let mod;
let instancePromise = null;

// instantiate() is only ever invoked for the request that wins the race to
// create instancePromise (see ensureBinding). env/ctx here are that winning
// request's bindings; every later request's own env/ctx are discarded. That
// is exactly the behavior FINDINGS.md documents and tests via /waituntil.
async function instantiate(env, ctx) {
  if (mod === undefined) {
    mod = await loadModule();
  }
  const go = new Go();
  const binding = {};
  let resolveReady;
  const readyPromise = new Promise((resolve) => {
    resolveReady = resolve;
  });
  const runtimeCtx = createRuntimeContext({ env, ctx, binding });
  const instance = new WebAssembly.Instance(mod, {
    ...go.importObject,
    workers: {
      ready: () => {
        resolveReady();
      },
    },
  });
  // Deliberately not awaited: main() in the resident Go program never
  // returns (ServeNonBlock + Ready + `select{}`), so this promise never
  // resolves. Awaiting it here would hang every request forever.
  go.run(instance, runtimeCtx);
  await readyPromise;
  return binding;
}

function ensureBinding(env, ctx) {
  if (!instancePromise) {
    instancePromise = instantiate(env, ctx);
  }
  return instancePromise;
}

async function fetch(req, env, ctx) {
  const binding = await ensureBinding(env, ctx);
  return binding.handleRequest(req);
}

export default { fetch };
