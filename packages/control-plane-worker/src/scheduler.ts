import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";
import { scheduleWake } from "./wake.ts";

const writeWindowMs = 20_000;

export class Scheduler extends WorkerEntrypoint<Env> {
  async poke(): Promise<void> {
    this.ctx.waitUntil(this.hold(writeWindowMs, 0));
  }

  async run(windowMs: number): Promise<void> {
    await this.hold(windowMs, windowMs);
  }

  private async hold(windowMs: number, minMs: number): Promise<void> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "scheduler", {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, this.env.APISERVER);
    const resp = await worker.fetch(`https://scheduler.internal/poke?window=${windowMs}&min=${minMs}`);
    const text = await resp.text();
    if (resp.status !== 200) return;
    const { next } = JSON.parse(text) as { next: number };
    if (next > 0) await scheduleWake(this.env, "scheduler", next);
  }
}
