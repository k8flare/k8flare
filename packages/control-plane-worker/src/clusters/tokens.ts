import { clusterName } from "../clusterid.ts";

const joinKey = "/vault/tokens/join";
const adminKey = "/vault/tokens/admin";

export async function clusterSecrets(env: Env, name: string): Promise<[string | undefined]> {
  const stub = env.CLUSTER.get(env.CLUSTER.idFromName(name || clusterName(env)));
  const join = await readToken(stub, joinKey);
  if (join) return [join];
  const seed = env.JOIN_TOKEN || env.ADMIN_TOKEN;
  if (!seed || (name && name !== "default" && name !== (env.CLUSTER_UID || "default"))) {
    return [undefined];
  }
  await writeToken(stub, joinKey, seed);
  return [seed];
}

export async function clusterAdminToken(env: Env): Promise<string> {
  const stub = env.CLUSTER.get(env.CLUSTER.idFromName(clusterName(env)));
  const token = await readToken(stub, adminKey);
  return token || env.ADMIN_TOKEN;
}

async function readToken(stub: DurableObjectStub, key: string): Promise<string | undefined> {
  const resp = await stub.fetch(`https://cluster.internal/kv?key=${encodeURIComponent(key)}`);
  if (!resp.ok) return undefined;
  const data = (await resp.json()) as { kv: { value: string } | null };
  if (!data.kv) return undefined;
  try {
    return atob(data.kv.value);
  } catch {
    return undefined;
  }
}

async function writeToken(stub: DurableObjectStub, key: string, value: string): Promise<void> {
  await stub.fetch("https://cluster.internal/kv", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ key, value: btoa(value), revision: 0 }),
  });
}
