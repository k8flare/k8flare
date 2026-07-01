/**
 * Apply common default values to a custom resource object.
 *
 * Sets apiVersion, kind, metadata.namespace, metadata.uid,
 * metadata.creationTimestamp, metadata.generation, and initializes
 * an empty status if not present.
 */
export function applyCommonDefaults(obj: any, apiVersion: string, kind: string): void {
  if (!obj.apiVersion) obj.apiVersion = apiVersion;
  if (!obj.kind) obj.kind = kind;
  if (!obj.metadata) obj.metadata = {};
  if (!obj.metadata.namespace) obj.metadata.namespace = "default";
  if (!obj.metadata.uid) obj.metadata.uid = crypto.randomUUID();
  if (!obj.metadata.creationTimestamp) obj.metadata.creationTimestamp = new Date().toISOString();
  if (obj.metadata.generation === undefined) obj.metadata.generation = 1;
  if (!obj.status) obj.status = {};
}
