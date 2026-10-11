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

const lease = (seconds?: number) => JSON.stringify({ spec: seconds === undefined ? {} : { leaseDurationSeconds: seconds } });
const node = (ready: string) => JSON.stringify({ status: { conditions: [{ type: "Ready", status: ready }] } });

async function fireIfDue(r: ReturnType<typeof rig>): Promise<void> {
  if (r.alarm.at !== null && r.alarm.at <= Date.now()) await r.fire();
}

test("a node that keeps renewing its lease is checked once when first seen and never again, and books one alarm at the lease expiry", async (t) => {
  const tick = clock(t);
  const sent: Sent[] = [];
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0", ...recordingQueues(sent) } as any);
  await r.settle();
  sent.length = 0;

  let rev = Number((await (await r.put("/registry/leases/kube-node-lease/n1", lease(40))).json()).revision);
  await r.settle();
  tick(1_000);
  await fireIfDue(r);
  assert.deepEqual(sent, [{ queue: "ctrl", body: { kind: "lease-check", node: "n1" }, delaySeconds: undefined }]);

  for (let i = 0; i < 30; i++) {
    tick(10_000);
    await fireIfDue(r);
    rev = Number((await (await r.put("/registry/leases/kube-node-lease/n1", lease(40), rev)).json()).revision);
    await r.settle();
    assert.deepEqual(booked(r).filter((d: any) => d.target !== "workloads"), [{ target: "node-lease", due: Date.now() + 60_000 }]);
  }
  assert.equal(sent.filter((m) => m.queue === "ctrl").length, 1);
});

test("a lease that stops being renewed is checked exactly at its expiry, and the next renewal is checked at once", async (t) => {
  const tick = clock(t);
  const sent: Sent[] = [];
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0", ...recordingQueues(sent) } as any);
  await r.settle();
  const rev = Number((await (await r.put("/registry/leases/kube-node-lease/n1", lease(15))).json()).revision);
  await r.settle();
  tick(1_000);
  await r.fire();
  sent.length = 0;

  assert.deepEqual(booked(r), [{ target: "node-lease", due: start + 35_000 }]);
  tick(34_000);
  await r.fire();
  assert.deepEqual(sent.filter((m) => m.queue === "ctrl"), [{ queue: "ctrl", body: { kind: "lease-check", node: "n1" }, delaySeconds: undefined }]);
  assert.deepEqual(booked(r), []);

  tick(120_000);
  await r.put("/registry/leases/kube-node-lease/n1", lease(15), rev);
  await r.settle();
  assert.deepEqual(booked(r), [
    { target: "leases", due: Date.now() },
    { target: "node-lease", due: Date.now() + 35_000 },
  ]);
});

test("a node whose Ready condition changes is checked at once, and other node writes book no check", async (t) => {
  clock(t);
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0" });
  await r.settle();
  const created = Number((await (await r.put("/registry/minions/n1", node("True"))).json()).revision);
  r.sql.exec("DELETE FROM deadlines");
  const relabelled = Number((await (await r.put("/registry/minions/n1", node("True"), created)).json()).revision);
  assert.deepEqual(booked(r), []);
  await r.put("/registry/minions/n1", node("False"), relabelled);
  assert.deepEqual(booked(r), [{ target: "leases", due: start }]);
});

test("deleting a node lease removes its expiry, so a removed node books nothing", async (t) => {
  clock(t);
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0" });
  await r.settle();
  await r.put("/registry/leases/kube-node-lease/n1", lease(40));
  r.sql.exec("DELETE FROM deadlines WHERE target = 'leases'");
  await r.remove("/registry/leases/kube-node-lease/n1");
  await r.settle();
  assert.deepEqual(booked(r), []);
});

test("a lease check a node health pass asks for is not pushed back by lease renewals", async (t) => {
  const tick = clock(t);
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0" });
  await r.settle();
  let rev = Number((await (await r.put("/registry/leases/kube-node-lease/n1", lease(40))).json()).revision);
  r.sql.exec("DELETE FROM deadlines WHERE target = 'leases'");
  await r.post("/lease-check", { node: "n1", delayMs: 300_000 });
  for (let i = 0; i < 3; i++) {
    tick(10_000);
    rev = Number((await (await r.put("/registry/leases/kube-node-lease/n1", lease(40), rev)).json()).revision);
  }
  assert.deepEqual(booked(r), [
    { target: "node-lease", due: Date.now() + 60_000 },
    { target: "leases", due: start + 300_000 },
  ]);
});

