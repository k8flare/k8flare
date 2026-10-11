import assert from "node:assert/strict";
import { test } from "node:test";
import { rig } from "./harness.ts";

const start = 1_800_000_000_000;

function clock(t: any) {
  t.mock.timers.enable({ apis: ["Date"], now: start });
  return (ms: number) => t.mock.timers.tick(ms);
}

type Sent = { queue: string; body: any; delaySeconds?: number };

function recordingQueues(sent: Sent[], failing = new Set<string>()) {
  const make = (name: string) => ({
    send: async (body: any, opts?: { delaySeconds?: number }) => {
      if (failing.has(name)) throw new Error(`${name} send failed`);
      sent.push({ queue: name, body, delaySeconds: opts?.delaySeconds });
    },
    sendBatch: async (batch: any[]) => void sent.push(...batch.map((m) => ({ queue: name, body: m.body }))),
  });
  return {
    CTRL_Q: make("ctrl"),
    SCHED_Q: make("sched"),
    WL_Q: make("wl"),
    CRD_Q: make("crd"),
    GC_Q: make("gc"),
    ACCT_Q: make("acct"),
    EXT_Q: make("ext"),
    CONTAINERS_Q: make("containers"),
    AD_Q: make("ad"),
    ADDON_Q: make("addon"),
    HPA_Q: make("hpa"),
  };
}

const booked = (r: ReturnType<typeof rig>) => r.sql.exec("SELECT target, due FROM deadlines ORDER BY due").toArray().map((row) => ({ ...row }));

test("a booked deadline sets the alarm and sends nothing until it is due", async (t) => {
  clock(t);
  const sent: Sent[] = [];
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0", ...recordingQueues(sent) } as any);
  await r.settle();
  sent.length = 0;
  assert.equal(r.alarm.at, null);

  await r.post("/enqueue", { target: "hpa", delayMs: 15_000 });
  await r.post("/enqueue", { target: "workloads", delayMs: 5_000, changed: ["jobs", "pods"] });
  await r.settle();

  assert.deepEqual(sent, []);
  assert.equal(r.alarm.at, start + 5_000);
});

test("each deadline fires on time on its own queue with the message it was booked with, and an emptied table clears the alarm", async (t) => {
  const tick = clock(t);
  const sent: Sent[] = [];
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0", ...recordingQueues(sent) } as any);
  await r.settle();
  sent.length = 0;
  await r.post("/enqueue", { target: "hpa", delayMs: 15_000 });
  await r.post("/enqueue", { target: "workloads", delayMs: 5_000, changed: ["jobs", "pods"] });
  await r.post("/enqueue", { target: "scheduler", delayMs: 300_000, attempt: 2 });

  tick(5_000);
  await r.fire();
  assert.deepEqual(sent, [{ queue: "wl", body: { kind: "retry", changed: ["jobs", "pods"] }, delaySeconds: undefined }]);
  assert.equal(r.alarm.at, start + 15_000);

  tick(10_000);
  await r.fire();
  assert.deepEqual(sent.slice(1), [{ queue: "hpa", body: { kind: "retry" }, delaySeconds: undefined }]);
  assert.equal(r.alarm.at, start + 300_000);

  tick(285_000);
  await r.fire();
  assert.deepEqual(sent.slice(2), [{ queue: "sched", body: { kind: "retry", attempt: 2 }, delaySeconds: undefined }]);
  assert.equal(r.alarm.at, null);
  assert.deepEqual(booked(r), []);
});

test("booking the same work again keeps one deadline at the earliest due, while different work keeps its own", async (t) => {
  clock(t);
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0" });
  await r.settle();
  await r.post("/enqueue", { target: "hpa", delayMs: 15_000 });
  await r.post("/enqueue", { target: "hpa", delayMs: 5_000 });
  await r.post("/enqueue", { target: "hpa", delayMs: 15_000 });
  await r.post("/enqueue", { target: "scheduler", delayMs: 300_000, attempt: 0 });
  await r.post("/enqueue", { target: "scheduler", delayMs: 300_000, attempt: 1 });
  await r.post("/enqueue", { target: "workloads", delayMs: 2_000, changed: ["pods", "jobs"] });
  await r.post("/enqueue", { target: "workloads", delayMs: 1_000, changed: ["jobs", "pods"] });
  await r.post("/enqueue", { target: "workloads", delayMs: 2_000, changed: ["services"] });
  assert.deepEqual(booked(r), [
    { target: "workloads", due: start + 1_000 },
    { target: "workloads", due: start + 2_000 },
    { target: "hpa", due: start + 5_000 },
    { target: "scheduler", due: start + 300_000 },
  ]);
  assert.equal(r.alarm.at, start + 1_000);
});

