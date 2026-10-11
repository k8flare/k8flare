import { WorkerEntrypoint } from "cloudflare:workers";
import { apiserverFetch } from "./loader.ts";

export class Metrics extends WorkerEntrypoint<Env> {
  fetch(request: Request): Promise<Response> {
    return apiserverFetch(this.env, request);
  }
}
