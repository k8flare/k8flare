import { WorkerEntrypoint } from "cloudflare:workers";
import { componentToken } from "./componenttoken.ts";
import { loadWasmWorker } from "@k8flare/loader-kit";

export interface SyncResult {
  objects: Record<string, number>;
  drained: boolean;
  nextMs: number;
}

export async function runSync(env: Env): Promise<SyncResult | null> {
  try {
    const worker = await loadWasmWorker(env.LOADER, env.ASSETS, "hpa", {
      APISERVER: env.APISERVER,
      API_TOKEN: await componentToken(env, "hpa"),
    }, env.APISERVER);
    const resp = await worker.fetch("https://hpa.internal/sync", { method: "POST", signal: AbortSignal.timeout(60_000) });
    const text = await resp.text();
    if (resp.status !== 200) {
      console.log(`hpa: status=${resp.status} ${text.slice(0, 200)}`);
      return null;
    }
    return JSON.parse(text) as SyncResult;
  } catch (err) {
    console.log(`hpa: load/fetch ${err}`);
    return null;
  }
}

export class HorizontalPodAutoscaler extends WorkerEntrypoint<Env> {
  sync(): Promise<SyncResult | null> {
    return runSync(this.env);
  }
}
