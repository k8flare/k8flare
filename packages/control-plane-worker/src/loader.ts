import { loadWasmWorker } from "@k8flare/loader-kit";

// The Go apiserver as a dynamic worker. Its env carries the Cluster DO's
// stub as STORAGE and the printers Worker as PRINTERS; a dynamic worker can
// be handed Fetchers, not bindings.
export async function apiserverFetch(env: Env, request: Request): Promise<Response> {
  const worker = await loadWasmWorker(env.LOADER, env.ASSETS, "apiserver", {
    STORAGE: env.CLUSTER.get(env.CLUSTER.idFromName("default")),
    PRINTERS: env.PRINTERS,
    ADMIN_TOKEN: env.ADMIN_TOKEN,
    JOIN_TOKEN: env.JOIN_TOKEN,
    KUBELET_SCHEME: env.KUBELET_SCHEME,
    KUBELET_PORT: env.KUBELET_PORT,
  });
  return worker.fetch(request);
}
