import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export interface SyncResult {
  objects: Record<string, number>;
  drained: boolean;
  nextMs: number;
}

export class AttachDetach extends WorkerEntrypoint<Env> {
  async sync(): Promise<SyncResult | null> {
    try {
      const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "attachdetach", {
        APISERVER: this.env.APISERVER,
        ADMIN_TOKEN: this.env.ADMIN_TOKEN,
      }, this.env.APISERVER);
      const resp = await worker.fetch("https://attachdetach.internal/sync", { method: "POST", signal: AbortSignal.timeout(60_000) });
      const text = await resp.text();
      if (resp.status !== 200) {
        console.log(`attachdetach: status=${resp.status} ${text.slice(0, 200)}`);
        return null;
      }
      return JSON.parse(text) as SyncResult;
    } catch (err) {
      console.log(`attachdetach: load/fetch ${err}`);
      return null;
    }
  }
}
