import { WorkerEntrypoint } from "cloudflare:workers";
import { tunnelName } from "./clusterid.ts";
import { vpcFetch } from "./vpc.ts";

export class NodeTunnels extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const viaVpc = await vpcFetch(this.env, request);
    if (viaVpc) return viaVpc;
    const match = new URL(request.url).pathname.match(/^\/(?:node|dial)\/([^/]+)\//);
    if (!match) return new Response("not found", { status: 404 });
    const stub = this.env.NODE_TUNNEL.get(this.env.NODE_TUNNEL.idFromName(tunnelName(this.env, match[1])));
    return stub.fetch(request);
  }
}
