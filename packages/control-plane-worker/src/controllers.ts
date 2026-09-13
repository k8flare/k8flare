import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export class Controllers extends WorkerEntrypoint<Env> {
  async poke(): Promise<void> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "controllers", {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, 30_000);
    this.ctx.waitUntil(
      worker.fetch("https://controllers.internal/poke").then((resp) => {
        if (resp.status === 202) return this.env.CONTROLLERS.poke();
      }).catch((err) => console.error("controllers poke:", err)),
    );
  }
}
