import assert from "node:assert/strict";
import { test } from "node:test";
import { consume } from "../src/queues.ts";

const PAGE_SIZE = 2;

class PagedBucket {
  objects = new Map<string, string>();
  lists = 0;
  async list({ cursor }: { cursor?: string } = {}) {
    this.lists++;
    const keys = [...this.objects.keys()].sort();
    const start = cursor ? Number(cursor) : 0;
    const end = start + PAGE_SIZE;
    const truncated = end < keys.length;
    return { objects: keys.slice(start, end).map((key) => ({ key })), truncated, cursor: truncated ? String(end) : undefined };
  }
  async get(key: string) {
    const body = this.objects.get(key);
    return body === undefined ? null : { text: async () => body };
  }
}

interface Call {
  worker: string;
  path: string;
  body: string;
}

function rig(manifests: Record<string, string>, helmBody = "{}", follow: Record<string, unknown> = { sends: [], stop: false }) {
  const bucket = new PagedBucket();
  for (const [key, body] of Object.entries(manifests)) bucket.objects.set(key, body);
  const calls: Call[] = [];
  const internal: string[] = [];
  const followUps: Record<string, unknown>[] = [];
  const cluster: { path: string; body: string }[] = [];
  const env = {
    DISABLE: "coredns",
    MANIFESTS_R2: bucket,
    LOADER: {
      load: (worker: string, workerEnv: Record<string, unknown>) => ({
        fetch: async (url: string, init?: RequestInit) => {
          calls.push({ worker: `${worker} disable=${workerEnv.DISABLE}`, path: new URL(url).pathname, body: String(init?.body ?? "") });
          return new Response(new URL(url).pathname === "/helm" ? helmBody : "{}");
        },
      }),
    },
    CLUSTER: {
      idFromName: (name: string) => name,
      get: () => ({
        fetch: async (url: string, init?: RequestInit) => {
          cluster.push({ path: new URL(url).pathname, body: String(init?.body ?? "") });
          return new Response("{}");
        },
      }),
    },
    __apiserver: async (request: Request) => {
      const path = new URL(request.url).pathname;
      internal.push(path);
      if (path === "/internal/storage-migrate") return Response.json({ done: true, rewritten: 0, pending: [] });
      if (path === "/internal/queue/followup") {
        followUps.push(await request.json());
        return Response.json(follow);
      }
      return Response.json({ sends: [], stop: false });
    },
  };
  return { bucket, calls, internal, followUps, cluster, env: env as unknown as Env };
}

function batchOf(bodies: unknown[]) {
  const batch = { queue: "k8flare-addons", messages: bodies.map((body) => ({ body })), acked: false, ackAll: () => void (batch.acked = true) };
  return batch;
}

const bucketChange = {
  account: "3f4b7e3dcab231cbfdaa90a6a28bd548",
  action: "PutObject",
  bucket: "k8flare-manifests",
  object: { key: "team/app.yaml", size: 12, eTag: "c846ff7a18f28c2e262116d6e8719ef0" },
  eventTime: "2026-10-01T00:00:00.000Z",
};

test("a change in the manifests bucket runs an addons pass with every manifest in the bucket", async () => {
  const manifests = { "a.yaml": "a", "b.yaml": "b", "c.yaml.skip": "", "team/app.yaml": "app", "team/db.json": "{}" };
  const { bucket, calls, env } = rig(manifests);
  const batch = batchOf([bucketChange]);

  await consume(batch as any, env);

  assert.deepEqual(calls.map((c) => `${c.worker} ${c.path}`), ["addons disable=coredns /deploy", "addons disable=coredns /helm"]);
  assert.deepEqual(JSON.parse(calls[0].body), manifests);
  assert.equal(bucket.lists, 3);
  assert.equal(batch.acked, true);
});

test("an object deleted from the manifests bucket runs an addons pass without it", async () => {
  const { bucket, calls, env } = rig({ "a.yaml": "a", "a.yaml.skip": "" });
  bucket.objects.delete("a.yaml.skip");
  const { size, eTag, ...object } = bucketChange.object;
  const batch = batchOf([{ ...bucketChange, action: "DeleteObject", object: { ...object, key: "a.yaml.skip" } }]);

  await consume(batch as any, env);

  assert.deepEqual(JSON.parse(calls[0].body), { "a.yaml": "a" });
  assert.equal(batch.acked, true);
});

test("a bucket change in a batch with a storage-migrate retry still deploys", async () => {
  const { calls, env } = rig({ "a.yaml": "a" });

  await consume(batchOf([{ kind: "retry", names: ["storage-migrate"] }, bucketChange]) as any, env);

  assert.deepEqual(calls.map((c) => c.path), ["/deploy", "/helm"]);
});

test("a storage-migrate retry alone reads nothing from the manifests bucket", async () => {
  const { bucket, calls, internal, env } = rig({ "a.yaml": "a" });

  await consume(batchOf([{ kind: "retry", names: ["storage-migrate"] }]) as any, env);

  assert.deepEqual(calls, []);
  assert.equal(bucket.lists, 0);
  assert.deepEqual(internal, ["/internal/storage-migrate", "/internal/queue/followup"]);
});

test("a helm pass waiting on a hook hands its deadline to the follow-up", async () => {
  const { followUps, env } = rig({}, JSON.stringify({ nextMs: 299_001 }));

  await consume(batchOf([{ kind: "retry" }]) as any, env);

  assert.equal(followUps.length, 1);
  assert.equal(followUps[0].target, "addons");
  assert.equal(followUps[0].ok, true);
  assert.equal(followUps[0].nextMs, 299_001);
});

test("a helm pass the follow-up finds no hook pending in clears the addons deadline on the Cluster object", async () => {
  const { followUps, cluster, env } = rig({}, "{}", { sends: [], cancel: ["addons"], stop: false });

  await consume(batchOf([{ kind: "retry" }]) as any, env);

  assert.equal(followUps[0].skip, false);
  assert.deepEqual(cluster.filter((c) => c.path === "/cancel"), [{ path: "/cancel", body: JSON.stringify({ target: "addons" }) }]);
});

test("a storage-migrate pass tells the follow-up it skipped helm", async () => {
  const { followUps, env } = rig({});

  await consume(batchOf([{ kind: "retry", names: ["storage-migrate"] }]) as any, env);

  assert.equal(followUps[0].skip, true);
});
