import { WT_KIND, WT_API_VERSION } from "./constants.ts";
import type { WorkerTriggerSpec } from "./types.ts";

/** Validate a WorkerTrigger spec. Returns an error string or null if valid. */
export function wtValidate(spec: WorkerTriggerSpec | undefined | null): string | null {
  if (!spec) return "spec is required";
  if (!spec.workerRef?.name) return "spec.workerRef.name is required";
  if (!spec.type) return "spec.type is required (http or cron)";
  if (spec.type !== "http" && spec.type !== "cron") return "spec.type must be http or cron";
  if (spec.type === "http" && !spec.http?.path) return "spec.http.path is required";
  if (spec.type === "cron" && !spec.cron?.schedule) return "spec.cron.schedule is required";
  return null;
}

/** Apply default values to a WorkerTrigger object. */
export function wtApplyDefaults(obj: any): void {
  if (!obj.apiVersion) obj.apiVersion = WT_API_VERSION;
  if (!obj.kind) obj.kind = WT_KIND;
  if (!obj.metadata) obj.metadata = {};
  if (!obj.metadata.namespace) obj.metadata.namespace = "default";
  if (!obj.metadata.uid) obj.metadata.uid = crypto.randomUUID();
  if (!obj.metadata.creationTimestamp) obj.metadata.creationTimestamp = new Date().toISOString();
  if (obj.metadata.generation === undefined) obj.metadata.generation = 1;
  if (!obj.status) obj.status = {};
}
