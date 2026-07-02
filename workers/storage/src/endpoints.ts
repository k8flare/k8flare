import { decodeKineValue, base64ToArrayBuffer } from "./helpers.ts";
import type { SqlExec } from "./queries.ts";
import { broadcastEvent, type WatchHost } from "./watch.ts";
import { storeGetCurrent, storeInsert, storeListRaw } from "./store.ts";

const SERVICES_PREFIX = "/registry/services/";
const PODS_PREFIX = "/registry/pods/";
const ENDPOINTSLICES_PREFIX = "/registry/endpointslices/";
const ENDPOINTS_PREFIX = "/registry/endpoints/";

/**
 * Whether writing this key is a Service or Pod change that may require
 * recomputing endpoints. Unlike the scheduler trigger this doesn't inspect the
 * value at all — any write under services/ or pods/ is potentially relevant.
 */
export function needsEndpointsAttention(key: string, value: ArrayBuffer | string | null): boolean {
  if (!value) return false;
  try {
    if (key.startsWith(SERVICES_PREFIX) || key.startsWith(PODS_PREFIX)) {
      return true;
    }
  } catch {
    return false;
  }
  return false;
}

/**
 * Recompute EndpointSlice and Endpoints objects for every managed Service,
 * based on the Pods currently matching its selector. Runs from the DO alarm
 * loop, like allocatePodCIDRs / allocateClusterIPs.
 *
 * A Service is managed by this controller when it has a selector and is not an
 * ExternalName Service. Headless Services (clusterIP === "None") with a
 * selector are still managed here — only ClusterIP allocation skips them, not
 * endpoint computation. Services without a selector are owned externally (by
 * the user or another controller) and are left untouched.
 *
 * Services, Pods, EndpointSlices and Endpoints are all namespaced (see
 * keyspace.ts): the Service scan fans out across every namespace facet, each
 * Service's Pod lookup is scoped to its own namespace facet, and every write
 * (upsertKey/deleteKey) is routed to the owning facet.
 */
export async function reconcileEndpoints(host: WatchHost, sql: SqlExec): Promise<void> {
  // includeDeleted so deleted Services can have their endpoints cleaned up.
  const svcRows = await storeListRaw(sql, host, SERVICES_PREFIX, true);

  for (const row of svcRows) {
    try {
      if (row.deleted === 1) {
        // Garbage-collect the endpoints of a deleted Service. Derive the
        // namespace/name from the key since the tombstone value may be stale.
        const nsName = keyNamespaceName(row.thename, SERVICES_PREFIX);
        if (!nsName) continue;
        await deleteKey(host, sql, endpointSliceKey(nsName.namespace, nsName.name));
        await deleteKey(host, sql, endpointsKey(nsName.namespace, nsName.name));
        continue;
      }

      const value = row.value;
      if (!value) continue;
      const svc = JSON.parse(decodeKineValue(value));

      if (svc.spec?.type === "ExternalName") continue;
      const selector = svc.spec?.selector;
      if (!selector || Object.keys(selector).length === 0) continue;

      await reconcileService(host, sql, svc, selector);
    } catch (e) {
      console.error("Endpoints reconciliation error:", e);
    }
  }
}

async function reconcileService(
  host: WatchHost,
  sql: SqlExec,
  svc: any,
  selector: Record<string, string>,
): Promise<void> {
  const namespace: string | undefined = svc.metadata?.namespace;
  const name: string | undefined = svc.metadata?.name;
  if (!namespace || !name) return;

  // Find Pods in the Service's namespace whose labels match every entry in the
  // Service selector. Service selectors are always simple equality maps, so a
  // plain object comparison is correct (no set-based label-selector parsing).
  const podPrefix = `${PODS_PREFIX}${namespace}/`;
  const podRows = await storeListRaw(sql, host, podPrefix, false);

  const pods: any[] = [];
  for (const podRow of podRows) {
    if (podRow.deleted === 1) continue;
    if (!podRow.value) continue;
    const pod = JSON.parse(decodeKineValue(podRow.value));
    const labels = pod.metadata?.labels || {};
    if (Object.entries(selector).every(([k, v]) => labels[k] === v)) {
      pods.push(pod);
    }
  }

  // Resolve each Service port to a single target port number applied to the
  // whole slice (EndpointSlice/Endpoints ports have no per-endpoint override).
  // Assuming homogeneous Pods, a named targetPort is resolved against the first
  // matching Pod that exposes a container port with that name; a Service port
  // with no resolvable target across any matching Pod is omitted entirely.
  const svcPorts: any[] = Array.isArray(svc.spec?.ports) ? svc.spec.ports : [];
  const ports: any[] = [];
  for (const sp of svcPorts) {
    const target = resolveTargetPort(sp, pods);
    if (target === undefined) continue;
    ports.push({ name: sp.name, protocol: sp.protocol || "TCP", port: target });
  }

  // Build one endpoint per matching Pod that already has a podIP.
  const sliceEndpoints: any[] = [];
  const readyAddresses: any[] = [];
  const notReadyAddresses: any[] = [];
  for (const pod of pods) {
    const podIP: string | undefined = pod.status?.podIP;
    if (!podIP) continue; // no address yet — nothing to publish for this Pod
    const ready = (pod.status?.conditions || []).some(
      (c: any) => c.type === "Ready" && c.status === "True",
    );
    const nodeName: string | undefined = pod.spec?.nodeName;
    const targetRef = {
      kind: "Pod",
      name: pod.metadata?.name,
      namespace: pod.metadata?.namespace,
      uid: pod.metadata?.uid,
    };

    const endpoint: any = { addresses: [podIP], conditions: { ready }, targetRef };
    if (nodeName) endpoint.nodeName = nodeName;
    sliceEndpoints.push(endpoint);

    const address: any = { ip: podIP };
    if (nodeName) address.nodeName = nodeName;
    address.targetRef = targetRef;
    if (ready) readyAddresses.push(address);
    else notReadyAddresses.push(address);
  }

  // EndpointSlice (discovery.k8s.io/v1). A single slice per Service is a valid
  // simplification at this scale; upstream hash-suffixes the name to shard a
  // large Service across multiple slices, which isn't needed here.
  const slice = {
    apiVersion: "discovery.k8s.io/v1",
    kind: "EndpointSlice",
    metadata: {
      name,
      namespace,
      // How kube-proxy and clients discover all slices for a Service.
      labels: { "kubernetes.io/service-name": name },
    },
    addressType: "IPv4",
    endpoints: sliceEndpoints,
    ports,
  };
  await upsertKey(host, sql, endpointSliceKey(namespace, name), JSON.stringify(slice));

  // Legacy core/v1 Endpoints, sharing the Service's name/namespace by
  // convention. A single subset covers the homogeneous-Pod case; an empty
  // subsets list is written when no Pods match yet (matches upstream).
  const subsets: any[] = [];
  if (readyAddresses.length > 0 || notReadyAddresses.length > 0) {
    const subset: any = {};
    if (readyAddresses.length > 0) subset.addresses = readyAddresses;
    if (notReadyAddresses.length > 0) subset.notReadyAddresses = notReadyAddresses;
    subset.ports = ports;
    subsets.push(subset);
  }
  const endpoints = {
    apiVersion: "v1",
    kind: "Endpoints",
    metadata: { name, namespace },
    subsets,
  };
  await upsertKey(host, sql, endpointsKey(namespace, name), JSON.stringify(endpoints));
}

