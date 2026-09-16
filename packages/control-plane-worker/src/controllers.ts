import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";
import { absorbedRetryMs, insuranceMs, scheduleWake, settleWake } from "./wake.ts";

const writeWindowMs = 20_000;

export class Controllers extends WorkerEntrypoint<Env> {
  async poke(): Promise<void> {
    await scheduleWake(this.env, "controllers", insuranceMs, 0, true);
    this.ctx.waitUntil(this.hold(writeWindowMs, 0, true));
  }

  async run(windowMs: number, minMs: number): Promise<void> {
    await this.hold(windowMs, minMs, false);
  }

  private async hold(windowMs: number, minMs: number, reset: boolean): Promise<void> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "controllers", {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, this.env.APISERVER);
    const resp = await worker.fetch(`https://controllers.internal/poke?window=${windowMs}&min=${minMs}&reset=${reset ? 1 : 0}`);
    const text = await resp.text();
    if (resp.status === 204 && minMs > 0) await scheduleWake(this.env, "controllers", absorbedRetryMs, minMs);
    if (resp.status !== 200) return;
    const { next } = JSON.parse(text) as { next: number };
    if (next > 0) await scheduleWake(this.env, "controllers", next);
    else await settleWake(this.env, "controllers");
  }
}
