import { crPut, makeCondition, setCondition } from "./helpers.ts";
import { DW_PREFIX } from "./constants.ts";

/**
 * Update a DynamicWorker's status with a LastRun condition.
 *
 * This is a fire-and-forget operation: errors are silently caught
 * so they do not affect the response returned to the caller.
 */
export function updateRunStatus(
  env: any,
  obj: any,
  modRevision: number,
  succeeded: boolean,
  message: string,
): void {
  const statusObj = obj.status || {};
  statusObj.observedGeneration = obj.metadata.generation || 1;
  statusObj.lastRunTime = new Date().toISOString();
  setCondition(
    statusObj,
    makeCondition(
      "LastRun",
      succeeded ? "True" : "False",
      succeeded ? "Succeeded" : "Failed",
      message,
    ),
  );
  obj.status = statusObj;
  crPut(env, DW_PREFIX, obj, modRevision).catch(() => {});
}
