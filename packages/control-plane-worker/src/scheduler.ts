import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";
import { absorbedRetryMs, pokeDelayMs, scheduleWake, settleWake } from "./wake.ts";

export class Scheduler extends WorkerEntrypoint<Env> {
  async poke(): Promise<void> {
    await scheduleWake(this.env, "scheduler", pokeDelayMs, 0, true, true);
    this.ctx.waitUntil(this.kick());
  }

  async run(windowMs: number, minMs: number, reset: boolean): Promise<void> {
    await this.hold(windowMs, minMs, reset);
  }

  private async worker(): Promise<Fetcher> {
    return loadWasmWorker(this.env.LOADER, this.env.ASSETS, "scheduler", {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, this.env.APISERVER);
  }

  private async kick(): Promise<void> {
    const resp = await (await this.worker()).fetch(`https://scheduler.internal/poke?kick=1`);
    await resp.text();
  }

  private async hold(windowMs: number, minMs: number, reset: boolean): Promise<void> {
    const resp = await (await this.worker()).fetch(`https://scheduler.internal/poke?window=${windowMs}&min=${minMs}&reset=${reset ? 1 : 0}`);
    const text = await resp.text();
    if (resp.status === 204 && minMs > 0) await scheduleWake(this.env, "scheduler", absorbedRetryMs, minMs);
    if (resp.status !== 200) return;
    const { next } = JSON.parse(text) as { next: number };
    if (next > 0) await scheduleWake(this.env, "scheduler", next);
    else if (next === 0) await settleWake(this.env, "scheduler");
  }
}
