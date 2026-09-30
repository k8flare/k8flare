import { WorkerEntrypoint } from "cloudflare:workers";
import { componentToken } from "./componenttoken.ts";
import { loadWasmWorker } from "@k8flare/loader-kit";
import { declaredImages } from "./podkubelet/images.generated.ts";

export interface ScheduleResult {
  bound: number;
  unschedulable: { ns: string; name: string; uid: string }[];
  retryAfterS?: number;
  attempt: number;
  skip?: boolean;
}

export class Scheduler extends WorkerEntrypoint<Env> {
  async schedule(messages: readonly { kind: string; attempt?: number }[]): Promise<ScheduleResult | null> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "scheduler", {
      APISERVER: this.env.APISERVER,
      API_TOKEN: await componentToken(this.env, "scheduler"),
      CONTAINERS_IMAGES: JSON.stringify(Object.keys(declaredImages)),
    }, this.env.APISERVER);
    const resp = await worker.fetch("https://scheduler.internal/schedule", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ messages }),
    });
    if (resp.status === 204) return { bound: 0, unschedulable: [], attempt: 0, skip: true };
    const text = await resp.text();
    if (resp.status !== 200) {
      console.log(`scheduler: schedule status=${resp.status} ${text.slice(0, 200)}`);
      return null;
    }
    return JSON.parse(text) as ScheduleResult;
  }
}
