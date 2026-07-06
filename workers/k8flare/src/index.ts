// k8flare -- the ONE deployed Worker (6→1 consolidation, S19).
// Public routing lives in gateway/handleGateway; every Durable Object
// class and the ClusterLoopback entrypoint are re-exported here so the
// single wrangler.jsonc can bind them all locally.
import type { Env } from "./env.ts";
import { handleGateway } from "./gateway/index.ts";

export { Cluster, WatchHub } from "./storage/index.ts";
export { Controllers } from "./controllers/index.ts";
export { CFContainersScheduler } from "./nodes/scheduler.ts";
export { NodeVMLarge, NodeVMMedium, NodeVMSmall } from "./nodes/nodevm.ts";
export { ClusterLoopback } from "./entrypoints.ts";

export default {
  async fetch(req: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    return handleGateway(req, env, ctx);
  },
};
