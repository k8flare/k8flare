import { DW_API_VERSION, DW_KIND } from "./constants.ts";
import type { DynamicWorkerSpec } from "./types.ts";

/** Validate DynamicWorker spec. Returns an error string or null if valid. */
export function dwValidate(spec: DynamicWorkerSpec | undefined | null): string | null {
  if (!spec) return "spec is required";
  if (!spec.modules || typeof spec.modules !== "object" || Object.keys(spec.modules).length === 0)
    return "spec.modules is required and must contain at least one module";
  const main = spec.mainModule || "index.js";
  if (!(main in spec.modules)) return `spec.mainModule "${main}" not found in spec.modules`;
  if (
    spec.networkAccess !== undefined &&
    spec.networkAccess !== "none" &&
    spec.networkAccess !== "inherit"
  ) {
    return `spec.networkAccess must be "none" or "inherit"`;
  }
  return null;
}

/** Apply default values to a DynamicWorker object. */
export function dwApplyDefaults(obj: any): void {
  if (!obj.apiVersion) obj.apiVersion = DW_API_VERSION;
  if (!obj.kind) obj.kind = DW_KIND;
  if (!obj.metadata) obj.metadata = {};
  if (!obj.metadata.namespace) obj.metadata.namespace = "default";
  if (!obj.metadata.uid) obj.metadata.uid = crypto.randomUUID();
  if (!obj.metadata.creationTimestamp) obj.metadata.creationTimestamp = new Date().toISOString();
  if (obj.metadata.generation === undefined) obj.metadata.generation = 1;
  if (!obj.spec) obj.spec = {};
  if (!obj.spec.mainModule) obj.spec.mainModule = "index.js";
  if (!obj.spec.compatibilityDate) obj.spec.compatibilityDate = "2026-03-24";
  if (obj.spec.networkAccess === undefined) obj.spec.networkAccess = "none";
  if (!obj.status) obj.status = {};
}
