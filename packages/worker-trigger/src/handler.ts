import type { CRDConfig } from "@k8flare/crd";
import { handleResourceCRUD } from "@k8flare/crd";
import { WT_PREFIX, WT_RESOURCE, WT_KIND, WT_LIST_KIND, WT_API_VERSION } from "./constants.ts";
import { wtValidate, wtApplyDefaults } from "./validation.ts";

/**
 * Handle WorkerTrigger CRUD requests.
 *
 * Thin wrapper around the generic handleResourceCRUD with WorkerTrigger config.
 * No special subresources are supported (unlike DynamicWorker which has /run).
 */
export async function handleWorkerTriggerCRUD(
  req: Request,
  env: any,
  _ctx: ExecutionContext,
  namespace: string,
  name: string,
  subresource: string,
): Promise<Response> {
  return handleResourceCRUD(req, env, namespace, name, {
    prefix: WT_PREFIX,
    resource: WT_RESOURCE,
    kind: WT_KIND,
    listKind: WT_LIST_KIND,
    apiVersion: WT_API_VERSION,
    validate: wtValidate,
    applyDefaults: wtApplyDefaults,
    _subresource: subresource,
  } satisfies CRDConfig);
}
