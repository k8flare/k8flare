import assert from "node:assert/strict";
import { test } from "node:test";
import { rig } from "./harness.ts";

const start = 1_800_000_000_000;

function clock(t: any) {
  t.mock.timers.enable({ apis: ["Date"], now: start });
  return (ms: number) => t.mock.timers.tick(ms);
}

const decode = (b64: string) => Buffer.from(b64, "base64").toString();

test("on-demand save writes one object to R2 containing stored registry keys and returns its key", async () => {
  const r = rig();
  await r.settle();
  await r.put("/registry/pods/default/nginx", "pod-data");
  await r.put("/registry/services/specs/default/web", "svc-data");

  const res = await r.post("/snapshot");
  assert.equal(res.status, 200);
  const data = await res.json();

  assert.equal(data.ok, true);
  assert.ok(data.key.startsWith("clusters/test/snapshots/"));
  assert.ok(!data.key.includes("scheduled-"));
  assert.ok(data.key.endsWith(".json"));
  assert.equal(data.revision, (await r.get("/stats")).revision);
  assert.equal(data.count, 3);

  const obj = r.bucket.objects.get(data.key);
  assert.ok(obj);
  const snap = JSON.parse(obj.body);
  assert.equal(snap.schemaVersion, 1);
  assert.equal(snap.cluster, "test");
  assert.equal(snap.revision, data.revision);

  const keys = new Map(snap.keys.map((k: any) => [k.key, decode(k.value)]));
  assert.equal(keys.get("/registry/pods/default/nginx"), "pod-data");
  assert.equal(keys.get("/registry/services/specs/default/web"), "svc-data");
});

test("snapshot excludes key material under vault prefix from the object and payload", async () => {
  const r = rig();
  await r.settle();
  await r.put("/registry/pods/default/app", "app-payload");
  await r.put("/vault/server-ca", "ca-private-key-material");
  await r.put("/vault/sa-signing-key", "sa-secret-key-material");

  const res = await r.post("/snapshot");
  const data = await res.json();
  const obj = r.bucket.objects.get(data.key);
  assert.ok(obj);

  const snap = JSON.parse(obj.body);
  const storedKeys = snap.keys.map((k: any) => k.key);
  assert.ok(!storedKeys.includes("/vault/server-ca"));
  assert.ok(!storedKeys.includes("/vault/sa-signing-key"));
  assert.ok(!storedKeys.some((k: string) => k.startsWith("/vault/")));
  assert.ok(!obj.body.includes("/vault/"));
  assert.ok(!obj.body.includes(Buffer.from("ca-private-key-material").toString("base64")));
  assert.ok(!obj.body.includes(Buffer.from("sa-secret-key-material").toString("base64")));
  assert.ok(storedKeys.includes("/registry/pods/default/app"));
});

test("list returns saved snapshots in key order with sizes and upload timestamps", async (t) => {
  const tick = clock(t);
  const r = rig();
  await r.settle();

  const save1 = await (await r.post("/snapshot")).json();
  tick(2000);
  const save2 = await (await r.post("/snapshot")).json();

  const list = await r.get("/snapshots");
  assert.equal(list.items.length, 2);
  assert.deepEqual(list.items.map((i: any) => i.key), [save1.key, save2.key]);
  assert.equal(list.items[0].key, save1.key);
  assert.ok(list.items[0].size > 0);
  assert.ok(!Number.isNaN(Date.parse(list.items[0].uploaded)));
});

test("restore into an empty store brings registry keys back with their values", async () => {
  const r1 = rig();
  await r1.settle();
  await r1.put("/registry/configmaps/default/cfg", "cfg-data");
  await r1.put("/registry/services/specs/default/svc", "svc-data");
  const save = await (await r1.post("/snapshot")).json();

  const r2 = rig({ PODS_R2: r1.bucket } as any);
  await r2.settle();
  assert.equal((await r2.get("/list?prefix=/registry/")).kvs.length, 0);

  const restoreRes = await r2.post("/snapshot/restore", { key: save.key });
  assert.equal(restoreRes.status, 200);
  const data = await restoreRes.json();
  assert.equal(data.ok, true);
  assert.equal(data.key, save.key);
  assert.equal(data.written, 2);
  assert.equal(data.removed, 0);

  const cm = await r2.get("/kv?key=/registry/configmaps/default/cfg");
  assert.equal(decode(cm.kv.value), "cfg-data");
  const svc = await r2.get("/kv?key=/registry/services/specs/default/svc");
  assert.equal(decode(svc.kv.value), "svc-data");
});

test("restore into a store that already holds data is refused without force and with force replaces registry contents", async () => {
  const r1 = rig();
  await r1.settle();
  await r1.put("/registry/pods/default/keep", "keep-val");
  await r1.put("/registry/pods/default/snap-only", "snap-val");
  const save = await (await r1.post("/snapshot")).json();

  const r2 = rig({ PODS_R2: r1.bucket } as any);
  await r2.settle();
  await r2.put("/registry/pods/default/keep", "keep-val");
  await r2.put("/registry/pods/default/stale", "stale-val");

  const refused = await r2.post("/snapshot/restore", { key: save.key });
  assert.equal(refused.status, 409);
  const refBody = await refused.json();
  assert.equal(refBody.error, "cluster is not empty; restoring replaces its contents, pass force to proceed");
  assert.equal(refBody.keys, 2);
  assert.ok((await r2.get("/kv?key=/registry/pods/default/stale")).kv !== null);
  assert.equal((await r2.get("/kv?key=/registry/pods/default/snap-only")).kv, null);

  const forced = await r2.post("/snapshot/restore", { key: save.key, force: true });
  assert.equal(forced.status, 200);
  const forcedBody = await forced.json();
  assert.equal(forcedBody.ok, true);
  assert.equal(forcedBody.removed, 1);
  assert.equal(forcedBody.written, 1);
  assert.equal((await r2.get("/kv?key=/registry/pods/default/stale")).kv, null);
  assert.equal(decode((await r2.get("/kv?key=/registry/pods/default/snap-only")).kv.value), "snap-val");
  assert.equal(decode((await r2.get("/kv?key=/registry/pods/default/keep")).kv.value), "keep-val");
});