/**
 * Resolve a Service port's effective target port number. A numeric targetPort
 * is used directly; an unset targetPort defaults to the Service port; a named
 * targetPort is looked up against the first matching Pod that exposes a
 * container port with that name. Returns undefined if a named port can't be
 * resolved against any matching Pod.
 */
function resolveTargetPort(svcPort: any, pods: any[]): number | undefined {
  const targetPort = svcPort.targetPort;
  if (targetPort === undefined || targetPort === null) return svcPort.port;
  if (typeof targetPort === "number") return targetPort;
  for (const pod of pods) {
    const containers = pod.spec?.containers || [];
    for (const container of containers) {
      for (const p of container.ports || []) {
        if (p.name === targetPort) return p.containerPort;
      }
    }
  }
  return undefined;
}

/**
 * Write `json` to `key` unless the stored value is already identical. Skipping
 * unchanged writes avoids revision churn and watch-event noise on every
 * reconcile pass. includeDeleted=true is used (not false) so a re-create over
 * a delete tombstone chains prev_revision correctly instead of colliding on
 * the (name, prev_revision) unique index.
 */
async function upsertKey(host: WatchHost, sql: SqlExec, key: string, json: string): Promise<void> {
  const { event } = await storeGetCurrent(sql, host, key, true);
  const live = event !== null && !event.delete;
  // event.kv.value is base64 (storeGetCurrent goes through rowToEvent) --
  // decode back to a real ArrayBuffer before comparing/reusing as oldValue,
  // so decodeKineValue's proper-UTF8 (TextDecoder) path is used rather than
  // its raw-binary-string (atob) path.
  const currentValue = live && event!.kv.value ? base64ToArrayBuffer(event!.kv.value) : null;

  if (live && currentValue && decodeKineValue(currentValue) === json) return;

  const encoded = new TextEncoder().encode(json).buffer;
  let newId: number;
  if (live) {
    // Update the live row, chaining prev_revision to its current id.
    newId = await storeInsert(
      sql,
      host,
      key,
      false,
      false,
      event!.kv.createRevision,
      event!.kv.modRevision,
      0,
      encoded,
      currentValue,
    );
  } else if (event) {
    // Re-create over a delete tombstone.
    newId = await storeInsert(
      sql,
      host,
      key,
      true,
      false,
      0,
      event.kv.modRevision,
      0,
      encoded,
      null,
    );
  } else {
    // Brand new key.
    newId = await storeInsert(sql, host, key, true, false, 0, 0, 0, encoded, null);
  }
  await broadcastEvent(host, sql, key, newId);
}

/**
 * Write a delete tombstone for `key` if it currently exists and isn't already
 * deleted, mirroring handleDelete in index.ts.
 */
async function deleteKey(host: WatchHost, sql: SqlExec, key: string): Promise<void> {
  const { event } = await storeGetCurrent(sql, host, key, true);
  if (!event) return; // never existed
  if (event.delete) return; // already a tombstone

  const valueBuf = event.kv.value ? base64ToArrayBuffer(event.kv.value) : null;
  const newId = await storeInsert(
    sql,
    host,
    key,
    false,
    true,
    event.kv.createRevision,
    event.kv.modRevision,
    0,
    valueBuf,
    valueBuf,
  );
  await broadcastEvent(host, sql, key, newId);
}

function endpointSliceKey(namespace: string, name: string): string {
  return `${ENDPOINTSLICES_PREFIX}${namespace}/${name}`;
}

function endpointsKey(namespace: string, name: string): string {
  return `${ENDPOINTS_PREFIX}${namespace}/${name}`;
}

/** Split "<prefix><namespace>/<name>" into its namespace and name parts. */
function keyNamespaceName(key: string, prefix: string): { namespace: string; name: string } | null {
  const rest = key.slice(prefix.length);
  const slash = rest.indexOf("/");
  if (slash < 0) return null;
  return { namespace: rest.slice(0, slash), name: rest.slice(slash + 1) };
}
