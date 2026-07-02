import { GET_SQL, INSERT_SQL } from "./schema.ts";
import { decodeKineValue } from "./helpers.ts";
import type { SqlExec } from "./queries.ts";
import { broadcastEvent, type WatchHost } from "./watch.ts";
import { storeInsert, storeListRaw } from "./store.ts";

// Pod-to-node binding is done by a real kube-scheduler (cmd/scheduler) running
// as an external process against this apiserver — see
// docs/control-plane-architecture.md's "Migrating to the real kube-scheduler"
// section. This module now only handles PodCIDR allocation, which remains a
// controller-manager (node-ipam-controller) responsibility that kube-scheduler
// itself never performed, even before that migration.
export async function runScheduler(host: WatchHost, sql: SqlExec, _env: any): Promise<void> {
  await allocatePodCIDRs(host, sql);
}

/**
 * Whether writing this key is a change this module should react to promptly
 * (a node that still needs a PodCIDR) rather than waiting for the periodic
 * safety-net resync.
 */
export function needsSchedulerAttention(key: string, value: ArrayBuffer | string | null): boolean {
  if (!value) return false;
  try {
    if (key.startsWith("/registry/nodes/")) {
      const node = JSON.parse(decodeKineValue(value));
      return !node.spec?.podCIDR;
    }
  } catch {
    return false;
  }
  return false;
}

/**
 * Allocate PodCIDRs to nodes that don't have one.
 * Each node gets a /24 subnet from 10.42.0.0/16.
 * The subnet index is persisted in a counter key so allocations are stable.
 *
 * Nodes are cluster-scoped (see keyspace.ts), so this reads/writes through
 * storeListRaw/storeInsert purely for a uniform write path -- classifyKey
 * routes every key here straight to the parent's own log, identically to a
 * direct sql call.
 */
export async function allocatePodCIDRs(host: WatchHost, sql: SqlExec): Promise<void> {
  const nodePrefix = "/registry/nodes/";
  const nodeRows = await storeListRaw(sql, host, nodePrefix, false);

  for (const row of nodeRows) {
    if (row.deleted === 1) continue;
    try {
      const value = row.value;
      if (!value) continue;
      const node = JSON.parse(decodeKineValue(value));

      // Skip if already has PodCIDR
      if (node.spec && node.spec.podCIDR && node.spec.podCIDR !== "") continue;

      // Allocate a /24 from 10.42.0.0/16
      const subnetIndex = nextPodCIDRIndex(sql);
      if (subnetIndex > 255) {
        console.error("PodCIDR allocation exhausted (max 256 nodes)");
        continue;
      }

      if (!node.spec) node.spec = {};
      node.spec.podCIDR = `10.42.${subnetIndex}.0/24`;
      node.spec.podCIDRs = [`10.42.${subnetIndex}.0/24`];

      const updatedJson = JSON.stringify(node);
      const encodedValue = new TextEncoder().encode(updatedJson).buffer;
      const key = row.thename;

      const newId = await storeInsert(
        sql,
        host,
        key,
        false,
        false,
        row.create_revision,
        row.theid,
        0,
        encodedValue,
        value,
      );
      await broadcastEvent(host, sql, key, newId);
      console.log(`Allocated PodCIDR 10.42.${subnetIndex}.0/24 to ${node.metadata?.name}`);
    } catch (e) {
      console.error("PodCIDR allocation error:", e);
    }
  }
}

// The podcidr-counter key is a cluster-internal key (not under a namespace
// resource), so this stays on direct sql access -- see keyspace.ts's
// CLUSTER_SCOPED_RESOURCES ("_internal").
export function nextPodCIDRIndex(sql: SqlExec): number {
  const counterKey = "/registry/_internal/podcidr-counter";
  const q = GET_SQL(false);
  const rows = sql.exec(q, counterKey).toArray();

  let index = 0;
  if (rows.length > 0 && !rows[0].deleted) {
    const raw = rows[0].value;
    const val = decodeKineValue(raw!);
    index = parseInt(val) || 0;
  }

  const next = index + 1;
  const nextVal = new TextEncoder().encode(String(next)).buffer;

  if (rows.length > 0 && !rows[0].deleted) {
    sql.exec(
      INSERT_SQL,
      counterKey,
      0,
      0,
      rows[0].create_revision,
      rows[0].theid,
      0,
      nextVal,
      rows[0].value,
    );
  } else {
    sql.exec(INSERT_SQL, counterKey, 1, 0, 0, 0, 0, nextVal, null);
  }

  return index;
}
