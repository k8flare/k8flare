import { crList } from "@k8flare/crd";
import { dwError } from "@k8flare/k8s";
import { WT_PREFIX } from "./constants.ts";

/**
 * Callback type for executing a DynamicWorker by name.
 *
 * Accepts the same signature as handleDynamicWorkerRun from @k8flare/dynamic-worker.
 */
export type RunWorkerFn = (
  req: Request,
  env: any,
  ctx: ExecutionContext,
  namespace: string,
  name: string,
) => Promise<Response>;

/**
 * Handle an HTTP trigger request.
 *
 * Extracts the trigger path from the URL (everything after /trigger),
 * searches for a matching WorkerTrigger of type "http", and dispatches
 * to the referenced DynamicWorker via the provided runWorker callback.
 */
export async function handleHTTPTrigger(
  req: Request,
  env: any,
  ctx: ExecutionContext,
  url: URL,
  runWorker: RunWorkerFn,
): Promise<Response> {
  const triggerPath = url.pathname.slice("/trigger".length);
  const triggers = await crList(env, WT_PREFIX, "");
  const match = triggers.find(
    (t: any) =>
      t.spec?.type === "http" &&
      t.spec.http?.path === triggerPath &&
      (!t.spec.http.methods?.length || t.spec.http.methods.includes(req.method)),
  );
  if (!match) return dwError(404, `no trigger matches "${triggerPath}"`);
  return runWorker(req, env, ctx, match.metadata.namespace, match.spec.workerRef.name);
}
