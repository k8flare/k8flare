import { LIST_SQL, INSERT_SQL, GET_SQL } from "./schema.ts";
import { prefixEnd, decodeKineValue, rowToEvent } from "./helpers.ts";
import type { KineRow } from "./helpers.ts";
import { getCurrent, insert, type SqlExec } from "./queries.ts";
import { broadcastEvent, type DurableObjectContext } from "./watch.ts";
import { parseCPU } from "./quantity.ts";

export function runScheduler(ctx: DurableObjectContext, sql: SqlExec, _env: any): void {
  allocatePodCIDRs(ctx, sql);
  scheduleUnboundPods(ctx, sql);
}

/**
 * Whether writing this key is a change the scheduler should react to
 * promptly (an unbound pod, or a node that still needs a PodCIDR) rather
 * than waiting for the periodic safety-net resync.
 */
export function needsSchedulerAttention(key: string, value: ArrayBuffer | string | null): boolean {
  if (!value) return false;
  try {
    if (key.startsWith("/registry/pods/")) {
      const pod = JSON.parse(decodeKineValue(value));
      return !pod.spec?.nodeName;
    }
    if (key.startsWith("/registry/nodes/")) {
      const node = JSON.parse(decodeKineValue(value));
      return !node.spec?.podCIDR;
    }
  } catch {
    return false;
  }
  return false;
}

/** Whether a node is eligible to receive newly-scheduled pods. */
function isNodeSchedulable(node: any): boolean {
  if (node.spec?.unschedulable) return false;
  const conditions = node.status?.conditions;
  if (!Array.isArray(conditions)) return false;
  const ready = conditions.find((c: any) => c.type === "Ready");
  return ready?.status === "True";
}

/** Whether every key in the pod's nodeSelector matches an equal-valued node label. */
function nodeMatchesSelector(node: any, pod: any): boolean {
  const selector = pod.spec?.nodeSelector;
  if (!selector) return true;
  const labels = node.metadata?.labels || {};
  return Object.entries(selector).every(([k, v]) => labels[k] === v);
}

interface HostPortBinding {
  protocol: string;
  hostPort: number;
  hostIP: string;
}

interface NodeCommitment {
  cpu: number;
  hostPorts: HostPortBinding[];
}

/** Sum of a pod's containers' requested CPU, in millicores. */
function podRequestedCPU(pod: any): number {
  return (pod.spec?.containers || []).reduce(
    (sum: number, c: any) => sum + parseCPU(c.resources?.requests?.cpu),
    0,
  );
}

/** Collect a pod's containers' hostPort bindings (container ports with hostPort set). */
function podHostPorts(pod: any): HostPortBinding[] {
  const out: HostPortBinding[] = [];
  for (const c of pod.spec?.containers || []) {
    for (const p of c.ports || []) {
      if (!p.hostPort) continue;
      out.push({
        protocol: p.protocol || "TCP",
        hostPort: p.hostPort,
        hostIP: p.hostIP || "0.0.0.0",
      });
    }
  }
  return out;
}

/**
 * "" and "0.0.0.0" both mean "all interfaces" and conflict with any other
 * hostIP on the same port/protocol, matching real Kubernetes semantics.
 */
function hostIPsConflict(a: string, b: string): boolean {
  if (a === "0.0.0.0" || b === "0.0.0.0") return true;
  return a === b;
}

function hasHostPortConflict(pod: any, committedHostPorts: HostPortBinding[] | undefined): boolean {
  if (!committedHostPorts) return false;
  return podHostPorts(pod).some((mine) =>
    committedHostPorts.some(
      (theirs) =>
        theirs.protocol === mine.protocol &&
        theirs.hostPort === mine.hostPort &&
        hostIPsConflict(theirs.hostIP, mine.hostIP),
    ),
  );
}

/** Whether the node has enough uncommitted allocatable CPU for the pod's requests. */
function nodeHasCapacity(node: any, pod: any, committed: NodeCommitment | undefined): boolean {
  const requested = podRequestedCPU(pod);
  const allocatable = parseCPU(node.status?.allocatable?.cpu);
  return requested + (committed?.cpu || 0) <= allocatable;
}

