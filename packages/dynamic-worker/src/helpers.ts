import type { Condition } from "./types.ts";

/** Get the KineStore DO stub. */
export function dwStub(env: any): any {
  return env.KINE_STORE.get(env.KINE_STORE.idFromName("default"));
}

/** Decode a kine value (base64 string or Uint8Array) to a UTF-8 string. */
export function decodeKineValue(v: string | ArrayLike<number>): string {
  if (typeof v === "string") return atob(v);
  return new TextDecoder().decode(new Uint8Array(v as ArrayLike<number>));
}

/** Return a Response containing a Kubernetes Status error JSON body. */
export function dwError(status: number, message: string): Response {
  return Response.json(
    { kind: "Status", apiVersion: "v1", status: "Failure", message, code: status },
    { status },
  );
}

/** Construct a kine storage key for a namespaced resource. */
export function crKey(prefix: string, namespace: string, name: string): string {
  return prefix + namespace + "/" + name;
}

/** Result of a single CR get operation. */
export interface CRGetResult {
  obj: any;
  modRevision: number;
}

/** Fetch a single CR from the KineStore DO. */
export async function crGet(
  env: any,
  prefix: string,
  namespace: string,
  name: string,
): Promise<CRGetResult | null> {
  const key = crKey(prefix, namespace, name);
  const resp = await dwStub(env).fetch(
    new Request("http://do.internal/key" + key),
  );
  if (!resp.ok) return null;
  const body = await resp.json();
  if (!body.kv) return null;
  const v = body.kv.value;
  return { obj: JSON.parse(decodeKineValue(v)), modRevision: body.kv.modRevision };
}

/** Store a CR in the KineStore DO, optionally with a previous revision for CAS. */
export async function crPut(
  env: any,
  prefix: string,
  obj: any,
  prevRevision?: number,
): Promise<Response> {
  const key = crKey(prefix, obj.metadata.namespace, obj.metadata.name);
  const encoded = new TextEncoder().encode(JSON.stringify(obj));
  let binary = "";
  for (let i = 0; i < encoded.length; i++) binary += String.fromCharCode(encoded[i]);
  const value = btoa(binary);
  const reqBody: any = { value };
  if (prevRevision !== undefined) reqBody.revision = prevRevision;
  return dwStub(env).fetch(
    new Request("http://do.internal/key" + key, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(reqBody),
    }),
  );
}

/** Create a Kubernetes-style status condition. */
export function makeCondition(
  type: string,
  status: string,
  reason: string,
  message?: string,
): Condition {
  return {
    type,
    status,
    reason,
    message: message || "",
    lastTransitionTime: new Date().toISOString(),
  };
}

/**
 * Set (or update) a condition on a status object.
 *
 * If a condition with the same `type` already exists:
 *   - If `status` changed, the condition is fully replaced.
 *   - Otherwise, only `reason` and `message` are updated.
 * If no condition with the type exists, it is appended.
 */
export function setCondition(
  statusObj: { conditions?: Condition[] },
  condition: Condition,
): void {
  if (!statusObj.conditions) statusObj.conditions = [];
  const idx = statusObj.conditions.findIndex((c) => c.type === condition.type);
  if (idx >= 0) {
    const existing = statusObj.conditions[idx];
    if (existing.status !== condition.status) {
      statusObj.conditions[idx] = condition;
    } else {
      existing.reason = condition.reason;
      existing.message = condition.message;
    }
  } else {
    statusObj.conditions.push(condition);
  }
}
