import assert from "node:assert/strict";
import { test } from "node:test";
import { rig } from "./harness.ts";

const MINUTE = 60_000;
const start = 1_800_000_000_000;

function clock(t: any) {
  t.mock.timers.enable({ apis: ["Date"], now: start });
  return (ms: number) => t.mock.timers.tick(ms);
}

const decode = (b64: string) => Buffer.from(b64, "base64").toString();

test("history is kept for the retention window however many revisions it spans", async (t) => {
  const tick = clock(t);
  const r = rig();
  await r.settle();
  for (let i = 0; i < 1500; i++) await r.put("/registry/pods/default/a", `v${i}`, i === 0 ? 0 : i + 1);
  tick(4 * MINUTE);
  await r.fire();
  assert.equal((await r.get("/stats")).compactRevision, 0);
  assert.equal((await r.get("/list?prefix=/registry/&revision=2")).kvs.length, 1);
});

test("compaction after the window keeps the newest version at or below the target", async (t) => {
  const tick = clock(t);
  const r = rig();
  await r.settle();
  await r.put("/registry/pods/default/x", "v1");
  tick(6 * MINUTE);
  await r.put("/registry/pods/default/y", "y1");
  await r.put("/registry/pods/default/x", "v2", 2);
  await r.fire();
  const stats = await r.get("/stats");
  assert.equal(stats.compactRevision, 2);
  const at3 = await r.get("/list?prefix=/registry/&revision=3");
  assert.deepEqual(at3.kvs.map((kv: any) => [kv.key, decode(kv.value)]), [
    ["/registry/pods/default/x", "v1"],
    ["/registry/pods/default/y", "y1"],
  ]);
  assert.equal((await r.cluster.fetch(new Request("http://cluster.internal/list?prefix=/registry/&revision=1"))).status, 410);
});

test("the size bound compacts even inside the window", async (t) => {
  clock(t);
  const r = rig();
  await r.settle();
  for (let i = 0; i < 120_000; i++) r.sql.exec("INSERT INTO kine (name, deleted, value, ts) VALUES ('/registry/a', 0, X'00', ?)", Date.now());
  await r.put("/registry/pods/default/a", "v");
  assert.ok((await r.get("/stats")).compactRevision > 0);
});

test("replay reports modified when the predecessor survived compaction", async (t) => {
  const tick = clock(t);
  const r = rig();
  await r.settle();
  await r.put("/registry/pods/default/x", "v1");
  tick(6 * MINUTE);
  await r.put("/registry/pods/default/x", "v2", 2);
  await r.fire();
  assert.equal((await r.get("/stats")).compactRevision, 2);
  const ws = r.watch({ prefix: "/registry/pods/", since: "2" });
  assert.deepEqual(ws.sent.map((e) => [e.type, e.rev, decode(e.prev)]), [["modified", 3, "v1"]]);
});

test("replay reports created for a key recreated after a delete", async (t) => {
  clock(t);
  const r = rig();
  await r.settle();
  await r.put("/registry/pods/default/x", "v1");
  await r.remove("/registry/pods/default/x");
  await r.put("/registry/pods/default/x", "v2");
  const ws = r.watch({ prefix: "/registry/pods/", since: "1" });
  assert.deepEqual(ws.sent.map((e) => [e.type, e.rev, e.prev]), [
    ["created", 2, ""],
    ["deleted", 3, ""],
    ["created", 4, ""],
  ]);
});

test("open watches get progress on a timer and on request", async (t) => {
  const tick = clock(t);
  const r = rig();
  await r.settle();
  await r.put("/registry/pods/default/x", "v1");
  const ws = r.watch({ prefix: "/registry/pods/", since: "2" });
  await r.settle();
  assert.deepEqual(ws.sent, []);
  tick(31_000);
  await r.fire();
  assert.deepEqual(ws.sent.map((e) => [e.type, e.rev]), [["progress", 2]]);
  await r.post("/progress");
  assert.equal(ws.sent.length, 2);
  assert.ok(r.alarm.at !== null && r.alarm.at > Date.now());
});

