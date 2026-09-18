import { loadWasmWorker } from "@k8flare/loader-kit";

export async function apiserverFetch(env: Env, request: Request): Promise<Response> {
  const worker = await loadWasmWorker(env.LOADER, env.ASSETS, "apiserver", {
    STORAGE: env.STORAGE_SVC,
    APIGROUPS: env.APIGROUPS,
    OPENAPI: env.OPENAPI,
    CUSTOMRESOURCES: env.CUSTOMRESOURCES,
    ADMIN_TOKEN: env.ADMIN_TOKEN,
    READONLY_TOKEN: env.READONLY_TOKEN,
    JOIN_TOKEN: env.JOIN_TOKEN,
  }, env.APISERVER);
  return worker.fetch(request);
}