/**
 * Build a per-node map of CPU and hostPort commitments from all currently
 * scheduled, non-terminal pods (not just unbound ones), so a scheduling
 * pass can account for capacity already in use.
 */
function computeNodeCommitments(podRows: KineRow[]): Map<string, NodeCommitment> {
  const committed = new Map<string, NodeCommitment>();
  for (const row of podRows) {
    if (row.deleted === 1) continue;
    try {
      const value = row.value;
      if (!value) continue;
      const pod = JSON.parse(decodeKineValue(value));

      const nodeName = pod.spec?.nodeName;
      if (!nodeName) continue; // not yet scheduled, nothing to commit

      const phase = pod.status?.phase;
      if (phase === "Succeeded" || phase === "Failed") continue; // terminal pods release resources

      let entry = committed.get(nodeName);
      if (!entry) {
        entry = { cpu: 0, hostPorts: [] };
        committed.set(nodeName, entry);
      }
      entry.cpu += podRequestedCPU(pod);
      entry.hostPorts.push(...podHostPorts(pod));
    } catch (_) {
      // Skip pods that can't be parsed
    }
  }
  return committed;
}

/**
 * Set (or refresh) the pod's PodScheduled status condition. Returns false
 * when nothing would change, so callers can skip re-persisting/re-broadcasting
 * an unchanged condition for a permanently-unschedulable pod on every
 * periodic safety-net tick forever.
 */
function setPodScheduledCondition(
  pod: any,
  status: "True" | "False",
  reason?: string,
  message?: string,
): boolean {
  if (!pod.status) pod.status = {};
  if (!Array.isArray(pod.status.conditions)) pod.status.conditions = [];
  const existing = pod.status.conditions.find((c: any) => c.type === "PodScheduled");
  if (existing && existing.status === status && existing.reason === reason) return false;
  const next = {
    type: "PodScheduled",
    status,
    lastTransitionTime: new Date().toISOString(),
    reason,
    message,
  };
  pod.status.conditions = pod.status.conditions.filter((c: any) => c.type !== "PodScheduled");
  pod.status.conditions.push(next);
  return true;
}

/** Persist an updated pod (nodeName and/or status.conditions) and broadcast the change. */
function persistPod(ctx: DurableObjectContext, sql: SqlExec, row: KineRow, pod: any): void {
  const updatedJson = JSON.stringify(pod);
  const encodedValue = new TextEncoder().encode(updatedJson).buffer;
  const key = row.thename;
  sql.exec(INSERT_SQL, key, 0, 0, row.create_revision, row.theid, 0, encodedValue, row.value);
  const newId = sql.exec("SELECT last_insert_rowid() AS id").one().id as number;
  broadcastEvent(ctx, sql, key, newId);
}

/**
 * Emit a Kubernetes Event recording a scheduling decision. Real kube-scheduler
 * emits a "Scheduled" (Normal) or "FailedScheduling" (Warning) Event
 * alongside setting the pod's status directly; tooling (including the
 * official e2e conformance suite) watches for these Events specifically,
 * not just the PodScheduled condition, so both are needed.
 */
