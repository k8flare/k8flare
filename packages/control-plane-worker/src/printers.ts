import { WorkerEntrypoint } from "cloudflare:workers";
import { loadWasmWorker } from "@k8flare/loader-kit";

export class Printers extends WorkerEntrypoint<Env> {
  async convertToTable(group: string, object: Uint8Array): Promise<Uint8Array> {
    const worker = await loadWasmWorker(this.env.LOADER, this.env.ASSETS, `printers-${group}`, {});
    const resp = await worker.fetch("https://printers.internal/table", { method: "POST", body: object });
    if (!resp.ok) throw new Error(`printers-${group}: HTTP ${resp.status}: ${await resp.text()}`);
    return new Uint8Array(await resp.arrayBuffer());
  }
}
