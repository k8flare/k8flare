import { WorkerEntrypoint } from "cloudflare:workers";
import { componentToken } from "./componenttoken.ts";
import { loadWasmWorker } from "@k8flare/loader-kit";

export class Hooks extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "hookecho", {
      APISERVER: this.env.APISERVER,
      API_TOKEN: await componentToken(this.env, "hookecho"),
    }, this.env.APISERVER);
    return worker.fetch(request);
  }
}
