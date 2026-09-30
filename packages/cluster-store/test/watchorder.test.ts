import assert from "node:assert/strict";
import { test } from "node:test";
import { rig } from "./harness.ts";
import type { FakeSocket } from "./harness.ts";

const events = (ws: FakeSocket) => ws.sent.filter((e) => e.type !== "progress").map((e) => [e.type, e.rev, e.key]);

test("watches opened from every revision of a producer's stream see the same ordered events", async () => {
  const r = rig();
  await r.settle();
  const prefix = "/registry/configmaps/watch/";
  const existing = new Map<string, number>();
  let next = 0;
  let seed = 7;
  const random = (n: number) => {
    seed = (seed * 48271) % 2147483647;
    return seed % n;
  };
  const write = async (i: number) => {
    const op = existing.size === 0 ? 0 : random(3);
    if (op === 0) {
      const key = `${prefix}cm-${next++}`;
      await r.put(key, `v${i}`);
      existing.set(key, 0);
      return;
    }
    const key = [...existing.keys()][random(existing.size)];
    if (op === 1) {
      const current = (await r.get(`/kv?key=${encodeURIComponent(key)}`)).kv.modRevision;
      await r.put(key, `u${i}`, current);
      return;
    }
    await r.remove(key);
    existing.delete(key);
  };
  const lease = (i: number) => r.put(`/registry/leases/kube-node-lease/node-${i % 3}`, `lease${i}`);

  const first = r.watch({ prefix, since: String(await r.get("/revision").then((b) => b.revision)) });
  const watches = [first];
  for (let i = 0; i < 60; i++) {
    await write(i);
    if (i % 4 === 0) await lease(i);
    const seen = events(first);
    const latest = seen[seen.length - 1][1] as number;
    for (const ws of watches) assert.deepEqual(events(ws).slice(-1)[0], seen[seen.length - 1], `watch ${watches.indexOf(ws)} lags at write ${i}`);
    watches.push(r.watch({ prefix, since: String(latest) }));
  }
  const reference = events(first);
  for (const [n, ws] of watches.entries()) {
    const mine = events(ws);
    assert.deepEqual(mine, reference.slice(reference.length - mine.length), `watch ${n} differs from the first`);
  }
  assert.ok(reference.length >= 60);
});

test("a watch redialed from the last event it saw continues the stream without a gap or a repeat", async () => {
  const r = rig();
  await r.settle();
  const prefix = "/registry/configmaps/watch/";
  const start = (await r.get("/revision")).revision;
  const ws = r.watch({ prefix, since: String(start) });
  for (let i = 0; i < 5; i++) await r.put(`${prefix}cm-${i}`, `v${i}`);
  const seen = events(ws);
  ws.closed = "peer";
  await r.cluster.webSocketClose(ws as any);
  for (let i = 5; i < 10; i++) await r.put(`${prefix}cm-${i}`, `v${i}`);
  const again = r.watch({ prefix, since: String(seen[seen.length - 1][1]) });
  await r.put(`${prefix}cm-10`, "v10");
  assert.deepEqual(events(ws).length, 5);
  assert.deepEqual(
    events(again).map((e) => e[2]),
    [5, 6, 7, 8, 9, 10].map((i) => `${prefix}cm-${i}`),
  );
});
