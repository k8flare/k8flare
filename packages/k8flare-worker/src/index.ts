// k8flare -- the ONE deployed Worker (6→1 consolidation, S19).
// Public routing lives in gateway/handleGateway; every Durable Object
// class and the ClusterLoopback entrypoint are re-exported here so the
// single wrangler.jsonc can bind them all locally.
import type { Env } from "./env.ts";
import { handleGateway } from "./gateway/index.ts";
import { pumpTrace, pumpTraceEnabled } from "./trace.ts";

export { Cluster, WatchHub } from "./storage/index.ts";
export { Controllers } from "./controllers/index.ts";
export { ClusterRegistry } from "./clusters/registry.ts";
export { CFContainersScheduler } from "./nodes/scheduler.ts";
export { NodeVMLarge, NodeVMMedium, NodeVMSmall } from "./nodes/nodevm.ts";
export { ClusterLoopback } from "./entrypoints.ts";

export default {
  async fetch(req: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    // Guarded rather than left to pumpTrace: building the fields parses a
    // URL, and a boundary nobody asked to measure must cost nothing.
    if (pumpTraceEnabled(env)) {
      pumpTrace(env, "request", "gateway", {
        o: new URL(req.url).pathname,
        ua: req.headers.get("User-Agent") ?? "",
        m: req.method,
      });
    }
    return handleGateway(req, env, ctx);
  },
};
