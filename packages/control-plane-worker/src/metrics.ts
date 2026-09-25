import { WorkerEntrypoint } from "cloudflare:workers";
import { apiserverFetch } from "./loader.ts";

export class Metrics extends WorkerEntrypoint<Env> {
  fetch(request: Request): Promise<Response> {
    const headers = new Headers(request.headers);
    if (!headers.get("Authorization")) headers.set("Authorization", `Bearer ${this.env.ADMIN_TOKEN}`);
    return apiserverFetch(this.env, new Request(request, { headers }));
  }
}
