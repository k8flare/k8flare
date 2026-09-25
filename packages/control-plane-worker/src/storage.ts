import { WorkerEntrypoint } from "cloudflare:workers";
import { clusterStub } from "./clusterid.ts";

export class Storage extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    return clusterStub(this.env).fetch(request);
  }
}
