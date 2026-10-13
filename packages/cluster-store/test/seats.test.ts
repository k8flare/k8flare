import assert from "node:assert/strict";
import { test } from "node:test";
import { SEAT_HOLD_MS, Seats, type SeatOutcome, type SeatRequest } from "../src/seats.ts";
import { rig } from "./harness.ts";

class Clock {
  at = 0;
  timers = new Map<number, { due: number; fn: () => void }>();
  next = 1;
  now = () => this.at;
  set = (fn: () => void, ms: number) => {
    const id = this.next++;
    this.timers.set(id, { due: this.at + ms, fn });
    return id;
  };
  clear = (id: unknown) => void this.timers.delete(id as number);
  advance(ms: number): void {
    this.at += ms;
    for (const [id, t] of [...this.timers].sort(([, a], [, b]) => a.due - b.due)) {
      if (t.due > this.at) continue;
      this.timers.delete(id);
      t.fn();
    }
  }
}

function seats(clock: Clock, qps = 0, burst = 0): Seats {
  return new Seats(qps, burst, clock.now, clock.set, clock.clear);
}

const queuing = (over: Partial<SeatRequest> = {}): SeatRequest => ({ level: "workload-low", limit: 1, queues: 4, queueLengthLimit: 1, hand: [0, 1], waitMs: 15_000, ...over });

async function settled(p: Promise<SeatOutcome>): Promise<SeatOutcome | "pending"> {
  return Promise.race([p, new Promise<"pending">((r) => setImmediate(() => r("pending")))]);
}

test("a level at its limit queues the next request and rejects it with Retry-After when its wait runs out", async () => {
  const clock = new Clock();
  const s = seats(clock);
  const first = await s.acquire(queuing());
  assert.equal(first.admitted, true);
  const queued = s.acquire(queuing());
  assert.equal(await settled(queued), "pending");
  clock.advance(15_000);
  assert.deepEqual(await queued, { admitted: false, retryAfter: 1 });
});

test("a released seat goes to the queued request", async () => {
  const clock = new Clock();
  const s = seats(clock);
  const first = await s.acquire(queuing());
  const queued = s.acquire(queuing());
  assert.equal(await settled(queued), "pending");
  if (first.admitted) first.release();
  assert.equal((await queued).admitted, true);
});

test("requests beyond the queue length limit are rejected at once", async () => {
  const clock = new Clock();
  const s = seats(clock);
  await s.acquire(queuing({ hand: [0] }));
  const waiting = s.acquire(queuing({ hand: [0] }));
  assert.deepEqual(await s.acquire(queuing({ hand: [0] })), { admitted: false, retryAfter: 1 });
  clock.advance(15_000);
  await waiting;
});

test("a reject-type level rejects at its limit without queuing", async () => {
  const clock = new Clock();
  const s = seats(clock);
  const reject = { level: "e2e", limit: 1, queues: 0, queueLengthLimit: 0, hand: [], waitMs: 15_000 };
  assert.equal((await s.acquire(reject)).admitted, true);
  assert.deepEqual(await s.acquire(reject), { admitted: false, retryAfter: 1 });
});

test("a raised limit admits queued requests", async () => {
  const clock = new Clock();
  const s = seats(clock);
  await s.acquire(queuing());
  const queued = s.acquire(queuing());
  assert.equal(await settled(queued), "pending");
  assert.equal((await s.acquire(queuing({ limit: 3, hand: [2] }))).admitted, true);
  assert.equal((await queued).admitted, true);
});

test("queues are served in turn", async () => {
  const clock = new Clock();
  const s = seats(clock);
  const first = await s.acquire(queuing({ queueLengthLimit: 3, hand: [0] }));
  const order: string[] = [];
  const a1 = s.acquire(queuing({ queueLengthLimit: 3, hand: [0] })).then((o) => (order.push("a1"), o));
  const a2 = s.acquire(queuing({ queueLengthLimit: 3, hand: [0] })).then((o) => (order.push("a2"), o));
  const b1 = s.acquire(queuing({ queueLengthLimit: 3, hand: [1] })).then((o) => (order.push("b1"), o));
  let held = first;
  for (const next of [a1, b1, a2]) {
    if (held.admitted) held.release();
    held = await next;
  }
  assert.deepEqual(order, ["a1", "b1", "a2"]);
});

test("a seat that is never released is reclaimed after the hold limit", async () => {
  const clock = new Clock();
  const s = seats(clock);
  await s.acquire(queuing());
  const queued = s.acquire(queuing({ waitMs: SEAT_HOLD_MS * 2 }));
  clock.advance(SEAT_HOLD_MS);
  assert.equal((await queued).admitted, true);
});

test("the rate limiter rejects a user over the rate with Retry-After and lets other users and keyless requests through", async () => {
  const clock = new Clock();
  const s = seats(clock, 2, 2);
  const free = { ...queuing(), limit: 100 };
  assert.equal((await s.acquire({ ...free, user: "user:noisy" })).admitted, true);
  assert.equal((await s.acquire({ ...free, user: "user:noisy" })).admitted, true);
  assert.deepEqual(await s.acquire({ ...free, user: "user:noisy" }), { admitted: false, retryAfter: 1 });
  assert.equal((await s.acquire({ ...free, user: "user:quiet" })).admitted, true);
  for (let i = 0; i < 10; i++) assert.equal((await s.acquire(free)).admitted, true);
  clock.advance(500);
  assert.equal((await s.acquire({ ...free, user: "user:noisy" })).admitted, true);
});

test("the seat endpoint holds the seat until the caller cancels the body", async () => {
  const r = rig();
  const body = { level: "global-default", limit: 1, queues: 0, queueLengthLimit: 0, hand: [], waitMs: 1000 };
  const held = await r.post("/apf/seat", body);
  assert.equal(held.status, 200);
  const refused = await r.post("/apf/seat", body);
  assert.equal(refused.status, 429);
  assert.deepEqual(await refused.json(), { retryAfter: 1 });
  await held.body!.cancel();
  const again = await r.post("/apf/seat", body);
  assert.equal(again.status, 200);
  await again.body!.cancel();
});

test("the seat endpoint applies the configured rate limit", async () => {
  const r = rig({ API_RATE_LIMIT_QPS: "1", API_RATE_LIMIT_BURST: "1" });
  const body = { level: "global-default", limit: 10, queues: 0, queueLengthLimit: 0, hand: [], waitMs: 1000, user: "addr:203.0.113.9" };
  const first = await r.post("/apf/seat", body);
  assert.equal(first.status, 200);
  const second = await r.post("/apf/seat", body);
  assert.equal(second.status, 429);
  await first.body!.cancel();
});
