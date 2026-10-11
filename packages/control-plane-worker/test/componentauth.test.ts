import assert from "node:assert/strict";
import { createHash, createHmac } from "node:crypto";
import { test } from "node:test";
import { componentToken } from "../src/componenttoken.ts";
import { consume } from "../src/queues.ts";
import { deleteNode, getNode, listPendingContainersPods, mergePodEnv, planNodeVMs } from "../src/nodes/client.ts";
import { mintLocal } from "../src/nodes/r2creds.ts";
import { Metrics } from "../src/metrics.ts";
import { APIGroups } from "../src/apigroups.ts";
import { NodeTunnel } from "../../node-tunnel/src/index.ts";

const COMPONENT_KEY_PATH = "/vault/component-key";
const ADMIN = "admin-test-token";

class FakeCluster {
  kv = new Map<string, string>();
  puts = 0;
  idFromName = (name: string) => name;
  get = () => ({
    fetch: async (url: string, init?: RequestInit) => {
      const u = new URL(url);
      if (u.pathname === "/kv" && (init?.method ?? "GET") === "GET") {
        const key = u.searchParams.get("key") ?? "";
        const value = this.kv.get(key);
        return Response.json({ revision: 1, kv: value === undefined ? null : { key, value: btoa(value), modRevision: 1 } });
      }
      if (u.pathname === "/kv" && init?.method === "PUT") {
        this.puts++;
        const body = JSON.parse(String(init.body)) as { key: string; value: string; revision: number };
        if (this.kv.has(body.key)) return Response.json({ revision: 1, error: "exists" }, { status: 409 });
        this.kv.set(body.key, atob(body.value));
        return Response.json({ revision: 2 }, { status: 201 });
      }
      return Response.json({ ok: true });
    },
  });
}

let clusters = 0;

function rig(responses: Record<string, unknown> = {}) {
  const cluster = new FakeCluster();
  cluster.kv.set(COMPONENT_KEY_PATH, "stored-component-key");
  const calls: { path: string; auth: string | null }[] = [];
  const env: Record<string, unknown> = {
    ADMIN_TOKEN: ADMIN,
    CLUSTER_UID: `component-auth-${++clusters}`,
    CLUSTER: cluster,
    __apiserver: async (request: Request) => {
      const path = new URL(request.url).pathname;
      calls.push({ path, auth: request.headers.get("Authorization") });
      return Response.json(responses[path] ?? { sends: [], stop: false });
    },
  };
  return { cluster, calls, env };
}

function expected(component: string, key = "stored-component-key"): string {
  return `Bearer component:${component}:${createHmac("sha256", key).update(`k8flare-component:${component}`).digest("hex")}`;
}

function batchOf(queue: string, bodies: unknown[]) {
  return { queue, messages: bodies.map((body) => ({ body })), ackAll: () => {} };
}

test("a component token is the HMAC of the component name keyed by the stored component key, not by ADMIN_TOKEN", async () => {
  const { cluster, env } = rig();
  cluster.kv.set(COMPONENT_KEY_PATH, "change-me");
  assert.equal(await componentToken(env as unknown as Env, "scheduler"), "component:scheduler:46f4359322a0658846a0540bcbd9a96a89fcbc914d6489f1597b275c40bbcfb2");
  const other = rig();
  other.cluster.kv.set(COMPONENT_KEY_PATH, "change-me");
  other.env.ADMIN_TOKEN = "a-different-admin-token";
  assert.equal(await componentToken(other.env as unknown as Env, "scheduler"), "component:scheduler:46f4359322a0658846a0540bcbd9a96a89fcbc914d6489f1597b275c40bbcfb2");
  assert.equal(cluster.puts + other.cluster.puts, 0);
});

test("the component key is created once in the Cluster Durable Object when it does not exist", async () => {
  const { cluster, env } = rig();
  cluster.kv.clear();
  const first = await componentToken(env as unknown as Env, "queues");
  const second = await componentToken(env as unknown as Env, "queues");
  const stored = cluster.kv.get(COMPONENT_KEY_PATH) ?? "";
  assert.match(stored, /^[0-9a-f]{64}$/);
  assert.equal(cluster.puts, 1);
  assert.equal(`Bearer ${first}`, expected("queues", stored));
  assert.equal(second, first);
});

test("a component key written by another isolate first is the one used", async () => {
  const { cluster, env } = rig();
  cluster.kv.clear();
  const realGet = cluster.get;
  cluster.get = () => {
    const stub = realGet();
    return {
      fetch: async (url: string, init?: RequestInit) => {
        if (init?.method === "PUT") cluster.kv.set(COMPONENT_KEY_PATH, "won-the-race");
        return stub.fetch(url, init);
      },
    };
  };
  assert.equal(`Bearer ${await componentToken(env as unknown as Env, "nodes")}`, expected("nodes", "won-the-race"));
});

