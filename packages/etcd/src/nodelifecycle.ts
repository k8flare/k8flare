import { LIST_SQL, INSERT_SQL, GET_SQL } from "./schema.ts";
import { prefixEnd, decodeKineValue } from "./helpers.ts";
import type { SqlExec } from "./queries.ts";
import { broadcastEvent, type DurableObjectContext } from "./watch.ts";

// Matches upstream Kubernetes' default --node-monitor-grace-period: how long
// a Lease can go unrenewed before the node is considered unreachable.
const UNREACHABLE_GRACE_MS = 40_000;
// Matches upstream's default --pod-eviction-timeout: how much longer an
// unreachable node is given before its Pods are deleted outright.
const POD_EVICTION_MS = 5 * 60_000;

const NODES_PREFIX = "/registry/nodes/";
const LEASE_PREFIX = "/registry/leases/kube-node-lease/";
const PODS_PREFIX = "/registry/pods/";

/**
 * Detect nodes whose kubelet has stopped renewing its Lease, mark them
 * Unknown + taint them unreachable, and evict their Pods once stale for
 * long enough. Runs unconditionally on every alarm tick (like
 * allocatePodCIDRs) -- staleness is detected by the ABSENCE of an expected
 * write, so there is no useful needsXAttention trigger-check for this one;
 * the periodic safety-net interval is the only thing that can catch it.
 *
 * Deliberately not implemented: recovery (a node coming back healthy has its
 * Unknown status / taint cleared automatically by kubelet's own next
 * heartbeat overwriting status, but nothing here proactively removes a taint
 * on its own -- an accepted gap for this first pass).
 */
export function reconcileNodeLifecycle(ctx: DurableObjectContext, sql: SqlExec): void {
  const nodeQuery = LIST_SQL("AND mkv.name > ?4");
  const nodeRows = sql.exec(nodeQuery, NODES_PREFIX, prefixEnd(NODES_PREFIX), 0, "").toArray();

  for (const row of nodeRows) {
    if (row.deleted === 1) continue;
    try {
      const value = row.value;
      if (!value) continue;
      const node = JSON.parse(decodeKineValue(value));
      const nodeName: string | undefined = node.metadata?.name;
      if (!nodeName) continue;

      const leaseKey = `${LEASE_PREFIX}${nodeName}`;
      const leaseRows = sql.exec(GET_SQL(false), leaseKey).toArray();
      if (leaseRows.length === 0 || leaseRows[0].deleted || !leaseRows[0].value) {
        continue; // no lease yet -- node hasn't finished registering
      }
      const lease = JSON.parse(decodeKineValue(leaseRows[0].value));
      const renewTime = lease.spec?.renewTime;
      if (!renewTime) continue;
      const renewMs = new Date(renewTime).getTime();
      if (Number.isNaN(renewMs)) continue;

      const staleMs = Date.now() - renewMs;
      if (staleMs <= UNREACHABLE_GRACE_MS) continue;

      let changed = false;
      const conditions: any[] = node.status?.conditions || [];
      for (const cond of conditions) {
        if (
          cond.status !== "Unknown" &&
          (cond.type === "Ready" ||
            cond.type === "MemoryPressure" ||
            cond.type === "DiskPressure" ||
            cond.type === "PIDPressure")
        ) {
          cond.status = "Unknown";
          cond.lastTransitionTime = new Date().toISOString();
          if (cond.type === "Ready") {
            cond.reason = "NodeStatusUnknown";
            cond.message = "Kubelet stopped posting node status.";
          }
          changed = true;
        }
      }
      if (node.status) node.status.conditions = conditions;

      if (!node.spec) node.spec = {};
      if (!node.spec.taints) node.spec.taints = [];
      const hasUnreachableTaint = node.spec.taints.some(
        (t: any) => t.key === "node.kubernetes.io/unreachable",
      );
      if (!hasUnreachableTaint) {
        node.spec.taints.push({
          key: "node.kubernetes.io/unreachable",
          effect: "NoExecute",
          timeAdded: new Date().toISOString(),
        });
        changed = true;
      }

      if (changed) {
        upsertNode(ctx, sql, row, node);
      }

      if (staleMs > POD_EVICTION_MS) {
        evictPodsOnNode(ctx, sql, nodeName);
      }
    } catch (e) {
      console.error("Node lifecycle reconciliation error:", e);
    }
  }
}

function upsertNode(ctx: DurableObjectContext, sql: SqlExec, row: any, node: any): void {
  const updatedJson = JSON.stringify(node);
  const encodedValue = new TextEncoder().encode(updatedJson).buffer;
  const key = row.thename;
  sql.exec(INSERT_SQL, key, 0, 0, row.create_revision, row.theid, 0, encodedValue, row.value);
  const newId = sql.exec("SELECT last_insert_rowid() AS id").one().id as number;
  broadcastEvent(ctx, sql, key, newId);
  console.log(`Node ${node.metadata?.name} marked unreachable (Unknown + tainted)`);
}

function evictPodsOnNode(ctx: DurableObjectContext, sql: SqlExec, nodeName: string): void {
  const podQuery = LIST_SQL("AND mkv.name > ?4");
  const podRows = sql.exec(podQuery, PODS_PREFIX, prefixEnd(PODS_PREFIX), 0, "").toArray();

  for (const podRow of podRows) {
    if (podRow.deleted === 1) continue;
    try {
      if (!podRow.value) continue;
      const pod = JSON.parse(decodeKineValue(podRow.value));
      if (pod.spec?.nodeName !== nodeName) continue;

      const key = podRow.thename;
      sql.exec(
        INSERT_SQL,
        key,
        0,
        1,
        podRow.create_revision,
        podRow.theid,
        0,
        podRow.value,
        podRow.value,
      );
      const newId = sql.exec("SELECT last_insert_rowid() AS id").one().id as number;
      broadcastEvent(ctx, sql, key, newId);
      console.log(
        `Evicted pod ${pod.metadata?.namespace}/${pod.metadata?.name} from unreachable node ${nodeName}`,
      );
    } catch (e) {
      console.error("Pod eviction error:", e);
    }
  }
}