test("the alarm always books the earliest deadline among leases, progress, compaction and snapshots", async (t) => {
  clock(t);
  const r = rig();
  await r.settle();
  await r.put("/registry/pods/default/a", "v1");
  await r.put("/registry/pods/default/a", "v2", 2);
  assert.equal(r.alarm.at, start + 5 * MINUTE);
  r.watch({ prefix: "/registry/pods/", since: "1" });
  await r.settle();
  assert.equal(r.alarm.at, start + 30_000);
});

test("scheduled snapshots go to R2 on the interval and only scheduled ones are pruned", async (t) => {
  const tick = clock(t);
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "12", SNAPSHOT_RETENTION: "2" });
  await r.settle();
  await r.put("/registry/pods/default/x", "v1");
  await r.post("/snapshot");
  for (let i = 0; i < 4; i++) {
    tick(12 * 3600_000 + 1000);
    await r.fire();
  }
  const keys = [...r.bucket.objects.keys()];
  const scheduled = keys.filter((k) => k.includes("/scheduled-"));
  assert.equal(scheduled.length, 2);
  assert.equal(keys.length, 3);
  assert.ok(scheduled.every((k) => k.startsWith("clusters/test/snapshots/")));
  assert.ok(new Set(scheduled).size === 2);
});

test("snapshots are not taken before the interval and can be disabled", async (t) => {
  const tick = clock(t);
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "12" });
  await r.settle();
  tick(11 * 3600_000);
  await r.fire();
  assert.equal(r.bucket.objects.size, 0);
  const off = rig({ SNAPSHOT_INTERVAL_HOURS: "0" });
  await off.settle();
  tick(30 * 3600_000);
  await off.fire();
  assert.equal(off.bucket.objects.size, 0);
});

test("VolumeAttributesClass writes reach the workloads queue so its protection finalizer is released", async () => {
  const sent: string[] = [];
  const workloads = { send: async () => {}, sendBatch: async (batch: { body: { key: string } }[]) => void sent.push(...batch.map((m) => m.body.key)) };
  const r = rig({ WL_Q: workloads } as any);
  await r.settle();
  await r.put("/registry/volumeattributesclasses/e2e-1", "v1");
  await r.put("/registry/volumeattributesclasses/e2e-1", "v2", 2);
  await r.settle();
  assert.deepEqual(sent, ["/registry/volumeattributesclasses/e2e-1", "/registry/volumeattributesclasses/e2e-1"]);
});

test("ServiceCIDR, ValidatingAdmissionPolicy, ResourceClaim and ResourceSlice writes reach the workloads queue", async () => {
  const sent: string[] = [];
  const workloads = { send: async () => {}, sendBatch: async (batch: { body: { key: string } }[]) => void sent.push(...batch.map((m) => m.body.key)) };
  const r = rig({ WL_Q: workloads } as any);
  await r.settle();
  await r.put("/registry/servicecidrs/extra", "v1");
  await r.put("/registry/validatingadmissionpolicies/deny-label", "v1");
  await r.put("/registry/resourceclaims/default/gpu", "v1");
  await r.put("/registry/resourceslices/node-a-gpu", "v1");
  await r.remove("/registry/servicecidrs/extra");
  await r.settle();
  assert.deepEqual(sent, [
    "/registry/servicecidrs/extra",
    "/registry/validatingadmissionpolicies/deny-label",
    "/registry/resourceclaims/default/gpu",
    "/registry/resourceslices/node-a-gpu",
    "/registry/servicecidrs/extra",
  ]);
});

test("IPAddress and DeviceClass writes do not start a workloads pass", async () => {
  const sent: string[] = [];
  const workloads = { send: async () => {}, sendBatch: async (batch: { body: { key: string } }[]) => void sent.push(...batch.map((m) => m.body.key)) };
  const r = rig({ WL_Q: workloads } as any);
  await r.settle();
  await r.put("/registry/ipaddresses/10.43.0.5", "v1");
  await r.put("/registry/deviceclasses/gpu", "v1");
  await r.remove("/registry/ipaddresses/10.43.0.5");
  await r.settle();
  assert.deepEqual(sent, []);
});

