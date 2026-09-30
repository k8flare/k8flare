import { WorkerEntrypoint } from "cloudflare:workers";
import { isolateId, loadWasmWorker } from "@k8flare/loader-kit";

const workerName = /^apiserver-[a-z]+$/;

export class APIGroups extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const name = request.headers.get("X-K8flare-Worker") ?? "";
    if (!workerName.test(name)) return new Response(`unknown worker ${name}`, { status: 400 });
    console.log(`apigroups iso=${isolateId()} ${name}`);
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, name, {
      STORAGE: this.env.STORAGE_SVC,
      PRINTERS: this.env.PRINTERS,
      TUNNEL: this.env.TUNNEL,
      ADMISSION: this.env.ADMISSION,
      ADMIN_TOKEN: this.env.ADMIN_TOKEN,
      READONLY_TOKEN: this.env.READONLY_TOKEN,
      SECRETS_ENCRYPTION_KEYS: this.env.SECRETS_ENCRYPTION_KEYS,
    }, this.env.APISERVER);
    return worker.fetch(request);
  }
}
