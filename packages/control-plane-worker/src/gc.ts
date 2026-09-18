import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export interface CollectResult {
  items: number;
  deleted: number;
  patched: number;
  pending: number;
}

export class GarbageCollector extends WorkerEntrypoint<Env> {
  async collect(): Promise<CollectResult | null> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "gc", {
      APISERVER: this.env.APISERVER,
      STORAGE: this.env.CLUSTER.get(this.env.CLUSTER.idFromName("default")),
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, this.env.APISERVER);
    const resp = await worker.fetch("https://gc.internal/collect", { method: "POST" });
    const text = await resp.text();
    if (resp.status !== 200) {
      console.log(`gc: status=${resp.status} ${text.slice(0, 200)}`);
      return null;
    }
    return JSON.parse(text) as CollectResult;
  }
}