test("HelmChart and HelmChartConfig writes of every kind reach the addons queue", async () => {
  const sent: string[] = [];
  const addons = { send: async () => {}, sendBatch: async (batch: { body: { key: string } }[]) => void sent.push(...batch.map((m) => m.body.key)) };
  const r = rig({ ADDON_Q: addons } as any);
  await r.settle();
  await r.put("/registry/helm.cattle.io/helmcharts/kube-system/traefik", "v1");
  await r.put("/registry/helm.cattle.io/helmcharts/kube-system/traefik", "v2", 2);
  await r.put("/registry/helm.cattle.io/helmchartconfigs/kube-system/traefik", "v1");
  await r.remove("/registry/helm.cattle.io/helmcharts/kube-system/traefik");
  await r.put("/registry/pods/default/a", "v1");
  await r.settle();
  assert.deepEqual(sent, [
    "/registry/helm.cattle.io/helmcharts/kube-system/traefik",
    "/registry/helm.cattle.io/helmcharts/kube-system/traefik",
    "/registry/helm.cattle.io/helmchartconfigs/kube-system/traefik",
    "/registry/helm.cattle.io/helmcharts/kube-system/traefik",
  ]);
});

function targetQueue() {
  const sent: string[] = [];
  const queue = {
    send: async (body: { kind: string }) => void sent.push(body.kind),
    sendBatch: async (batch: { body: { key: string } }[]) => void sent.push(...batch.map((m) => m.body.key)),
  };
  return { sent, queue };
}

test("HorizontalPodAutoscaler status writes do not reach hpa queue, spec changes and deletes do", async () => {
  const { sent: hpaSent, queue: hpaQueue } = targetQueue();
  const r = rig({ HPA_Q: hpaQueue } as any);
  await r.settle();
  const hpa = (spec: string, status: string) => `{"metadata":{"name":"h"},"spec":${spec},"status":${status}}`;
  await r.put("/registry/horizontalpodautoscalers/default/h", hpa(`{"minReplicas":1}`, `{"currentReplicas":1}`));
  await r.settle();
  assert.deepEqual(hpaSent, ["/registry/horizontalpodautoscalers/default/h"]);

  await r.put("/registry/horizontalpodautoscalers/default/h", hpa(`{"minReplicas":1}`, `{"currentReplicas":2}`), 2);
  await r.settle();
  assert.equal(hpaSent.length, 1);

  await r.put("/registry/horizontalpodautoscalers/default/h", hpa(`{"minReplicas":2}`, `{"currentReplicas":2}`), 3);
  await r.settle();
  assert.equal(hpaSent.length, 2);

  await r.remove("/registry/horizontalpodautoscalers/default/h");
  await r.settle();
  assert.equal(hpaSent.length, 3);
});

test("POST /enqueue with once deduplicates within delay and reschedules after due", async () => {
  const sent: { delaySeconds?: number }[] = [];
  const queue = {
    send: async (_body: unknown, opts?: { delaySeconds?: number }) => void sent.push({ delaySeconds: opts?.delaySeconds }),
    sendBatch: async () => {},
  };
  const r = rig({ HPA_Q: queue } as any);
  await r.settle();

  const res1 = await (await r.post("/enqueue", { target: "hpa", delayMs: 15_000, once: true })).json();
  assert.deepEqual(res1, { ok: true, booked: true });
  assert.equal(sent.length, 1);
  assert.equal(sent[0].delaySeconds, 15);

  const res2 = await (await r.post("/enqueue", { target: "hpa", delayMs: 15_000, once: true })).json();
  assert.deepEqual(res2, { ok: true, booked: false });
  assert.equal(sent.length, 1);

  r.sql.exec("UPDATE deadlines SET due = ? WHERE target = 'hpa'", Date.now() - 1000);

  const res3 = await (await r.post("/enqueue", { target: "hpa", delayMs: 15_000, once: true })).json();
  assert.deepEqual(res3, { ok: true, booked: true });
  assert.equal(sent.length, 2);

  const resPlain1 = await (await r.post("/enqueue", { target: "hpa", delayMs: 5_000 })).json();
  const resPlain2 = await (await r.post("/enqueue", { target: "hpa", delayMs: 5_000 })).json();
  assert.deepEqual(resPlain1, { ok: true });
  assert.deepEqual(resPlain2, { ok: true });
  assert.equal(sent.length, 4);
});

