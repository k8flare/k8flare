import assert from "node:assert/strict";
import { test } from "node:test";
import { consume } from "../src/queues.ts";

function batchOf(queue: string, bodies: unknown[]) {
  const batch = {
    queue,
    messages: bodies.map((body) => ({ body })),
    acked: false,
    ackAll: () => void (batch.acked = true),
  };
  return batch;
}

function envWith(sends: unknown[], cluster: { path: string; body: any }[], asked: any[] = []) {
  const refuse = { send: async () => assert.fail("a follow-up went to a queue with delaySeconds instead of the Cluster deadline table") };
  return {
    WORKLOADS: {
      sync: async () => ({ objects: { jobs: 1 }, drained: true, nextMs: 4_000 }),
      nodeHealth: async () => ({ evicted: 0, waiting: 0, nextMs: 30_000 }),
    },
    __apiserver: async (request: Request) => {
      const url = new URL(request.url);
      if (url.pathname === "/internal/queue/plan") return Response.json({ resources: ["jobs"], serviceKeys: [], gatewayKeys: [] });
      if (url.pathname === "/internal/queue/followup") {
        asked.push(await request.json());
        return Response.json({ sends, stop: false });
      }
      return new Response("{}");
    },
    WL_Q: refuse,
    CTRL_Q: refuse,
    CLUSTER: {
      idFromName: (name: string) => name,
      get: () => ({
        fetch: async (url: string, init?: RequestInit) => {
          cluster.push({ path: new URL(url).pathname, body: JSON.parse(String(init?.body ?? "{}")) });
          return Response.json({ ok: true });
        },
      }),
    },
  };
}

test("a workloads follow-up is booked on the Cluster object with the resources it re-syncs", async () => {
  const cluster: { path: string; body: any }[] = [];
  const env = envWith([{ queue: "wl", delaySeconds: 4, kind: "retry", changed: ["jobs"] }], cluster);
  const batch = batchOf("k8flare-workloads", [{ kind: "change", key: "/registry/jobs/default/j", type: "created", rev: 2 }]);
  await consume(batch as any, env as any);
  assert.equal(batch.acked, true);
  assert.deepEqual(cluster.filter((c) => c.path === "/enqueue"), [{ path: "/enqueue", body: { target: "workloads", delayMs: 4_000, changed: ["jobs"] } }]);
});

test("a lease check follow-up is booked on the Cluster object for its node", async () => {
  const cluster: { path: string; body: any }[] = [];
  const env = envWith([{ queue: "ctrl", delaySeconds: 30, kind: "lease-check", node: "n1" }], cluster);
  const batch = batchOf("k8flare-controllers", [{ kind: "lease-check", node: "n1" }]);
  await consume(batch as any, env as any);
  assert.equal(batch.acked, true);
  assert.deepEqual(cluster, [{ path: "/lease-check", body: { node: "n1", delayMs: 30_000 } }]);
});

test("a failed retry's count is read from the batch and booked again with the follow-up's count", async () => {
  const cluster: { path: string; body: any }[] = [];
  const asked: any[] = [];
  const env = envWith([{ queue: "wl", delaySeconds: 40, kind: "retry", changed: ["jobs"], failures: 4 }], cluster, asked);
  const batch = batchOf("k8flare-workloads", [
    { kind: "retry", changed: ["jobs"], failures: 3 },
    { kind: "retry", changed: ["jobs"], failures: 1 },
    { kind: "change", key: "/registry/jobs/default/j", type: "created", rev: 2 },
  ]);
  await consume(batch as any, env as any);
  assert.equal(asked[0].failures, 3);
  assert.deepEqual(cluster.filter((c) => c.path === "/enqueue"), [{ path: "/enqueue", body: { target: "workloads", delayMs: 40_000, changed: ["jobs"], failures: 4 } }]);
});

test("a lease check's failure count is read from its own message and booked again for its node", async () => {
  const cluster: { path: string; body: any }[] = [];
  const asked: any[] = [];
  const env = envWith([{ queue: "ctrl", delaySeconds: 10, kind: "lease-check", node: "n1", failures: 2 }], cluster, asked);
  const batch = batchOf("k8flare-controllers", [{ kind: "lease-check", node: "n1", failures: 1 }]);
  await consume(batch as any, env as any);
  assert.equal(asked[0].failures, 1);
  assert.deepEqual(cluster, [{ path: "/lease-check", body: { node: "n1", delayMs: 10_000, failures: 2 } }]);
});

test("a node scheduler reconcile that answers with an error status is followed up as a failure", async () => {
  const asked: any[] = [];
  const env = {
    ...envWith([], [], asked),
    NODE_SCHED: {
      idFromName: (name: string) => name,
      get: () => ({ fetch: async () => new Response("unavailable", { status: 503 }) }),
    },
  };
  const batch = batchOf("k8flare-containers", [{ kind: "retry", failures: 2 }]);
  await consume(batch as any, env as any);
  assert.equal(batch.acked, true);
  assert.deepEqual(asked, [{ target: "containers", ok: false, nextMs: 0, failures: 2 }]);
});