test("restore never touches vault keys even during a forced restore", async () => {
  const r1 = rig();
  await r1.settle();
  await r1.put("/registry/pods/default/p1", "v1");
  const save = await (await r1.post("/snapshot")).json();

  const r2 = rig({ PODS_R2: r1.bucket } as any);
  await r2.settle();
  await r2.put("/registry/pods/default/p2", "v2");
  await r2.put("/vault/server-ca", "ca-secret");
  await r2.put("/vault/sa-signing-key", "sa-secret");

  const res = await r2.post("/snapshot/restore", { key: save.key, force: true });
  assert.equal(res.status, 200);

  const ca = await r2.get("/kv?key=/vault/server-ca");
  assert.equal(decode(ca.kv.value), "ca-secret");
  const sa = await r2.get("/kv?key=/vault/sa-signing-key");
  assert.equal(decode(sa.kv.value), "sa-secret");
});

test("secrets stored encrypted are preserved byte-for-byte through save and restore", async () => {
  const r1 = rig();
  await r1.settle();
  const encSecret = "k8s:enc:aesgcm:v1:key1:arbitrary-binary-payload-data";
  await r1.put("/registry/secrets/default/app-secret", encSecret);
  const save = await (await r1.post("/snapshot")).json();

  const obj = r1.bucket.objects.get(save.key);
  assert.ok(obj);
  const snap = JSON.parse(obj.body);
  const stored = snap.keys.find((k: any) => k.key === "/registry/secrets/default/app-secret");
  assert.equal(decode(stored.value), encSecret);

  const r2 = rig({ PODS_R2: r1.bucket } as any);
  await r2.settle();
  await r2.post("/snapshot/restore", { key: save.key });

  const fetched = await r2.get("/kv?key=/registry/secrets/default/app-secret");
  assert.equal(decode(fetched.kv.value), encSecret);
});

test("restore in an existing store increments revision beyond current revision and emits events to watchers without altering compaction floor", async () => {
  const r1 = rig();
  await r1.settle();
  await r1.put("/registry/pods/default/a", "v1");
  const save = await (await r1.post("/snapshot")).json();

  const r2 = rig({ PODS_R2: r1.bucket } as any);
  await r2.settle();
  await r2.put("/registry/pods/default/old", "old-val");
  const beforeRev = (await r2.get("/stats")).revision;
  r2.sql.exec("INSERT INTO meta (key, value) VALUES ('compact_revision', 1) ON CONFLICT(key) DO UPDATE SET value = excluded.value");

  const ws = r2.watch({ prefix: "/registry/pods/", since: String(beforeRev) });
  await r2.settle();

  const res = await r2.post("/snapshot/restore", { key: save.key, force: true });
  assert.equal(res.status, 200);
  const data = await res.json();
  const afterRev = (await r2.get("/stats")).revision;

  assert.equal(data.revision, afterRev);
  assert.ok(afterRev > beforeRev);
  assert.equal((await r2.get("/stats")).compactRevision, 1);
  assert.deepEqual(
    ws.sent.map((e) => [e.type, e.key]),
    [
      ["deleted", "/registry/pods/default/old"],
      ["created", "/registry/pods/default/a"],
    ],
  );
});

test("restore into a fresh store assigns revisions from local sequence which can be lower than the snapshot revision", async () => {
  const r1 = rig();
  await r1.settle();
  for (let i = 0; i < 20; i++) {
    await r1.put(`/registry/pods/default/p${i}`, `v${i}`);
  }
  for (let i = 0; i < 18; i++) {
    await r1.remove(`/registry/pods/default/p${i}`);
  }
  const save = await (await r1.post("/snapshot")).json();
  assert.ok(save.revision >= 39);

  const r2 = rig({ PODS_R2: r1.bucket } as any);
  await r2.settle();
  assert.equal((await r2.get("/stats")).revision, 1);

  const res = await r2.post("/snapshot/restore", { key: save.key });
  assert.equal(res.status, 200);
  const data = await res.json();
  assert.equal(data.written, 2);
  assert.equal(data.revision, 3);
  assert.ok(data.revision < save.revision);
  assert.equal((await r2.get("/stats")).compactRevision, 0);
});

test("restore rejects malformed keys, missing objects, and mismatched schema versions", async () => {
  const r = rig();
  await r.settle();

  const badKey = await r.post("/snapshot/restore", { key: "invalid-key" });
  assert.equal(badKey.status, 400);

  const notFound = await r.post("/snapshot/restore", { key: "clusters/test/snapshots/missing.json" });
  assert.equal(notFound.status, 404);

  await r.bucket.put("clusters/test/snapshots/v99.json", JSON.stringify({ schemaVersion: 99, keys: [] }));
  const mismatch = await r.post("/snapshot/restore", { key: "clusters/test/snapshots/v99.json" });
  assert.equal(mismatch.status, 409);
});
