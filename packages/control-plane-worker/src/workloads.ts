import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export interface SyncResult {
  pods: number;
  replicaSets: number;
  deployments: number;
  drained: boolean;
}

export class Workloads extends WorkerEntrypoint<Env> {
  async sync(): Promise<SyncResult | null> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "workloads", {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, this.env.APISERVER);
    const resp = await worker.fetch("https://workloads.internal/sync", { method: "POST" });
    const text = await resp.text();
    if (resp.status !== 200) {
      console.log(`workloads: sync status=${resp.status} ${text.slice(0, 200)}`);
      return null;
    }
    return JSON.parse(text) as SyncResult;
  }
}
