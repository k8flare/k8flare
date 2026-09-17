import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export class CustomResources extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "customresources", {
      STORAGE: this.env.CLUSTER.get(this.env.CLUSTER.idFromName("default")),
    }, this.env.APISERVER);
    return worker.fetch(request);
  }
}
