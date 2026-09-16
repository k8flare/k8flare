import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";
import { pokeDeadlineMs, retryMs, scheduleWake } from "./wake.ts";

const writeWindowMs = 20_000;

export class Scheduler extends WorkerEntrypoint<Env> {
  async poke(): Promise<void> {
    this.ctx.waitUntil(this.pokeWithDeadline());
  }

  async run(windowMs: number): Promise<void> {
    await this.hold(windowMs, 0, false);
  }

  private async pokeWithDeadline(): Promise<void> {
    const deadline = new Promise<"deadline">((resolve) => setTimeout(() => resolve("deadline"), pokeDeadlineMs));
    if ((await Promise.race([this.hold(writeWindowMs, 0, true), deadline])) === "deadline") {
      await scheduleWake(this.env, "scheduler", retryMs);
    }
  }

  private async hold(windowMs: number, minMs: number, reset: boolean): Promise<void> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "scheduler", {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, this.env.APISERVER);
    const resp = await worker.fetch(`https://scheduler.internal/poke?window=${windowMs}&min=${minMs}&reset=${reset ? 1 : 0}`);
    const text = await resp.text();
    if (resp.status !== 200) return;
    const { next } = JSON.parse(text) as { next: number };
    if (next > 0) await scheduleWake(this.env, "scheduler", next);
  }
}
