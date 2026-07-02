import { decodeKineValue, base64ToArrayBuffer } from "./helpers.ts";
import type { SqlExec } from "./queries.ts";
import { broadcastEvent, type WatchHost } from "./watch.ts";
import { storeGetCurrent, storeInsert, storeListRaw } from "./store.ts";

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
 *
 * Nodes are cluster-scoped (direct sql, unchanged); Leases and Pods are
 * namespaced (see keyspace.ts) -- the Lease lookup is routed to the
 * "kube-node-lease" namespace facet and the Pod eviction scan fans out
 * across every namespace facet via storeListRaw/storeInsert.
 */
export async function reconcileNodeLifecycle(host: WatchHost, sql: SqlExec): Promise<void> {
  const nodeRows = await storeListRaw(sql, host, NODES_PREFIX, false);

  for (const row of nodeRows) {
    if (row.deleted === 1) continue;
    try {
      const value = row.value;
      if (!value) continue;
      const node = JSON.parse(decodeKineValue(value));
      const nodeName: string | undefined = node.metadata?.name;
      if (!nodeName) continue;

      const leaseKey = `${LEASE_PREFIX}${nodeName}`;
      const { event: leaseEvent } = await storeGetCurrent(sql, host, leaseKey, false);
      if (!leaseEvent || leaseEvent.delete || !leaseEvent.kv.value) {
        continue; // no lease yet -- node hasn't finished registering
      }
      const leaseValue = base64ToArrayBuffer(leaseEvent.kv.value);
      if (!leaseValue) continue;
      const lease = JSON.parse(decodeKineValue(leaseValue));
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
        await upsertNode(host, sql, row, node);
      }

      if (staleMs > POD_EVICTION_MS) {
        await evictPodsOnNode(host, sql, nodeName);
      }
    } catch (e) {
      console.error("Node lifecycle reconciliation error:", e);
    }
  }
}

async function upsertNode(host: WatchHost, sql: SqlExec, row: any, node: any): Promise<void> {
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
    row.value,
  );
  await broadcastEvent(host, sql, key, newId);
  console.log(`Node ${node.metadata?.name} marked unreachable (Unknown + tainted)`);
}

async function evictPodsOnNode(host: WatchHost, sql: SqlExec, nodeName: string): Promise<void> {
  const podRows = await storeListRaw(sql, host, PODS_PREFIX, false);

  for (const podRow of podRows) {
    if (podRow.deleted === 1) continue;
    try {
      if (!podRow.value) continue;
      const pod = JSON.parse(decodeKineValue(podRow.value));
      if (pod.spec?.nodeName !== nodeName) continue;

      const key = podRow.thename;
      // storeListRaw always yields a real ArrayBuffer here (see
      // store.ts's facetRawToKineRow) -- KineRow's wider `string` case
      // models a raw-driver edge case this path doesn't take.
      const evictValue = podRow.value as ArrayBuffer | null;
      const newId = await storeInsert(
        sql,
        host,
        key,
        false,
        true,
        podRow.create_revision,
        podRow.theid,
        0,
        evictValue,
        podRow.value,
      );
      await broadcastEvent(host, sql, key, newId);
      console.log(
        `Evicted pod ${pod.metadata?.namespace}/${pod.metadata?.name} from unreachable node ${nodeName}`,
      );
    } catch (e) {
      console.error("Pod eviction error:", e);
    }
  }
}
