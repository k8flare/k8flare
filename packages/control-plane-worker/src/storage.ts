import { WorkerEntrypoint } from "cloudflare:workers";

export class Storage extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    return this.env.CLUSTER.get(this.env.CLUSTER.idFromName("default")).fetch(request);
  }
}
