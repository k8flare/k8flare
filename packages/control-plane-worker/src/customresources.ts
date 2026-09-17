import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export class CustomResources extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, "customresources", {
      STORAGE: this.env.CLUSTER.get(this.env.CLUSTER.idFromName("default")),
    }, this.env.APISERVER);
    const method = request.method;
    const path = new URL(request.url).pathname;
    const resp = await worker.fetch(request);
    console.log(
      `crd-diag inst=${resp.headers.get("X-CRD-Instance")} refill=${resp.headers.get("X-CRD-Refill")} rv=${resp.headers.get("X-CRD-RV")} status=${resp.status} ${method} ${path}`,
    );
    return resp;
  }
}