test("queue consumers call the apiserver as system:k8flare:queues, and storage migration as addons", async () => {
  const { calls, env } = rig({
    "/internal/queue/plan": { resources: [], serviceKeys: ["default/svc"], gatewayKeys: ["default/gw"] },
    "/internal/storage-migrate": { done: true, rewritten: 0, pending: [] },
  });
  env.HPA = { sync: async () => null };
  await consume(batchOf("k8flare-workloads", [{ kind: "change", key: "/registry/services/default/svc" }]) as any, env as unknown as Env);
  await consume(batchOf("k8flare-hpa", [{ kind: "retry" }]) as any, env as unknown as Env);
  await consume(batchOf("k8flare-extensions", [{ kind: "retry" }]) as any, env as unknown as Env);
  await consume(batchOf("k8flare-addons", [{ kind: "retry", names: ["storage-migrate"] }]) as any, env as unknown as Env);
  const paths = new Set(calls.map((c) => c.path));
  for (const path of ["/internal/queue/plan", "/internal/loadbalancer/provision", "/internal/gateway/provision", "/internal/queue/followup", "/internal/metrics/scrape", "/internal/extensions/dispatch", "/internal/storage-migrate"]) {
    assert.ok(paths.has(path), `${path} was not called`);
  }
  for (const call of calls) {
    assert.equal(call.auth, expected(call.path === "/internal/storage-migrate" ? "addons" : "queues"), call.path);
  }
});

test("the nodes client and the R2 mint call the apiserver as system:k8flare:nodes", async () => {
  const { calls, env } = rig({
    "/api/v1/nodes/n1": { apiVersion: "v1", kind: "Node", metadata: { name: "n1" } },
    "/api/v1/pods": { items: [] },
    "/api/v1/namespaces/default/pods/web": { apiVersion: "v1", kind: "Pod", metadata: { name: "web", namespace: "default" } },
    "/internal/nodes/vm-plan": { boots: [], teardowns: [], markBound: [], hasWork: false },
    "/internal/pods/merge-env": { apiVersion: "v1", kind: "Pod", metadata: { name: "web", namespace: "default" } },
    "/internal/r2/mint": { accessKeyId: "a", secretAccessKey: "s", sessionToken: "t" },
  });
  Object.assign(env, { R2_ACCESS_KEY_ID: "id", R2_SECRET_ACCESS_KEY: "secret", R2_ACCOUNT_ID: "acct", R2_BUCKET: "bucket" });
  const e = env as unknown as Env;
  await getNode(e, "n1");
  await listPendingContainersPods(e);
  await planNodeVMs(e, {});
  await mergePodEnv(e, "default", "web", { A: "1" });
  await deleteNode(e, "n1");
  assert.deepEqual(await mintLocal(e as any, "clusters/x/"), { accessKeyId: "a", secretAccessKey: "s", sessionToken: "t" });
  assert.equal(calls.length, 8);
  for (const call of calls) assert.equal(call.auth, expected("nodes"), call.path);
});

test("the Metrics entrypoint does not add the admin token to a request without credentials", async () => {
  const { calls, env } = rig();
  const metrics = new (Metrics as any)({}, env);
  await metrics.fetch(new Request("https://k8flare.internal/apis/metrics.k8s.io/v1beta1/pods"));
  assert.deepEqual(calls, [{ path: "/apis/metrics.k8s.io/v1beta1/pods", auth: null }]);
});

test("group workers get the SHA-256 of the admin token, never the token itself", async () => {
  const loaded: Record<string, unknown>[] = [];
  const env: Record<string, unknown> = {
    ADMIN_TOKEN: ADMIN,
    LOADER: { load: (_name: string, workerEnv: Record<string, unknown>) => (loaded.push(workerEnv), { fetch: async () => new Response("ok") }) },
  };
  const request = () => new Request("https://apigroups.internal/apis/apps/v1", { headers: { "X-K8flare-Worker": "apiserver-apps" } });
  await new (APIGroups as any)({}, env).fetch(request());
  env.ADMIN_TOKEN = "";
  await new (APIGroups as any)({}, env).fetch(request());
  assert.equal("ADMIN_TOKEN" in loaded[0], false);
  assert.equal(loaded[0].ADMIN_TOKEN_SHA256, createHash("sha256").update(ADMIN).digest("hex"));
  assert.equal(loaded[1].ADMIN_TOKEN_SHA256, "");
});

test("the node tunnel fetches its client certificates as system:k8flare:nodetunnel and does not hand ADMIN_TOKEN to its Go program", async () => {
  const { calls, env } = rig({
    "/internal/kubelet-client": { cert: "c", key: "k", ca: "ca" },
    "/internal/proxy-client": { cert: "pc", key: "pk" },
  });
  const ctx = { getWebSockets: () => [], blockConcurrencyWhile: (fn: () => Promise<void>) => fn(), storage: { get: async () => undefined } };
  const tunnel = new NodeTunnel(ctx as unknown as DurableObjectState, env as unknown as Env);
  const resp = await tunnel.fetch(new Request("https://nodetunnel.internal/node/n1/stats/summary"));
  assert.equal(resp.status, 200);
  assert.deepEqual(calls.map((c) => c.path), ["/internal/kubelet-client", "/internal/proxy-client"]);
  for (const call of calls) assert.equal(call.auth, expected("nodetunnel"), call.path);
  const goEnv = (globalThis as unknown as { __goEnv: Record<string, unknown> }).__goEnv;
  assert.equal("ADMIN_TOKEN" in goEnv, false);
  assert.equal(goEnv.KUBELET_CLIENT_CERT, "c");
});
