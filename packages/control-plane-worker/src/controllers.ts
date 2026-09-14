import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export class Controllers extends WorkerEntrypoint<Env> {
  // poke returns as soon as the wake is under way: a write must not wait
  // for a cold worker. run() is the opposite, and the Cron Trigger uses
  // it, because a scheduled invocation that returns early has its
  // waitUntil work cancelled before the worker has even loaded.
  async poke(): Promise<void> {
    this.ctx.waitUntil(this.run());
  }

  async run(): Promise<void> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "controllers", {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, 30_000);
    const resp = await worker.fetch("https://controllers.internal/poke");
    if (resp.status === 202) return this.env.CONTROLLERS.run();
  }
}
