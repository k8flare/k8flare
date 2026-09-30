import { registerHooks } from "node:module";

const durableObject = "data:text/javascript,export class DurableObject { constructor(ctx, env) { this.ctx = ctx; this.env = env; } }";
const loader = "data:text/javascript,export async function apiserverFetch(env, request) { return env.__apiserver(request); }";

registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier === "cloudflare:workers") return { url: durableObject, shortCircuit: true };
    if (specifier.endsWith("/loader.ts") && context.parentURL?.includes("/src/podkubelet/")) return { url: loader, shortCircuit: true };
    return nextResolve(specifier, context);
  },
});
