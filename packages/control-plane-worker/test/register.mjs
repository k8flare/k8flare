import { registerHooks } from "node:module";

const durableObject = "data:text/javascript,export class DurableObject { constructor(ctx, env) { this.ctx = ctx; this.env = env; } }";
const loader = "data:text/javascript,export async function apiserverFetch(env, request) { return env.__apiserver(request); }";
const loaderKit = "data:text/javascript,export async function loadWasmWorker(loader, assets, name, env) { return loader.load(name, env); }";

const otel = `data:text/javascript,${encodeURIComponent(`
const span = { measure: (fn) => fn(), child: () => span, context: { traceId: "0", spanId: "0" } };
export class Trace {
  static start() { return new Trace(); }
  root() { return span; }
  async flush() {}
}`)}`;

registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier === "cloudflare:workers") return { url: durableObject, shortCircuit: true };
    if (specifier.endsWith("/loader.ts") && (context.parentURL?.includes("/src/podkubelet/") || context.parentURL?.endsWith("/src/queues.ts"))) return { url: loader, shortCircuit: true };
    if (specifier === "@k8flare/loader-kit") return { url: loaderKit, shortCircuit: true };
    if (specifier === "./otel") return { url: otel, shortCircuit: true };
    return nextResolve(specifier, context);
  },
});
