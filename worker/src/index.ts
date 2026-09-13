import { WorkerEntrypoint } from "cloudflare:workers";
import { apiserverFetch } from "./loader.ts";

export { Cluster } from "./cluster.ts";

// ClusterLoopback is the self service binding (STORAGE) that gives the
// Loader-hosted Go apiserver a route to the Cluster Durable Object, which a
// dynamic worker cannot be handed directly.
export class ClusterLoopback extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const ns = this.env.CLUSTER;
    return ns.get(ns.idFromName("default")).fetch(request);
  }
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    return apiserverFetch(env, request);
  },
} satisfies ExportedHandler<Env>;
