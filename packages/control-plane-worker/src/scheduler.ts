import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export class Scheduler extends WorkerEntrypoint<Env> {
  async poke(): Promise<void> {
    this.ctx.waitUntil(
      loadWasmWorker(this.env.LOADER, this.env.ASSETS, "scheduler", {
        APISERVER: this.env.APISERVER,
        ADMIN_TOKEN: this.env.ADMIN_TOKEN,
      }, 30_000)
        .then((worker) => worker.fetch("https://scheduler.internal/poke"))
        .then((resp) => {
          if (resp.status === 202) return this.env.SCHEDULER.poke();
        })
        .catch((err) => console.error("scheduler poke:", err)),
    );
  }
}
