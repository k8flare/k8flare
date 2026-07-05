export { VirtualNode } from "./virtualnode.ts";
export { PodContainerLarge, PodContainerMedium, PodContainerSmall } from "./podcontainer.ts";

import type { Env } from "./env.ts";

// This Worker is not meant to be routed to by kubectl/gateway (workers/nodes
// runs no public API of its own -- see README.md); the only inbound traffic
// it expects is an operator's one-time bootstrap ping (or a health check) to
// wake VirtualNode's alarm loop for the first time (see virtualnode.ts's
// fetch() doc comment), and Cloudflare's own routing of a request to
// *some* default export (Durable Objects cannot be exported from a
// service-worker-format Worker, same reasoning as workers/storage/src/
// index.ts's default export). NODE_POOL selects which pool this Worker
// instance's single default export routes to -- deploying more than one
// pool means deploying more than one copy of this Worker with a different
// NODE_POOL, not one Worker fanning out to many (v1 simplification, see
// README.md).
export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    // Token-gate the bootstrap ping: this Worker keeps a public
    // workers.dev URL only so an operator can wake VirtualNode's alarm
    // loop after deploy, and that wake starts the permanent ~10s
    // Lease-heartbeat cycle -- a real, recurring cost lever that must not
    // be pullable by unauthenticated strangers (cost invariant #5).
    const token = env.K3S_TOKEN || "k8flare-dev-token";
    const auth = request.headers.get("Authorization") || "";
    if (auth !== `Bearer ${token}`) {
      return new Response("unauthorized", { status: 401 });
    }
    const stub = env.VIRTUAL_NODE.get(env.VIRTUAL_NODE.idFromName("default"));
    return stub.fetch(request);
  },
};
