import { LIST_SQL, INSERT_SQL, GET_SQL } from "./schema.ts";
import { prefixEnd, decodeKineValue, rowToEvent } from "./helpers.ts";
import type { KineRow } from "./helpers.ts";
import type { SqlExec } from "./queries.ts";
import { broadcastEvent, type DurableObjectContext } from "./watch.ts";

export function runScheduler(ctx: DurableObjectContext, sql: SqlExec, _env: any): void {
  allocatePodCIDRs(ctx, sql);
  scheduleUnboundPods(ctx, sql);
}

/**
 * Allocate PodCIDRs to nodes that don't have one.
 * Each node gets a /24 subnet from 10.42.0.0/16.
 * The subnet index is persisted in a counter key so allocations are stable.
 */
export function allocatePodCIDRs(ctx: DurableObjectContext, sql: SqlExec): void {
  const nodePrefix = "/registry/nodes/";
  const nodeQuery = LIST_SQL("AND mkv.name > ?4");
  const nodeRows = sql.exec(nodeQuery, nodePrefix, prefixEnd(nodePrefix), 0, "").toArray();

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

      sql.exec(INSERT_SQL, key, 0, 0, row.create_revision, row.theid, 0, encodedValue, value);
      const newId = sql.exec("SELECT last_insert_rowid() AS id").one().id as number;
      broadcastEvent(ctx, sql, key, newId);
      console.log(`Allocated PodCIDR 10.42.${subnetIndex}.0/24 to ${node.metadata?.name}`);
    } catch (e) {
      console.error("PodCIDR allocation error:", e);
    }
  }
}

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

export function scheduleUnboundPods(ctx: DurableObjectContext, sql: SqlExec): void {
  // Get all current pod entries (latest revision per key)
  const podPrefix = "/registry/pods/";
  const podQuery = LIST_SQL("AND mkv.name > ?4");
  const podRows = sql.exec(podQuery, podPrefix, prefixEnd(podPrefix), 0, "").toArray();

  // Get all current node entries
  const nodePrefix = "/registry/nodes/";
  const nodeRows = sql.exec(podQuery, nodePrefix, prefixEnd(nodePrefix), 0, "").toArray();

  if (nodeRows.length === 0) return; // No nodes available

  // Decode nodes
  const nodes: string[] = [];
  for (const row of nodeRows) {
    if (row.deleted === 1) continue;
    try {
      const value = row.value;
      if (!value) continue;
      const obj = JSON.parse(decodeKineValue(value));
      if (obj.metadata && obj.metadata.name) {
        nodes.push(obj.metadata.name);
      }
    } catch (_) {}
  }

  if (nodes.length === 0) return;

  let nodeIndex = 0;

  for (const row of podRows) {
    if (row.deleted === 1) continue;
    try {
      const value = row.value;
      if (!value) continue;
      const pod = JSON.parse(decodeKineValue(value));

      // Skip if already scheduled
      if (pod.spec && pod.spec.nodeName && pod.spec.nodeName !== "") continue;

      // Assign to a node (round-robin)
      if (!pod.spec) pod.spec = {};
      pod.spec.nodeName = nodes[nodeIndex % nodes.length];
      nodeIndex++;

      // Re-encode and update
      const updatedJson = JSON.stringify(pod);
      const encodedValue = new TextEncoder().encode(updatedJson).buffer;

      // Update the pod in storage using kine insert pattern
      const key = row.thename;
      const prevRevision = row.theid;

      sql.exec(INSERT_SQL, key, 0, 0, row.create_revision, prevRevision, 0, encodedValue, value);

      // Broadcast the change
      const newId = sql.exec("SELECT last_insert_rowid() AS id").one().id as number;
      broadcastEvent(ctx, sql, key, newId);
    } catch (e) {
      // Skip pods that can't be parsed
      console.error("Scheduler error:", e);
    }
  }
}
