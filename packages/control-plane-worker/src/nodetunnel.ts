import { WorkerEntrypoint } from "cloudflare:workers";
export class NodeTunnels extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const match = new URL(request.url).pathname.match(/^\/node\/([^/]+)\//);
    if (!match) return new Response("not found", { status: 404 });
    const stub = this.env.NODE_TUNNEL.get(this.env.NODE_TUNNEL.idFromName(match[1]));
    return stub.fetch(request);
  }
}
