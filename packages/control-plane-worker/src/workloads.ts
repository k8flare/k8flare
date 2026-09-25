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

// ValidatingAdmissionPolicy runs in a worker of its own. Its CEL type checker
// closure is 13 MB, nothing else needs it, and controllerNeeds gives it
// validatingadmissionpolicies alone, so a pod write never pays for CEL.
const VAP_RESOURCE = "validatingadmissionpolicies";
const VAP_WORKER = "workloads-vap";

export class Workloads extends WorkerEntrypoint<Env> {
  async sync(changed: string[]): Promise<SyncResult | null> {
    // An empty list means "run everything", which each worker reads for the
    // shards it carries, so both are asked.
    const workers = changed.length === 0
      ? [[VAP_WORKER, changed] as const, ["workloads", changed] as const]
      : ([
          [VAP_WORKER, changed.filter((c) => c === VAP_RESOURCE)] as const,
          ["workloads", changed.filter((c) => c !== VAP_RESOURCE)] as const,
        ].filter(([, list]) => list.length > 0));
    const results: SyncResult[] = [];
    for (const [worker, list] of workers) {
      const r = await this.call<SyncResult>(`/sync?changed=${encodeURIComponent(list.join(","))}`, 120_000, undefined, worker);
      if (r) results.push(r);
    }
    if (results.length === 0) return null;
    return results.reduce((acc, r) => ({
      objects: { ...acc.objects, ...r.objects },
      drained: acc.drained || r.drained,
      nextMs: acc.nextMs === 0 ? r.nextMs : r.nextMs === 0 ? acc.nextMs : Math.min(acc.nextMs, r.nextMs),
    }));
  }

  async namespaces(messages: readonly { kind: string; key?: string; names?: string[] }[]): Promise<NamespaceResult | null> {
    return this.call<NamespaceResult>("/namespaces", 90_000, JSON.stringify({ messages }));
  }

  async nodeHealth(node: string): Promise<NodeHealthResult | null> {
    return this.call<NodeHealthResult>(`/nodehealth?node=${encodeURIComponent(node)}`);
  }

  private async call<T>(path: string, timeoutMs = 120_000, body?: string, name = "workloads"): Promise<T | null> {
    const bindings: Record<string, unknown> = {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
      STORAGE: this.env.STORAGE_SVC,
      CLUSTER_SERVER: (this.env as Env & { GATEWAY_URL?: string }).GATEWAY_URL || "https://api.k8flare.com",
    };
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, name, bindings, this.env.APISERVER);
    const resp = await worker.fetch(`https://workloads.internal${path}`, {
      method: "POST",
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body,
      signal: AbortSignal.timeout(timeoutMs),
    });
    const text = await resp.text();
    if (resp.status !== 200) {
      console.log(`${name}: ${path} status=${resp.status} ${text.slice(0, 200)}`);
      return null;
    }
    return JSON.parse(text) as T;
  }
}
