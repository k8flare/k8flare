import { apiserverFetch } from "./loader.ts";

export { Cluster } from "@k8flare/cluster-store";
export { Printers } from "./printers.ts";
export { OpenAPI } from "./openapi.ts";
export { CustomResources } from "./customresources.ts";
export { APIGroups } from "./apigroups.ts";
export { Scheduler } from "./scheduler.ts";

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
