import { registerHooks } from "node:module";

const stub = "data:text/javascript,export class DurableObject { constructor(ctx, env) { this.ctx = ctx; this.env = env; } }";

registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier === "cloudflare:workers") return { url: stub, shortCircuit: true };
    return nextResolve(specifier, context);
  },
});
