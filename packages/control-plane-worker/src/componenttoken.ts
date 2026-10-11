import { clusterName, clusterStub } from "./clusterid.ts";

export type Component = "scheduler" | "gc" | "hpa" | "attachdetach" | "admission" | "workloads" | "addons" | "hookecho" | "podkubelet" | "nodetunnel" | "queues" | "nodes";

const encoder = new TextEncoder();
const componentKeyPath = "/vault/component-key";
const componentKeys = new Map<string, Promise<CryptoKey>>();

function hex(bytes: Uint8Array): string {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

async function readComponentKey(stub: DurableObjectStub): Promise<string | null> {
  const resp = await stub.fetch(`https://cluster.internal/kv?key=${encodeURIComponent(componentKeyPath)}`);
  if (!resp.ok) throw new Error(`component key: ${resp.status}`);
  const data = (await resp.json()) as { kv?: { value: string } | null };
  return data.kv ? atob(data.kv.value) : null;
}

async function createComponentKey(stub: DurableObjectStub): Promise<string | null> {
  const fresh = hex(crypto.getRandomValues(new Uint8Array(32)));
  const created = await stub.fetch("https://cluster.internal/kv", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ key: componentKeyPath, value: btoa(fresh), revision: 0 }),
  });
  if (created.ok) return fresh;
  if (created.status === 409) return readComponentKey(stub);
  throw new Error(`component key: ${created.status}`);
}

async function loadComponentKey(env: Env): Promise<CryptoKey> {
  const stub = clusterStub(env);
  const key = (await readComponentKey(stub)) ?? (await createComponentKey(stub));
  if (key === null) throw new Error("component key: missing after a conflicting create");
  return crypto.subtle.importKey("raw", encoder.encode(key), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
}

function componentKey(env: Env): Promise<CryptoKey> {
  const name = clusterName(env);
  let key = componentKeys.get(name);
  if (!key) {
    key = loadComponentKey(env);
    key.catch(() => componentKeys.delete(name));
    componentKeys.set(name, key);
  }
  return key;
}

export async function componentToken(env: Env, component: Component): Promise<string> {
  const mac = new Uint8Array(await crypto.subtle.sign("HMAC", await componentKey(env), encoder.encode(`k8flare-component:${component}`)));
  return `component:${component}:${hex(mac)}`;
}
