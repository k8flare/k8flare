import { WorkerEntrypoint } from "cloudflare:workers";
import { componentToken } from "./componenttoken.ts";
import { loadWasmWorker } from "@k8flare/loader-kit";

const hookPath = /^\/hook\/([^/]+)(.*)$/;

export class Hooks extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    const [, name, path] = url.pathname.match(hookPath) ?? [];
    const userWorker = name ? (this.env as unknown as Record<string, Fetcher | undefined>)[`WORKER_${name}`] : undefined;
    if (userWorker) {
      url.pathname = path || "/";
      return userWorker.fetch(new Request(url, request));
    }
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "hookecho", {
      APISERVER: this.env.APISERVER,
      API_TOKEN: await componentToken(this.env, "hookecho"),
    }, this.env.APISERVER);
    return worker.fetch(request);
  }
}
