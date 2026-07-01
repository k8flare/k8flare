import type { Condition } from "./types.ts";

/** Decode a kine value (base64 string or Uint8Array) to a UTF-8 string. */
export function decodeKineValue(v: string | ArrayLike<number>): string {
  if (typeof v === "string") return atob(v);
  return new TextDecoder().decode(new Uint8Array(v as ArrayLike<number>));
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
export function setCondition(statusObj: { conditions?: Condition[] }, condition: Condition): void {
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
