/**
 * Compute the exclusive upper bound for a prefix range query.
 * For "/registry/pods/", returns "/registry/pods0" (0x30 = '/' + 1).
 * This allows replacing LIKE 'prefix%' with name >= prefix AND name < prefixEnd.
 */
export function prefixEnd(prefix: string): string {
  if (prefix.length === 0) return "\xff";
  const last = prefix.charCodeAt(prefix.length - 1);
  return prefix.slice(0, -1) + String.fromCharCode(last + 1);
}

export interface KineRow {
  current_rev: number;
  compact_rev: number | null;
  theid: number;
  thename: string;
  created: number;
  deleted: number;
  create_revision: number;
  prev_revision: number;
  lease: number;
  value: ArrayBuffer | string | null;
  old_value: ArrayBuffer | string | null;
}

export interface KineKV {
  key: string;
  modRevision: number;
  createRevision: number;
  value: string | null;
  lease: number;
}

export interface KineEvent {
  kv: KineKV;
  create: boolean;
  delete: boolean;
  prevKV?: {
    modRevision: number;
    value: string | null;
  };
}

export function rowToEvent(row: KineRow): KineEvent {
  const event: KineEvent = {
    kv: {
      key: row.thename,
      modRevision: row.theid,
      createRevision: row.create_revision,
      value: row.value ? arrayBufferToBase64(row.value) : null,
      lease: row.lease,
    },
    create: row.created === 1,
    delete: row.deleted === 1,
  };
  if (event.create) {
    event.kv.createRevision = event.kv.modRevision;
  } else {
    event.prevKV = {
      modRevision: row.prev_revision,
      value: row.old_value ? arrayBufferToBase64(row.old_value) : null,
    };
  }
  return event;
}

export function arrayBufferToBase64(buffer: ArrayBuffer | string | null): string | null {
  if (buffer === null || buffer === undefined) return null;
  if (typeof buffer === "string") return btoa(buffer);
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.byteLength; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

export function base64ToArrayBuffer(base64: string | null): ArrayBuffer | null {
  if (!base64) return null;
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

export function jsonResponse(data: unknown, status: number = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
