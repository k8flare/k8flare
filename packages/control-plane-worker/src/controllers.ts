import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";
import { clusterHasNodes } from "./cluster.ts";
import { scheduleWake } from "./wake.ts";

const writeWindowMs = 20_000;
const safetyNetMs = 300_000;

export class Controllers extends WorkerEntrypoint<Env> {
  async poke(): Promise<void> {
    this.ctx.waitUntil(this.hold(writeWindowMs, 0));
  }

  async run(windowMs: number): Promise<void> {
    await this.hold(windowMs, windowMs);
  }

  private async hold(windowMs: number, minMs: number): Promise<void> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "controllers", {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, this.env.APISERVER);
    const resp = await worker.fetch(`https://controllers.internal/poke?window=${windowMs}&min=${minMs}`);
    const text = await resp.text();
    if (resp.status !== 200) return;
    let { next } = JSON.parse(text) as { next: number };
    if (next === 0 && (await clusterHasNodes(this.env))) next = safetyNetMs;
    if (next > 0) await scheduleWake(this.env, "controllers", next);
  }
}
