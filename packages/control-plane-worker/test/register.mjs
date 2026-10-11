import { registerHooks } from "node:module";

const durableObject = "data:text/javascript,export class DurableObject { constructor(ctx, env) { this.ctx = ctx; this.env = env; } } export class WorkerEntrypoint { constructor(ctx, env) { this.ctx = ctx; this.env = env; } }";
const loader = "data:text/javascript,export async function apiserverFetch(env, request) { return env.__apiserver(request); }";
const loaderKit = "data:text/javascript,export async function loadWasmWorker(loader, assets, name, env) { return loader.load(name, env); } export function isolateId() { return 'test'; }";
const emptyWasm = "data:text/javascript,export default new WebAssembly.Module(new Uint8Array([0, 97, 115, 109, 1, 0, 0, 0]));";

const otel = `data:text/javascript,${encodeURIComponent(`
const span = { measure: (fn) => fn(), child: () => span, context: { traceId: "0", spanId: "0" } };
export class Trace {
  static start() { return new Trace(); }
  root() { return span; }
  async flush() {}
}`)}`;

const goStub = `data:text/javascript,${encodeURIComponent(`
globalThis.Go = class {
  importObject = {};
  run(instance, context) {
    globalThis.__goEnv = context.env;
    context.binding.handleRequest = async () => ({ status: 200, headers: [], body: null });
    context.ready();
    return new Promise(() => {});
  }
};`)}`;

function stubsLoader(parent) {
  return parent?.includes("/src/podkubelet/") || parent?.includes("/src/nodes/") || parent?.includes("/node-tunnel/src/") || parent?.endsWith("/src/queues.ts") || parent?.endsWith("/src/metrics.ts");
}

registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier === "cloudflare:workers") return { url: durableObject, shortCircuit: true };
    if (specifier.endsWith("/loader.ts") && stubsLoader(context.parentURL)) return { url: loader, shortCircuit: true };
    if (specifier === "@k8flare/loader-kit") return { url: loaderKit, shortCircuit: true };
    if (specifier === "./otel") return { url: otel, shortCircuit: true };
    if (specifier.endsWith("/wasm_exec.js")) return { url: goStub, shortCircuit: true };
    if (specifier.endsWith(".wasm")) return { url: emptyWasm, shortCircuit: true };
    return nextResolve(specifier, context);
  },
});