test("work booked again after its deadline passed is booked at the new due, not lost when the old one is sent", async (t) => {
  const tick = clock(t);
  const sent: Sent[] = [];
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0", ...recordingQueues(sent) } as any);
  await r.settle();
  sent.length = 0;
  await r.post("/enqueue", { target: "hpa", delayMs: 15_000 });
  tick(16_000);
  await r.post("/enqueue", { target: "hpa", delayMs: 15_000 });
  await r.fire();
  assert.deepEqual(sent, []);
  assert.deepEqual(booked(r), [{ target: "hpa", due: start + 31_000 }]);
  assert.equal(r.alarm.at, start + 31_000);
});

test("a deadline survives eviction: a new instance on the same storage books the alarm again", async (t) => {
  clock(t);
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0" });
  await r.settle();
  await r.post("/enqueue", { target: "gc", delayMs: 2_000 });
  r.alarm.at = null;

  const pending: Promise<unknown>[] = [];
  const ctx = {
    storage: {
      sql: r.sql,
      getAlarm: async () => r.alarm.at,
      setAlarm: async (at: number) => void (r.alarm.at = at),
      deleteAlarm: async () => void (r.alarm.at = null),
    },
    waitUntil: (p: Promise<unknown>) => void pending.push(p),
    blockConcurrencyWhile: (fn: () => Promise<void>) => fn(),
    acceptWebSocket: () => {},
    getWebSockets: () => [],
  };
  new (r.cluster.constructor as any)(ctx, { ...recordingQueues([]), SNAPSHOT_INTERVAL_HOURS: "0", CLUSTER_UID: "test" });
  while (pending.length) await pending.shift();

  assert.equal(r.alarm.at, start + 2_000);
});

test("a deadline whose send fails stays booked and is retried a few seconds later, not in a tight loop", async (t) => {
  const tick = clock(t);
  const sent: Sent[] = [];
  const failing = new Set(["gc"]);
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0", ...recordingQueues(sent, failing) } as any);
  await r.settle();
  sent.length = 0;
  await r.post("/enqueue", { target: "gc", delayMs: 2_000 });
  tick(2_000);
  await r.fire();
  assert.deepEqual(sent, []);
  assert.equal(r.alarm.at, start + 7_000);

  failing.clear();
  tick(5_000);
  await r.fire();
  assert.deepEqual(sent, [{ queue: "gc", body: { kind: "retry" }, delaySeconds: undefined }]);
  assert.equal(r.alarm.at, null);
});

test("an unknown target is refused and books nothing", async (t) => {
  clock(t);
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0" });
  await r.settle();
  assert.equal((await r.post("/enqueue", { target: "nope", delayMs: 1_000 })).status, 400);
  assert.deepEqual(booked(r), []);
  assert.equal(r.alarm.at, null);
});

test("a node lease books its check as a deadline on the alarm, never as a delayed queue message", async (t) => {
  const tick = clock(t);
  const sent: Sent[] = [];
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0", ...recordingQueues(sent) } as any);
  await r.settle();
  sent.length = 0;
  await r.put("/registry/leases/kube-node-lease/n1", "v1");
  await r.settle();
  assert.deepEqual(sent, []);
  assert.deepEqual(booked(r), [{ target: "leases", due: start + 60_000 }]);

  tick(60_000);
  await r.fire();
  assert.deepEqual(sent, [{ queue: "ctrl", body: { kind: "lease-check", node: "n1" }, delaySeconds: undefined }]);
  assert.deepEqual(booked(r), []);

  await r.post("/lease-check", { node: "n1", delayMs: 30_000 });
  assert.deepEqual(booked(r), [{ target: "leases", due: start + 90_000 }]);
  assert.equal(r.alarm.at, start + 90_000);
});
