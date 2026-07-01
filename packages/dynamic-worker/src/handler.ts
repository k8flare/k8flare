import { DW_PREFIX, DW_RESOURCE, DW_KIND, DW_LIST_KIND, DW_API_VERSION } from "./constants.ts";
import { dwValidate, dwApplyDefaults } from "./validation.ts";
import { handleDynamicWorkerRun } from "./run.ts";

/**
 * Handle DynamicWorker CRUD and /run subresource.
 *
 * If the request is POST with subresource="run", delegates to the run handler.
 * Otherwise delegates to the generic handleResourceCRUD with the DW config.
 */
export async function handleDynamicWorkerCRUD(
  req: Request,
  env: any,
  ctx: any,
  namespace: string,
  name: string,
  subresource: string,
  handleResourceCRUD: (
    req: Request,
    env: any,
    namespace: string,
    name: string,
    config: any,
  ) => Promise<Response>,
): Promise<Response> {
  // POST .../dynamicworkers/{name}/run -- execute
  if (subresource === "run" && name && req.method === "POST") {
    return handleDynamicWorkerRun(req, env, ctx, namespace, name);
  }

  return handleResourceCRUD(req, env, namespace, name, {
    prefix: DW_PREFIX,
    resource: DW_RESOURCE,
    kind: DW_KIND,
    listKind: DW_LIST_KIND,
    apiVersion: DW_API_VERSION,
    validate: dwValidate,
    applyDefaults: dwApplyDefaults,
    _subresource: subresource,
  });
}
