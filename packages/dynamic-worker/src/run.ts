import { crGet, dwError } from "./helpers.ts";
import { DW_PREFIX, DW_RESOURCE } from "./constants.ts";
import { resolveEnvFrom } from "./env-from.ts";
import { updateRunStatus } from "./status.ts";

/**
 * Execute a DynamicWorker's code via the LOADER binding.
 *
 * Reads the DynamicWorker resource from kine, constructs WorkerCode from its
 * spec, and runs it in a sandboxed V8 isolate. The request body is passed as
 * input to the worker's fetch handler.
 */
export async function handleDynamicWorkerRun(
  req: Request,
  env: any,
  ctx: any,
  namespace: string,
  name: string,
): Promise<Response> {
  const result = await crGet(env, DW_PREFIX, namespace, name);
  if (!result) return dwError(404, `${DW_RESOURCE} "${name}" not found`);

  const obj = result.obj;
  const spec = obj.spec;

  // Build modules map — handle typed module variants
  const modules: Record<string, any> = {};
  for (const [modName, modContent] of Object.entries(spec.modules || {})) {
    if (typeof modContent === "string") {
      modules[modName] = modContent;
    } else if (typeof modContent === "object" && modContent !== null) {
      // Typed module: { js, cjs, py, text, json }
      // data (ArrayBuffer) is not supported via JSON API
      modules[modName] = modContent;
    }
  }

  // Determine globalOutbound from networkAccess
  let globalOutbound: null | undefined;
  switch (spec.networkAccess) {
    case "none":
      globalOutbound = null;
      break;
    case "inherit":
      globalOutbound = undefined;
      break;
    default:
      globalOutbound = null;
      break;
  }

  const workerCode: any = {
    compatibilityDate: spec.compatibilityDate || "2026-03-24",
    mainModule: spec.mainModule || "index.js",
    modules,
    globalOutbound,
  };

  if (spec.compatibilityFlags?.length) {
    workerCode.compatibilityFlags = spec.compatibilityFlags;
  }

  // Resolve envFrom (secrets/configmaps) and merge with spec.env
  let workerEnv: Record<string, string> = {};
  if (spec.env && typeof spec.env === "object") {
    workerEnv = { ...spec.env };
  }
  try {
    const fromEnv = await resolveEnvFrom(env, obj.metadata.namespace, spec.envFrom);
    workerEnv = { ...fromEnv, ...workerEnv }; // spec.env takes precedence
  } catch (err: any) {
    return dwError(400, err.message);
  }
  if (Object.keys(workerEnv).length > 0) {
    workerCode.env = workerEnv;
  }

  try {
    // Use get() with resource UID + generation as cache key for stable IDs
    const cacheId = `${namespace}/${name}:${obj.metadata.uid}:${obj.metadata.resourceVersion || "0"}`;
    const worker = env.LOADER.get(cacheId, () => workerCode);

    // Forward the incoming request body as input
    const input = req.headers.get("Content-Type")?.includes("json")
      ? await req.text()
      : "{}";

    const entrypointName = spec.entrypoint || undefined;
    const entrypoint = worker.getEntrypoint(entrypointName);

    const proxyReq = new Request("http://internal/run", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: input,
    });

    const response = await entrypoint.fetch(proxyReq);

    // Best-effort status update using conditions pattern
    updateRunStatus(env, obj, result.modRevision, true, "");

    // Return the Dynamic Worker's response directly
    return new Response(response.body, {
      status: response.status,
      headers: response.headers,
    });
  } catch (err: any) {
    updateRunStatus(env, obj, result.modRevision, false, err.message || String(err));
    return Response.json({ ok: false, error: err.message || String(err) }, { status: 500 });
  }
}
