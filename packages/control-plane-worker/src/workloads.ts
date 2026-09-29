import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";
import { Span, Trace } from "./otel";

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

const SYNC_TIMEOUT_MS = 240_000;
const SYNC_RESERVE_MS = 20_000;

export class Workloads extends WorkerEntrypoint<Env> {
  async sync(changed: string[], traceparent?: string): Promise<SyncResult | null> {
    const trace = Trace.start(this.env, "workloads-rpc", traceparent);
    const root = trace.root("workloads.sync", traceparent, {
      "k8flare.changed": changed.join(",") || "(all)",
      "k8flare.changed_count": changed.length,
    });
    try {
      return await root.measure(() => this.syncInner(changed, root));
    } finally {
      await trace.flush();
    }
  }

  private async syncInner(changed: string[], parent: Span): Promise<SyncResult | null> {
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
      const path = `/sync?changed=${encodeURIComponent(list.join(","))}&budgetMs=${SYNC_TIMEOUT_MS - SYNC_RESERVE_MS}`;
      const r = await this.call<SyncResult>(path, SYNC_TIMEOUT_MS, undefined, worker, parent);
      if (r) results.push(r);
    }
    if (results.length === 0) return null;
    return results.reduce((acc, r) => ({
      objects: { ...acc.objects, ...r.objects },
      drained: acc.drained || r.drained,
      nextMs: acc.nextMs === 0 ? r.nextMs : r.nextMs === 0 ? acc.nextMs : Math.min(acc.nextMs, r.nextMs),
    }));
  }

  async namespaces(messages: readonly { kind: string; key?: string; names?: string[] }[], traceparent?: string): Promise<NamespaceResult | null> {
    const trace = Trace.start(this.env, "workloads-rpc", traceparent);
    const root = trace.root("workloads.namespaces", traceparent, { "k8flare.messages": messages.length });
    try {
      return await root.measure(() => this.call<NamespaceResult>("/namespaces", 90_000, JSON.stringify({ messages }), "workloads", root));
    } finally {
      await trace.flush();
    }
  }

  async nodeHealth(node: string): Promise<NodeHealthResult | null> {
    return this.call<NodeHealthResult>(`/nodehealth?node=${encodeURIComponent(node)}`);
  }

  private async call<T>(path: string, timeoutMs = 120_000, body?: string, name = "workloads", parent?: Span): Promise<T | null> {
    const bindings: Record<string, unknown> = {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
      STORAGE: this.env.STORAGE_SVC,
      CLUSTER_SERVER: (this.env as Env & { GATEWAY_URL?: string }).GATEWAY_URL || "https://api.k8flare.com",
    };
    const load = parent?.child("loader.load", { "k8flare.worker": name });
    const loadWorker = () => loadWasmWorker(this.env.LOADER, this.env.ASSETS, name, bindings, this.env.APISERVER);
    const worker = load ? await load.measure(loadWorker) : await loadWorker();
    const call = parent?.child("worker.fetch", {
      "k8flare.worker": name,
      "k8flare.path": path.split("?")[0],
      "k8flare.timeout_ms": timeoutMs,
    });
    const doFetch = () => worker.fetch(`https://workloads.internal${path}`, {
      method: "POST",
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body,
      signal: AbortSignal.timeout(timeoutMs),
    });
    const resp = call ? await call.measure(doFetch) : await doFetch();
    const read = parent?.child("worker.body", { "k8flare.worker": name });
    const text = read ? await read.measure(() => resp.text()) : await resp.text();
    if (resp.status !== 200) {
      console.log(`${name}: ${path} status=${resp.status} ${text.slice(0, 200)}`);
      return null;
    }
    return JSON.parse(text) as T;
  }
}
