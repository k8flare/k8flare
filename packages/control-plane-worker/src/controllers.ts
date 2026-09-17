import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export interface RunResult {
  status: number;
  nextMs: number;
  retryAfterMs: number;
}

export class Controllers extends WorkerEntrypoint<Env> {
  async run(windowMs: number, minMs: number, reset: boolean): Promise<RunResult> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "controllers", {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, this.env.APISERVER);
    const resp = await worker.fetch(`https://controllers.internal/poke?window=${windowMs}&min=${minMs}&reset=${reset ? 1 : 0}`);
    const text = await resp.text();
    const retryAfterMs = Number(resp.headers.get("X-Retry-After-Ms") ?? "0") || 0;
    if (resp.status !== 200) return { status: resp.status, nextMs: 0, retryAfterMs };
    const { next } = JSON.parse(text) as { next: number };
    return { status: 200, nextMs: next, retryAfterMs };
  }
}
