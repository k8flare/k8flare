import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export interface ScheduleResult {
  bound: number;
  unschedulable: { ns: string; name: string; uid: string }[];
}

export class Scheduler extends WorkerEntrypoint<Env> {
  async schedule(): Promise<ScheduleResult | null> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "scheduler", {
      APISERVER: this.env.APISERVER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
    }, this.env.APISERVER);
    const resp = await worker.fetch("https://scheduler.internal/schedule", { method: "POST" });
    const text = await resp.text();
    if (resp.status !== 200) {
      console.log(`scheduler: schedule status=${resp.status} ${text.slice(0, 200)}`);
      return null;
    }
    return JSON.parse(text) as ScheduleResult;
  }
}