test("the garbage collector hears about ownership and deletion, not every status write of an owned object", async () => {
  const sent: string[] = [];
  const gc = { send: async () => {}, sendBatch: async (batch: { body: { key: string; type: string } }[]) => void sent.push(...batch.map((m) => `${m.body.type} ${m.body.key}`)) };
  const r = rig({ GC_Q: gc } as any);
  await r.settle();
  const owned = (status: string, owners = '[{"uid":"rs-1"}]') => `{"metadata":{"ownerReferences":${owners}},"status":{"phase":"${status}"}}`;
  await r.put("/registry/pods/default/a", owned("Pending"));
  await r.put("/registry/pods/default/a", owned("Running"), 2);
  await r.put("/registry/pods/default/a", owned("Running", '[{"uid":"rs-2"}]'), 3);
  await r.put("/registry/pods/default/b", `{"metadata":{}}`);
  await r.put("/registry/pods/default/b", `{"metadata":{"deletionTimestamp":"2026-01-01T00:00:00Z","finalizers":["foregroundDeletion"]}}`, 5);
  await r.remove("/registry/pods/default/a");
  await r.settle();
  assert.deepEqual(sent, [
    "created /registry/pods/default/a",
    "modified /registry/pods/default/a",
    "modified /registry/pods/default/b",
    "deleted /registry/pods/default/a",
  ]);
});

test("a node keeps exactly one pending lease check: lease writes and follow-ups only schedule once the pending one is due", async (t) => {
  const tick = clock(t);
  const sent: { node: string; delaySeconds?: number }[] = [];
  const ctrl = { send: async (body: { node: string }, opts?: { delaySeconds?: number }) => void sent.push({ node: body.node, delaySeconds: opts?.delaySeconds }), sendBatch: async () => {} };
  const r = rig({ CTRL_Q: ctrl } as any);
  await r.settle();
  await r.put("/registry/leases/kube-node-lease/n1", "v1");
  await r.settle();
  assert.deepEqual(sent, [{ node: "n1", delaySeconds: 60 }]);
  tick(10_000);
  await r.put("/registry/leases/kube-node-lease/n1", "v2", 2);
  await r.settle();
  assert.equal((await (await r.post("/lease-check", { node: "n1", delayMs: 30_000 })).json()).scheduled, false);
  assert.equal(sent.length, 1);
  tick(51_000);
  assert.equal((await (await r.post("/lease-check", { node: "n1", delayMs: 30_000 })).json()).scheduled, true);
  assert.deepEqual(sent[1], { node: "n1", delaySeconds: 30 });
  await r.put("/registry/leases/kube-node-lease/n1", "v3", 3);
  await r.settle();
  assert.equal(sent.length, 2);
  tick(51_000);
  await r.put("/registry/leases/kube-node-lease/n1", "v4", 4);
  await r.settle();
  assert.equal(sent.length, 3);
});

test("closing a watcher updates or clears alarm when nothing else is due", async (t) => {
  const tick = clock(t);
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0" });
  await r.settle();
  r.sql.exec("DELETE FROM meta WHERE key = 'compact_due'");
  const ws1 = r.watch({ prefix: "/registry/pods/", since: "1" });
  await r.settle();
  assert.equal(r.alarm.at, start + 30_000);
  await r.cluster.webSocketClose(ws1 as any);
  await r.settle();
  assert.equal(r.alarm.at, null);

  const wsA = r.watch({ prefix: "/registry/pods/", since: "1" });
  await r.settle();
  tick(10_000);
  const wsB = r.watch({ prefix: "/registry/pods/", since: "1" });
  await r.settle();
  tick(31_000);
  await r.fire();
  assert.equal(r.alarm.at, start + 41_000 + 30_000);
  (r.cluster as any).progressDue = start + 500_000;
  await (r.cluster as any).armAlarm();
  assert.equal(r.alarm.at, start + 360_000);
  await r.cluster.webSocketClose(wsA as any);
  await r.settle();
  assert.equal(r.alarm.at, start + 10_000 + 360_000);
});

