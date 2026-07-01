import type { CRGetResult } from "./types.ts";

/** Decode a kine value (base64 string or Uint8Array) to a UTF-8 string. */
function decodeKineValue(v: string | ArrayLike<number>): string {
  if (typeof v === "string") return atob(v);
  return new TextDecoder().decode(new Uint8Array(v as ArrayLike<number>));
}

/** Get the Etcd DO stub. */
export function dwStub(env: any): any {
  return env.ETCD.get(env.ETCD.idFromName("default"));
}

/** Construct a kine storage key for a namespaced resource. */
export function crKey(prefix: string, namespace: string, name: string): string {
  return prefix + namespace + "/" + name;
}

/** Construct a kine list prefix, optionally scoped to a namespace. */
export function crListPrefix(prefix: string, namespace: string): string {
  return namespace ? prefix + namespace + "/" : prefix;
}

/** Fetch a single CR from the Etcd DO. */
export async function crGet(
  env: any,
  prefix: string,
  namespace: string,
  name: string,
): Promise<CRGetResult | null> {
  const key = crKey(prefix, namespace, name);
  const resp = await dwStub(env).fetch(new Request("http://do.internal/key" + key));
  if (!resp.ok) return null;
  const body = await resp.json();
  if (!body.kv) return null;
  const v = body.kv.value;
  return { obj: JSON.parse(decodeKineValue(v)), modRevision: body.kv.modRevision };
}

/** List CRs from the Etcd DO, optionally scoped to a namespace. */
export async function crList(env: any, prefix: string, namespace: string): Promise<any[]> {
  const lp = crListPrefix(prefix, namespace);
  const resp = await dwStub(env).fetch(new Request("http://do.internal/list" + lp));
  if (!resp.ok) return [];
  const body = await resp.json();
  return (body.kvs || []).map((kv: any) => {
    return JSON.parse(decodeKineValue(kv.value));
  });
}

/** Store a CR in the Etcd DO, optionally with a previous revision for CAS. */
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

/** Delete a CR from the Etcd DO. */
export async function crDelete(
  env: any,
  prefix: string,
  namespace: string,
  name: string,
): Promise<Response> {
  const key = crKey(prefix, namespace, name);
  return dwStub(env).fetch(new Request("http://do.internal/key" + key, { method: "DELETE" }));
}
