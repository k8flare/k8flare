import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

const workerName = /^apiserver-[a-z]+$/;

export class APIGroups extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const name = request.headers.get("X-K8flare-Worker") ?? "";
    if (!workerName.test(name)) return new Response(`unknown worker ${name}`, { status: 400 });
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, name, {
      STORAGE: this.env.CLUSTER.get(this.env.CLUSTER.idFromName("default")),
      PRINTERS: this.env.PRINTERS,
      SCHEDULER: this.env.SCHEDULER,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
      KUBELET_SCHEME: this.env.KUBELET_SCHEME,
      KUBELET_PORT: this.env.KUBELET_PORT,
    }, 5_000);
    return worker.fetch(request);
  }
}