test("compaction is due only when there is something to compact", async (t) => {
  const tick = clock(t);
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0" });
  await r.settle();
  assert.equal(r.alarm.at, null);

  await r.put("/registry/pods/default/a", "v1");
  await r.settle();
  assert.equal(r.alarm.at, null);

  await r.put("/registry/pods/default/a", "v2", 2);
  await r.settle();
  assert.equal(r.alarm.at, start + 5 * MINUTE);

  tick(5 * MINUTE + 1000);
  await r.fire();
  assert.equal(r.alarm.at, null);

  await r.put("/registry/pods/default/a", "v3", 3);
  await r.settle();
  assert.equal(r.alarm.at, start + 5 * MINUTE + 1000 + 5 * MINUTE);
});

test("constructing the object on an idle store sends nothing to any queue", async (t) => {
  const tick = clock(t);
  const r0 = rig();
  await r0.settle();
  tick(120_000);

  const sent: { queue: string; body: any }[] = [];
  const makeQueue = (name: string) => ({
    send: async (body: any) => void sent.push({ queue: name, body }),
    sendBatch: async (batch: any[]) => void sent.push(...batch.map((m) => ({ queue: name, body: m.body }))),
  });
  const queues = {
    CTRL_Q: makeQueue("ctrl"),
    SCHED_Q: makeQueue("sched"),
    WL_Q: makeQueue("wl"),
    CRD_Q: makeQueue("crd"),
    GC_Q: makeQueue("gc"),
    ACCT_Q: makeQueue("acct"),
    EXT_Q: makeQueue("ext"),
    CONTAINERS_Q: makeQueue("containers"),
    AD_Q: makeQueue("ad"),
    ADDON_Q: makeQueue("addon"),
    HPA_Q: makeQueue("hpa"),
  };

  const pending: Promise<unknown>[] = [];
  const ctx = {
    storage: {
      sql: r0.sql,
      getAlarm: async () => r0.alarm.at,
      setAlarm: async (at: number) => void (r0.alarm.at = at),
      deleteAlarm: async () => void (r0.alarm.at = null),
    },
    waitUntil: (p: Promise<unknown>) => void pending.push(p),
    blockConcurrencyWhile: (fn: () => Promise<void>) => fn(),
    acceptWebSocket: () => {},
    getWebSockets: () => [],
  };
  const env = { ...queues, PODS_R2: r0.bucket, CLUSTER_UID: "test" };
  new (r0.cluster.constructor as any)(ctx, env);
  while (pending.length) await pending.shift();

  assert.deepEqual(sent, []);
});

test("alarms carry every deadline, so nothing polls: an idle cluster after compaction books no alarm and sends no queue messages", async (t) => {
  const tick = clock(t);
  const sent: { queue: string; body: any }[] = [];
  const makeQueue = (name: string) => ({
    send: async (body: any) => void sent.push({ queue: name, body }),
    sendBatch: async (batch: any[]) => void sent.push(...batch.map((m) => ({ queue: name, body: m.body }))),
  });
  const queues = {
    CTRL_Q: makeQueue("ctrl"),
    SCHED_Q: makeQueue("sched"),
    WL_Q: makeQueue("wl"),
    CRD_Q: makeQueue("crd"),
    GC_Q: makeQueue("gc"),
    ACCT_Q: makeQueue("acct"),
    EXT_Q: makeQueue("ext"),
    CONTAINERS_Q: makeQueue("containers"),
    AD_Q: makeQueue("ad"),
    ADDON_Q: makeQueue("addon"),
    HPA_Q: makeQueue("hpa"),
  };
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0", ...queues });
  await r.settle();

  await r.put("/registry/pods/default/a", "v1");
  await r.put("/registry/pods/default/b", "v1");
  await r.put("/registry/pods/default/a", "v2", 2);
  await r.remove("/registry/pods/default/b");
  await r.settle();

  assert.equal(r.alarm.at, start + 5 * MINUTE);
  tick(5 * MINUTE + 1000);

  sent.length = 0;
  await r.fire();
  await r.settle();

  assert.equal(r.alarm.at, null);
  assert.deepEqual(sent, []);
  assert.equal((await r.get("/stats")).watchers, 0);
});