function emitSchedulingEvent(
  ctx: DurableObjectContext,
  sql: SqlExec,
  pod: any,
  type: "Normal" | "Warning",
  reason: string,
  message: string,
): void {
  const namespace = pod.metadata?.namespace;
  const podName = pod.metadata?.name;
  if (!namespace || !podName) return;

  const now = new Date().toISOString();
  const suffix = Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
  const eventName = `${podName}.${suffix}`;
  const key = `/registry/events/${namespace}/${eventName}`;

  const eventObj = {
    kind: "Event",
    apiVersion: "v1",
    metadata: { name: eventName, namespace },
    involvedObject: {
      kind: "Pod",
      namespace,
      name: podName,
      uid: pod.metadata?.uid,
    },
    reason,
    message,
    source: { component: "default-scheduler" },
    firstTimestamp: now,
    lastTimestamp: now,
    count: 1,
    type,
  };

  const encodedValue = new TextEncoder().encode(JSON.stringify(eventObj)).buffer;
  const { rev, event: existing } = getCurrent(sql, key, true);
  const prevRevision = existing ? existing.kv.modRevision : rev;
  const newId = insert(sql, key, true, false, 0, prevRevision, 0, encodedValue, null);
  broadcastEvent(ctx, sql, key, newId);
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

  // Decode nodes, keeping only those ready and schedulable to receive pods.
  // Full node objects (not just names) are kept so the nodeSelector and
  // resource-capacity predicates below can read labels/allocatable.
  const schedulableNodes: any[] = [];
  for (const row of nodeRows) {
    if (row.deleted === 1) continue;
    try {
      const value = row.value;
      if (!value) continue;
      const obj = JSON.parse(decodeKineValue(value));
      if (obj.metadata && obj.metadata.name && isNodeSchedulable(obj)) {
        schedulableNodes.push(obj);
      }
    } catch (_) {}
  }

  if (schedulableNodes.length === 0) return;

  // Per-node CPU/hostPort commitments from all currently scheduled pods.
  // Mutated in place as pods are tentatively bound below, so a burst of
  // new pods (coalesced into this one pass by the debounce mechanism) is
  // accounted against each other, not just against previously-persisted
  // state.
  const committed = computeNodeCommitments(podRows);

  let nodeIndex = 0;

  for (const row of podRows) {
    if (row.deleted === 1) continue;
    try {
      const value = row.value;
      if (!value) continue;
      const pod = JSON.parse(decodeKineValue(value));

      // Skip if already scheduled
      if (pod.spec && pod.spec.nodeName && pod.spec.nodeName !== "") continue;

      if (!pod.spec) pod.spec = {};

      const candidates = schedulableNodes.filter((n) => nodeMatchesSelector(n, pod));
      if (candidates.length === 0) {
        const message = `0/${schedulableNodes.length} nodes are available: node(s) didn't match Pod's node selector.`;
        const changed = setPodScheduledCondition(pod, "False", "Unschedulable", message);
        if (changed) {
          persistPod(ctx, sql, row, pod);
          // Only emit a fresh Event when the condition actually changed, so a
          // permanently-unschedulable pod doesn't accumulate a new Event
          // object on every periodic safety-net tick forever.
          emitSchedulingEvent(ctx, sql, pod, "Warning", "FailedScheduling", message);
        }
        continue;
      }

      const feasible = candidates.filter((n) => {
        const nodeCommitted = committed.get(n.metadata.name);
        return (
          nodeHasCapacity(n, pod, nodeCommitted) &&
          !hasHostPortConflict(pod, nodeCommitted?.hostPorts)
        );
      });
      if (feasible.length === 0) {
        const message = `0/${candidates.length} nodes are available: insufficient resources or hostPort conflict.`;
        const changed = setPodScheduledCondition(pod, "False", "Unschedulable", message);
        if (changed) {
          persistPod(ctx, sql, row, pod);
          emitSchedulingEvent(ctx, sql, pod, "Warning", "FailedScheduling", message);
        }
        continue;
      }

      // Assign to a node (round-robin among feasible candidates)
      const chosen = feasible[nodeIndex % feasible.length];
      nodeIndex++;
      pod.spec.nodeName = chosen.metadata.name;
      setPodScheduledCondition(pod, "True", "Scheduled");
      persistPod(ctx, sql, row, pod);
      emitSchedulingEvent(
        ctx,
        sql,
        pod,
        "Normal",
        "Scheduled",
        `Successfully assigned ${pod.metadata.namespace}/${pod.metadata.name} to ${chosen.metadata.name}`,
      );

      // Commit this pod's own usage before scheduling the next pod in this pass.
      let entry = committed.get(chosen.metadata.name);
      if (!entry) {
        entry = { cpu: 0, hostPorts: [] };
        committed.set(chosen.metadata.name, entry);
      }
      entry.cpu += podRequestedCPU(pod);
      entry.hostPorts.push(...podHostPorts(pod));
    } catch (e) {
      // Skip pods that can't be parsed
      console.error("Scheduler error:", e);
    }
  }
}
