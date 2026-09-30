import { WorkerEntrypoint } from "cloudflare:workers";
import { componentToken } from "./componenttoken.ts";
import { loadWasmWorker } from "@k8flare/loader-kit";

export class Admission extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "admission", {
      STORAGE: this.env.STORAGE_SVC,
      TUNNEL: this.env.TUNNEL,
      HOOKS: this.env.HOOKS,
      OUTBOUND: this.env.OUTBOUND,
      APISERVER: this.env.APISERVER,
      API_TOKEN: await componentToken(this.env, "admission"),
      SECRETS_ENCRYPTION_KEYS: this.env.SECRETS_ENCRYPTION_KEYS,
    }, this.env.APISERVER);
    return worker.fetch(request);
  }
}
