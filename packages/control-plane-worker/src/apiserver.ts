import { WorkerEntrypoint } from "cloudflare:workers";
import { apiserverFetch } from "./loader.ts";

export class APIServer extends WorkerEntrypoint<Env> {
  fetch(request: Request): Promise<Response> {
    const headers = new Headers(request.headers);
    headers.set("Authorization", `Bearer ${this.env.ADMIN_TOKEN}`);
    return apiserverFetch(this.env, new Request(request, { headers }));
  }
}
