import { WorkerEntrypoint } from "cloudflare:workers";
import { isolateId, loadWasmWorker } from "@k8flare/loader-kit";

const workerName = /^apiserver-[a-z]+$/;

export class APIGroups extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const name = request.headers.get("X-K8flare-Worker") ?? "";
    if (!workerName.test(name)) return new Response(`unknown worker ${name}`, { status: 400 });
    console.log(`apigroups iso=${isolateId()} ${name}`);
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, name, {
      STORAGE: this.env.CLUSTER.get(this.env.CLUSTER.idFromName("default")),
      PRINTERS: this.env.PRINTERS,
      SCHEDULER: this.env.SCHEDULER,
      CONTROLLERS: this.env.CONTROLLERS,
      TUNNEL: this.env.TUNNEL,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
      READONLY_TOKEN: this.env.READONLY_TOKEN,
    }, this.env.APISERVER);
    return worker.fetch(request);
  }
}
