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
  names?: string[];
}

export class Workloads extends WorkerEntrypoint<Env> {
  async sync(changed: string[]): Promise<SyncResult | null> {
    return this.call<SyncResult>(`/sync?changed=${encodeURIComponent(changed.join(","))}`);
  }

  async namespaces(messages: readonly { kind: string; key?: string; names?: string[] }[]): Promise<NamespaceResult | null> {
    return this.call<NamespaceResult>("/namespaces", 90_000, JSON.stringify({ messages }));
  }

  async nodeHealth(node: string): Promise<NodeHealthResult | null> {
    return this.call<NodeHealthResult>(`/nodehealth?node=${encodeURIComponent(node)}`);
  }

  private async call<T>(path: string, timeoutMs = 120_000, body?: string): Promise<T | null> {
    const bindings: Record<string, unknown> = {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
      STORAGE: this.env.STORAGE_SVC,
      CLUSTER_SERVER: (this.env as Env & { GATEWAY_URL?: string }).GATEWAY_URL || "https://api.k8flare.com",
    };
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "workloads", bindings, this.env.APISERVER);
    const resp = await worker.fetch(`https://workloads.internal${path}`, {
      method: "POST",
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body,
      signal: AbortSignal.timeout(timeoutMs),
    });
    const text = await resp.text();
    if (resp.status !== 200) {
      console.log(`workloads: ${path} status=${resp.status} ${text.slice(0, 200)}`);
      return null;
    }
    return JSON.parse(text) as T;
  }
}
