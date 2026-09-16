import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

const holdWindowMs = 15_000;
let holding = false;

export class CustomResources extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "customresources", {
      STORAGE: this.env.CLUSTER.get(this.env.CLUSTER.idFromName("default")),
      CONTROLLERS: this.env.CONTROLLERS,
    }, this.env.APISERVER);
    if (!holding) {
      holding = true;
      this.ctx.waitUntil(
        worker
          .fetch(`https://customresources.internal/hold?window=${holdWindowMs}`)
          .then((resp) => resp.text())
          .catch(() => {})
          .finally(() => {
            holding = false;
          }),
      );
    }
    return worker.fetch(request);
  }
}
