import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export interface SyncResult {
  objects: Record<string, number>;
  drained: boolean;
  nextMs: number;
}

export interface NodeHealthResult {
  evicted: number;
  waiting: number;
  nextMs: number;
}

export interface NamespaceResult {
  terminating: number;
  deleted: number;
  remaining: number;
  nextMs: number;
}

export class Workloads extends WorkerEntrypoint<Env> {
  async sync(): Promise<SyncResult | null> {
    return this.call<SyncResult>("/sync");
  }

  async namespaces(): Promise<NamespaceResult | null> {
    return this.call<NamespaceResult>("/namespaces");
  }

  async nodeHealth(node: string): Promise<NodeHealthResult | null> {
    return this.call<NodeHealthResult>(`/nodehealth?node=${encodeURIComponent(node)}`);
  }

  private async call<T>(path: string): Promise<T | null> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "workloads", {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, this.env.APISERVER);
    const resp = await worker.fetch(`https://workloads.internal${path}`, { method: "POST" });
    const text = await resp.text();
    if (resp.status !== 200) {
      console.log(`workloads: ${path} status=${resp.status} ${text.slice(0, 200)}`);
      return null;
    }
    return JSON.parse(text) as T;
  }
}
