import { WorkerEntrypoint } from "cloudflare:workers";
import { apiserverFetch } from "./loader.ts";
import { componentToken, type Component } from "./componenttoken.ts";

async function asComponent(env: Env, component: Component, request: Request): Promise<Response> {
  const headers = new Headers(request.headers);
  headers.set("Authorization", `Bearer ${await componentToken(env, component)}`);
  return apiserverFetch(env, new Request(request, { headers }));
}

export class HPAAPIServer extends WorkerEntrypoint<Env> {
  fetch(request: Request): Promise<Response> {
    return asComponent(this.env, "hpa", request);
  }
}

export class AttachDetachAPIServer extends WorkerEntrypoint<Env> {
  fetch(request: Request): Promise<Response> {
    return asComponent(this.env, "attachdetach", request);
  }
}
