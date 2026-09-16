import { loadWasmWorker } from "@k8flare/loader-kit";

export async function apiserverFetch(env: Env, ctx: ExecutionContext, request: Request): Promise<Response> {
  const worker = await loadWasmWorker(env.LOADER, env.ASSETS, "apiserver", {
    STORAGE: env.CLUSTER.get(env.CLUSTER.idFromName("default")),
    APIGROUPS: ctx.exports.APIGroups,
    OPENAPI: ctx.exports.OpenAPI,
    CUSTOMRESOURCES: ctx.exports.CustomResources,
    ADMIN_TOKEN: env.ADMIN_TOKEN,
    READONLY_TOKEN: env.READONLY_TOKEN,
    JOIN_TOKEN: env.JOIN_TOKEN,
  });
  return worker.fetch(request);
}
