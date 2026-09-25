import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export class Admission extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "admission", {
      STORAGE: this.env.STORAGE_SVC,
      TUNNEL: this.env.TUNNEL,
      HOOKS: this.env.HOOKS,
      OUTBOUND: this.env.OUTBOUND,
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, this.env.APISERVER);
    return worker.fetch(request);
  }
}
