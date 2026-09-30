import assert from "node:assert/strict";
import { test } from "node:test";
import { compactionTarget, nextAlarmAt, snapshotSchedule, snapshotsToPrune } from "../src/schedule.ts";

test("nextAlarmAt picks the earliest deadline and never sooner than the minimum delay", () => {
  assert.equal(nextAlarmAt([null, null], 1000), null);
  assert.equal(nextAlarmAt([9000, null, 5000], 1000), 5000);
  assert.equal(nextAlarmAt([1200, 900], 1000), 2000);
});

test("compactionTarget keeps the time-based target unless the size bound is exceeded", () => {
  assert.equal(compactionTarget({ revision: 500, timeTarget: 300, maxRetained: 1000 }), 300);
  assert.equal(compactionTarget({ revision: 5000, timeTarget: 300, maxRetained: 1000 }), 4000);
  assert.equal(compactionTarget({ revision: 5000, timeTarget: 5000, maxRetained: 1000 }), 5000);
});

test("snapshotSchedule defaults to every 12 hours keeping 5 and can be disabled", () => {
  assert.deepEqual(snapshotSchedule({}), { intervalMs: 12 * 3600_000, retention: 5 });
  assert.deepEqual(snapshotSchedule({ SNAPSHOT_INTERVAL_HOURS: "6", SNAPSHOT_RETENTION: "3" }), { intervalMs: 6 * 3600_000, retention: 3 });
  assert.equal(snapshotSchedule({ SNAPSHOT_INTERVAL_HOURS: "0" }), null);
  assert.deepEqual(snapshotSchedule({ SNAPSHOT_INTERVAL_HOURS: "junk", SNAPSHOT_RETENTION: "-1" }), { intervalMs: 12 * 3600_000, retention: 5 });
});

test("snapshotsToPrune drops the oldest scheduled snapshots beyond the retention", () => {
  const keys = ["p/scheduled-2026-03.json", "p/2026-01.json", "p/scheduled-2026-01.json", "p/scheduled-2026-02.json"];
  assert.deepEqual(snapshotsToPrune(keys, "p/scheduled-", 2), ["p/scheduled-2026-01.json"]);
  assert.deepEqual(snapshotsToPrune(keys, "p/scheduled-", 5), []);
});
