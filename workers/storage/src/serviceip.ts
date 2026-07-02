import { GET_SQL, INSERT_SQL } from "./schema.ts";
import { decodeKineValue } from "./helpers.ts";
import type { SqlExec } from "./queries.ts";
import { broadcastEvent, type WatchHost } from "./watch.ts";
import { storeInsert, storeListRaw } from "./store.ts";

// 10.43.0.1 is reserved for the future "kubernetes.default" Service (the
// standard first address in the service range) and 10.43.0.10 for
// "kube-dns" (see docs/general-purpose-k8s-plan.md Phase 4) -- neither is
// provisioned yet, but the counter starts past both so a real allocation
// never collides with them once they are.
const FIRST_ALLOCATABLE_INDEX = 11;

/**
 * Whether writing this key is a Service that needs a ClusterIP allocated.
 */
export function needsServiceIPAttention(key: string, value: ArrayBuffer | string | null): boolean {
  if (!value) return false;
  try {
    if (key.startsWith("/registry/services/")) {
      const svc = JSON.parse(decodeKineValue(value));
      return serviceNeedsClusterIP(svc);
    }
  } catch {
    return false;
  }
  return false;
}

function serviceNeedsClusterIP(svc: any): boolean {
  if (svc.spec?.type === "ExternalName") return false;
  const clusterIP = svc.spec?.clusterIP;
  return !clusterIP || clusterIP === "";
}

/**
 * Allocate ClusterIPs to Services that don't have one.
 * Skips headless Services (spec.clusterIP === "None") and ExternalName
 * Services, neither of which get a ClusterIP. Addresses come from
 * 10.43.0.0/16, the ServiceIPRange declared in supervisor.go.
 *
 * Services are namespaced (see keyspace.ts), so the scan fans out across
 * every namespace facet via storeListRaw and each write is routed to the
 * owning facet via storeInsert.
 */
export async function allocateClusterIPs(host: WatchHost, sql: SqlExec): Promise<void> {
  const svcPrefix = "/registry/services/";
  const svcRows = await storeListRaw(sql, host, svcPrefix, false);

  for (const row of svcRows) {
    if (row.deleted === 1) continue;
    try {
      const value = row.value;
      if (!value) continue;
      const svc = JSON.parse(decodeKineValue(value));

      if (svc.spec?.clusterIP === "None") continue; // headless, no IP by design
      if (!serviceNeedsClusterIP(svc)) continue;

      const index = nextServiceIPIndex(sql);
      if (index > 65535) {
        console.error("ClusterIP allocation exhausted (10.43.0.0/16 full)");
        continue;
      }
      const clusterIP = serviceIPFromIndex(index);

      if (!svc.spec) svc.spec = {};
      svc.spec.clusterIP = clusterIP;
      svc.spec.clusterIPs = [clusterIP];

      const updatedJson = JSON.stringify(svc);
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
      console.log(
        `Allocated ClusterIP ${clusterIP} to ${svc.metadata?.namespace}/${svc.metadata?.name}`,
      );
    } catch (e) {
      console.error("ClusterIP allocation error:", e);
    }
  }
}

function serviceIPFromIndex(index: number): string {
  const high = (index >> 8) & 0xff;
  const low = index & 0xff;
  return `10.43.${high}.${low}`;
}

// The serviceip-counter key is a cluster-internal key (not under a
// namespace resource), so this stays on direct sql access -- see
// keyspace.ts's CLUSTER_SCOPED_RESOURCES ("_internal").
export function nextServiceIPIndex(sql: SqlExec): number {
  const counterKey = "/registry/_internal/serviceip-counter";
  const q = GET_SQL(false);
  const rows = sql.exec(q, counterKey).toArray();

  let index = FIRST_ALLOCATABLE_INDEX;
  if (rows.length > 0 && !rows[0].deleted) {
    const raw = rows[0].value;
    const val = decodeKineValue(raw!);
    index = parseInt(val) || FIRST_ALLOCATABLE_INDEX;
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
