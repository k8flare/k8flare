import { WorkerEntrypoint } from "cloudflare:workers";

export class Outbound extends WorkerEntrypoint<Env> {
  fetch(request: Request): Promise<Response> {
    return fetch(request);
  }
}
