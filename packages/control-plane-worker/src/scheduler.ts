import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

const writeWindowMs = 20_000;
const cronWindowMs = 290_000;

export class Scheduler extends WorkerEntrypoint<Env> {
  async poke(): Promise<void> {
    this.ctx.waitUntil(this.hold(writeWindowMs));
  }

  async run(): Promise<void> {
    await this.hold(cronWindowMs);
  }

  private async hold(windowMs: number): Promise<void> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "scheduler", {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, this.env.APISERVER);
    const resp = await worker.fetch(`https://scheduler.internal/poke?window=${windowMs}`);
    await resp.text();
  }
}
