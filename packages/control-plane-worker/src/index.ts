import { apiserverFetch } from "./loader.ts";

export { Cluster } from "@k8flare/cluster-store";
export { NodeTunnel } from "@k8flare/node-tunnel";
export { Printers } from "./printers.ts";
export { OpenAPI } from "./openapi.ts";
export { CustomResources } from "./customresources.ts";
export { APIGroups } from "./apigroups.ts";
export { Scheduler } from "./scheduler.ts";
export { Controllers } from "./controllers.ts";
export { NodeTunnels } from "./nodetunnel.ts";
async function acceptTunnel(request: Request, env: Env): Promise<Response> {
  if (request.headers.get("Upgrade") !== "websocket") {
    return new Response("websocket upgrade required", { status: 426 });
  }
  const nodeName = await authenticateNode(request, env);
  if (!nodeName) return new Response("not authorized", { status: 401 });
  const stub = env.NODE_TUNNEL.get(env.NODE_TUNNEL.idFromName(nodeName));
  const headers = new Headers(request.headers);
  headers.set("X-K8flare-Node", nodeName);
  return stub.fetch(new Request(request.url, { method: request.method, headers }));
}

async function authenticateNode(request: Request, env: Env): Promise<string | null> {
  const token = request.headers.get("Authorization")?.match(/^Bearer (.+)$/)?.[1] ?? "";
  const match = token.match(/^node:([^:]+):(.+)$/);
  if (!match) return null;
  const [, nodeName, password] = match;
  if (!(await checkNodePassword(env, nodeName, password))) return null;
  return nodeName;
}

async function checkNodePassword(env: Env, nodeName: string, password: string): Promise<boolean> {
  const stub = env.CLUSTER.get(env.CLUSTER.idFromName("default"));
  const resp = await stub.fetch(`https://cluster.internal/kv?key=${encodeURIComponent("/vault/node/" + nodeName)}`);
  const data = (await resp.json()) as { kv: { value: string } | null };
  if (!data.kv) return false;
  const stored = atob(data.kv.value);
  const want = await sha256Hex(password);
  return stored.length === want.length && stored === want;
}

async function sha256Hex(s: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(s));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

// The periodic wake exists for work no write announces: a node going
// silent, a CronJob coming due, a workqueue retry. None of it can happen
// without a node, so an empty cluster dispatches nothing and costs one
// Durable Object read a minute instead of two resident workers.
async function clusterHasNodes(env: Env): Promise<boolean> {
  const store = env.CLUSTER.get(env.CLUSTER.idFromName("default"));
  const resp = await store.fetch("http://cluster.internal/list?prefix=%2Fregistry%2Fnodes%2F&limit=1");
  if (!resp.ok) return true;
  const body = (await resp.json()) as { kvs?: unknown[] };
  return (body.kvs ?? []).length > 0;
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const path = new URL(request.url).pathname;
    if (path === "/v1-k3s/connect") return acceptTunnel(request, env);
    if (path === "/stats" && request.headers.get("Authorization") === `Bearer ${env.ADMIN_TOKEN}`) {
      return env.CLUSTER.get(env.CLUSTER.idFromName("default")).fetch("https://cluster.internal/stats");
    }
    return apiserverFetch(env, request);
  },
  tail(events: TraceItem[]): void {
    for (const e of events) {
      if (e.event && "consumedEvents" in e.event) continue;
      console.log(`wasmcpu ${e.scriptName ?? "?"} ${e.entrypoint ?? "-"} ${e.outcome} ${e.cpuTime} ${e.wallTime}`);
    }
  },
  async scheduled(_controller: ScheduledController, env: Env): Promise<void> {
    if (!(await clusterHasNodes(env))) return;
    await Promise.all([
      env.SCHEDULER.run().catch((err) => console.error("scheduled scheduler run:", err)),
      env.CONTROLLERS.run().catch((err) => console.error("scheduled controllers run:", err)),
    ]);
  },
} satisfies ExportedHandler<Env>;
