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

// The k3s agent keeps a remotedialer tunnel to its server so the server
// can reach the kubelet. Nothing dials back through it here yet, so the
// endpoint only accepts the socket and leaves it open; a missing endpoint
// would have the agent reconnect every few seconds.
function acceptTunnel(request: Request): Response {
  if (request.headers.get("Upgrade") !== "websocket") {
    return new Response("websocket upgrade required", { status: 426 });
  }
  const pair = new WebSocketPair();
  pair[1].accept();
  return new Response(null, { status: 101, webSocket: pair[0] });
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    if (new URL(request.url).pathname === "/v1-k3s/connect") return acceptTunnel(request);
    return apiserverFetch(env, request);
  },
} satisfies ExportedHandler<Env>;