test("a pending Helm hook's deadline is carried by the alarm as one addons row and fires once", async (t) => {
  const tick = clock(t);
  const sent: Sent[] = [];
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0", ...recordingQueues(sent) } as any);
  await r.settle();
  sent.length = 0;

  await r.post("/enqueue", { target: "addons", delayMs: 300_000 });
  tick(10_000);
  await r.post("/enqueue", { target: "addons", delayMs: 295_000 });
  assert.deepEqual(booked(r), [{ target: "addons", due: start + 300_000 }]);
  assert.equal(r.alarm.at, start + 300_000);

  tick(289_999);
  await fireIfDue(r);
  assert.deepEqual(sent, []);

  tick(1);
  await r.fire();
  assert.deepEqual(sent, [{ queue: "addon", body: { kind: "retry" }, delaySeconds: undefined }]);
  assert.deepEqual(booked(r), []);
  assert.equal(r.alarm.at, null);

  tick(600_000);
  await r.fire();
  assert.equal(sent.length, 1);
});

test("a Helm pass with no hook pending clears the addons row and leaves a storage-migrate retry booked", async (t) => {
  const tick = clock(t);
  const sent: Sent[] = [];
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0", ...recordingQueues(sent) } as any);
  await r.settle();
  sent.length = 0;

  await r.post("/enqueue", { target: "addons", delayMs: 300_000 });
  await r.post("/cancel", { target: "addons" });
  assert.deepEqual(booked(r), []);
  assert.equal(r.alarm.at, null);

  await r.post("/enqueue", { target: "addons", delayMs: 300_000 });
  await r.post("/enqueue", { target: "addons", delayMs: 5_000, names: ["storage-migrate"] });
  await r.post("/cancel", { target: "addons" });
  assert.deepEqual(booked(r), [{ target: "addons", due: start + 5_000 }]);

  tick(5_000);
  await r.fire();
  tick(300_000);
  await r.fire();
  assert.deepEqual(sent, [{ queue: "addon", body: { kind: "retry", names: ["storage-migrate"] }, delaySeconds: undefined }]);
  assert.equal(r.alarm.at, null);
  assert.equal((await r.post("/cancel", { target: "nope" })).status, 400);
});

test("a failed retry's count is kept in its deadline row: a new instance sends it, a repeated failure keeps the highest count, and a booking without failures clears it", async (t) => {
  const tick = clock(t);
  const sent: Sent[] = [];
  const r = rig({ SNAPSHOT_INTERVAL_HOURS: "0", ...recordingQueues(sent) } as any);
  await r.settle();
  sent.length = 0;

  await r.post("/enqueue", { target: "gc", delayMs: 40_000, failures: 4 });
  await r.post("/enqueue", { target: "gc", delayMs: 5_000, failures: 1 });
  await r.post("/lease-check", { node: "n1", delayMs: 20_000, failures: 3 });
  assert.deepEqual(booked(r), [
    { target: "gc", due: start + 5_000 },
    { target: "leases", due: start + 20_000 },
  ]);

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
  const fresh = new (r.cluster.constructor as any)(ctx, { ...recordingQueues(sent), SNAPSHOT_INTERVAL_HOURS: "0", CLUSTER_UID: "test" });
  while (pending.length) await pending.shift();
  tick(20_000);
  await fresh.alarm();
  assert.deepEqual(sent, [
    { queue: "gc", body: { kind: "retry", failures: 4 }, delaySeconds: undefined },
    { queue: "ctrl", body: { kind: "lease-check", node: "n1", failures: 3 }, delaySeconds: undefined },
  ]);

  await r.post("/enqueue", { target: "workloads", delayMs: 80_000, changed: ["pods"], failures: 5 });
  await r.post("/enqueue", { target: "workloads", delayMs: 3_000, changed: ["pods"] });
  tick(3_000);
  await r.fire();
  assert.deepEqual(sent.slice(2), [{ queue: "wl", body: { kind: "retry", changed: ["pods"] }, delaySeconds: undefined }]);
});
