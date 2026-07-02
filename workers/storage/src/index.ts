import { SCHEMA, LIST_SQL } from "./schema.ts";
import { prefixEnd, base64ToArrayBuffer, jsonResponse } from "./helpers.ts";
import { currentRevision, type SqlExec } from "./queries.ts";
import {
  handleWebSocket,
  handleReplay,
  broadcastEvent,
  type DurableObjectContext,
  type WatchHost,
} from "./watch.ts";
import { storeGetCurrent, storeInsert, storeList, deleteNamespaceFacet } from "./store.ts";
import { runScheduler, needsSchedulerAttention } from "./scheduler.ts";
import { allocateClusterIPs, needsServiceIPAttention } from "./serviceip.ts";
import { reconcileEndpoints, needsEndpointsAttention } from "./endpoints.ts";
import { reconcileNodeLifecycle } from "./nodelifecycle.ts";
export { WatchHub } from "./watchhub.ts";

// The scheduler wakes on-demand (see wakeSchedulerSoon) whenever a write
// needs its attention, so this is only a safety net for a missed trigger
// (e.g. a pod that only becomes schedulable once a node's Ready condition
// flips, without any further pod/node write of its own) — not the primary
// mechanism. Keeping it long keeps idle clusters cheap.
const SAFETY_NET_INTERVAL_MS = 60_000;
// How long to wait after a write that needs scheduling before waking the
// alarm, so a burst of writes coalesces into a single scheduler pass.
const DEBOUNCE_MS = 1_000;

const NAMESPACES_PREFIX = "/registry/namespaces/";

/** The namespace name if `key` is exactly a Namespace object's own key, else null. */
function namespaceNameFromKey(key: string): string | null {
  if (!key.startsWith(NAMESPACES_PREFIX)) return null;
  const rest = key.slice(NAMESPACES_PREFIX.length);
  if (rest.length === 0 || rest.includes("/")) return null;
  return rest;
}

/** Cheap local existence check -- no facet round trip needed since a key's
 * envelope (including its `deleted` flag) always lives in the parent, even
 * for namespaced keys whose value lives in a facet (see store.ts). */
function hasLiveKeyUnderPrefix(sql: SqlExec, prefix: string): boolean {
  const q = LIST_SQL("AND mkv.name > ?4") + " LIMIT 1";
  const rows = sql.exec(q, prefix, prefixEnd(prefix), 0, "").toArray();
  return rows.length > 0;
}

/**
 * Whether the safety-net alarm should stay armed. Node lease staleness
 * (nodelifecycle.ts) can only be detected by the ABSENCE of a write, so it
 * needs a periodic check for as long as any Node is registered; Service/
 * Endpoint reconciliation is event-triggered via wakeSchedulerSoon but keeps
 * this as a safety net for missed triggers for as long as any Service
 * exists. A cluster with neither has nothing left for the safety net to do
 * -- it parks (cost invariants #1/#3: no alarm chain on an idle cluster).
 */
function hasPendingSafetyNetWork(sql: SqlExec): boolean {
  return (
    hasLiveKeyUnderPrefix(sql, "/registry/nodes/") ||
    hasLiveKeyUnderPrefix(sql, "/registry/services/")
  );
}

export class Cluster {
  private ctx: DurableObjectContext & {
    storage: { sql: SqlExec; setAlarm(ms: number): void; getAlarm(): Promise<number | null> };
  };
  private env: any;
  private sql: SqlExec;
  private host: WatchHost;
  private initialized: boolean;

  constructor(ctx: any, env: any) {
    this.ctx = ctx;
    this.env = env;
    this.sql = ctx.storage.sql;
    this.host = { env, ctx };
    this.initialized = false;
  }

  private initialize(): void {
    if (this.initialized) return;
    for (const stmt of SCHEMA) {
      this.sql.exec(stmt);
    }
    this.initialized = true;
    // Only arm the safety net if there's already something for it to watch
    // (e.g. this DO woke from hibernation/eviction with live Nodes/Services
    // from before). A genuinely fresh/empty cluster stays parked until its
    // first write arms it via wakeSchedulerSoon.
    if (hasPendingSafetyNetWork(this.sql)) {
      this.ctx.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }
  }

  /**
   * Pull the alarm in to fire soon if it isn't already due sooner, so a
   * write that needs scheduling gets handled promptly instead of waiting
   * for the next safety-net resync. Never pushes the alarm further out.
   */
  private async wakeSchedulerSoon(): Promise<void> {
    const target = Date.now() + DEBOUNCE_MS;
    const current = await this.ctx.storage.getAlarm();
    if (current === null || current > target) {
      this.ctx.storage.setAlarm(target);
    }
  }

  /** Whether any alarm-driven controller needs to react to this write. */
  private needsControllerAttention(key: string, value: ArrayBuffer | string | null): boolean {
    return (
      needsSchedulerAttention(key, value) ||
      needsServiceIPAttention(key, value) ||
      needsEndpointsAttention(key, value)
    );
  }

  async fetch(request: Request): Promise<Response> {
    this.initialize();
    const url = new URL(request.url);
    const path = url.pathname;

    if (request.headers.get("Upgrade") === "websocket" || path === "/watch") {
      return handleWebSocket(this.host, this.sql, request);
    }

    try {
      if (path === "/revision" && request.method === "GET") {
        return jsonResponse({ revision: currentRevision(this.sql) });
      }

      if (path === "/replay" && request.method === "GET") {
        return await handleReplay(this.host, this.sql, request);
      }

      if (path.startsWith("/key/")) {
        const key = "/" + path.slice(5);
        switch (request.method) {
          case "GET":
            return await this.handleGet(key);
          case "PUT":
            return await this.handlePut(key, await request.json());
          case "DELETE":
            return await this.handleDelete(key, parseInt(url.searchParams.get("revision") || "0"));
        }
      }

      if (path.startsWith("/list/") && request.method === "GET") {
        let prefix = "/" + path.slice(6);
        if (!prefix.endsWith("/")) prefix += "/";
        return await this.handleList(
          prefix,
          parseInt(url.searchParams.get("limit") || "0"),
          parseInt(url.searchParams.get("revision") || "0"),
        );
      }

      return new Response("Not found", { status: 404 });
    } catch (e: any) {
      if (e.message && e.message.includes("UNIQUE constraint failed")) {
        return jsonResponse({ error: "key already exists" }, 409);
      }
      return jsonResponse({ error: e.message || String(e) }, 500);
    }
  }

  private async handleGet(key: string): Promise<Response> {
    const { rev, event } = await storeGetCurrent(this.sql, this.host, key, false);
    if (!event || event.delete) return jsonResponse({ revision: rev, kv: null });
    return jsonResponse({ revision: rev, kv: event.kv });
  }

  private async handlePut(key: string, body: any): Promise<Response> {
    const value = body.value ? base64ToArrayBuffer(body.value) : null;
    const lease = body.lease || 0;
    const revision = body.revision || 0;

    if (revision === 0) {
      const { rev, event } = await storeGetCurrent(this.sql, this.host, key, true);
      let prevRevision = rev;
      if (event && !event.delete) return jsonResponse({ error: "key already exists" }, 409);
      if (event) prevRevision = event.kv.modRevision;
      const id = await storeInsert(
        this.sql,
        this.host,
        key,
        true,
        false,
        0,
        prevRevision,
        lease,
        value,
        null,
      );
      await broadcastEvent(this.host, this.sql, key, id);
      if (this.needsControllerAttention(key, value)) await this.wakeSchedulerSoon();
      return jsonResponse({ revision: id }, 201);
    } else {
      const { rev, event } = await storeGetCurrent(this.sql, this.host, key, false);
      if (!event || event.delete) return jsonResponse({ revision: rev, kv: null, updated: false });
      if (event.kv.modRevision !== revision)
        return jsonResponse({ revision: rev, kv: event.kv, updated: false }, 409);
      const oldValue = body.value ? base64ToArrayBuffer(event.kv.value) : null;
      const id = await storeInsert(
        this.sql,
        this.host,
        key,
        false,
        false,
        event.kv.createRevision,
        event.kv.modRevision,
        lease,
        value,
        oldValue,
      );
      const kv = {
        key,
        createRevision: event.kv.createRevision,
        modRevision: id,
        value: body.value,
        lease,
      };
      await broadcastEvent(this.host, this.sql, key, id);
      if (this.needsControllerAttention(key, value)) await this.wakeSchedulerSoon();
      return jsonResponse({ revision: id, kv, updated: true });
    }
  }

  private async handleDelete(key: string, revision: number): Promise<Response> {
    const { rev, event } = await storeGetCurrent(this.sql, this.host, key, true);
    if (!event) return jsonResponse({ revision: rev, kv: null, deleted: true });
    if (event.delete) return jsonResponse({ revision: rev, kv: event.kv, deleted: true });
    if (revision !== 0 && event.kv.modRevision !== revision)
      return jsonResponse({ revision: rev, kv: event.kv, deleted: false });
    const oldValue = event.kv.value ? base64ToArrayBuffer(event.kv.value) : null;
    const id = await storeInsert(
      this.sql,
      this.host,
      key,
      false,
      true,
      event.kv.createRevision,
      event.kv.modRevision,
      0,
      oldValue,
      oldValue,
    );
    await broadcastEvent(this.host, this.sql, key, id);
    if (this.needsControllerAttention(key, oldValue)) await this.wakeSchedulerSoon();

    // Namespace deletion cascade (pkg/apiserver/namespacedelete.go) deletes
    // every namespaced object individually before finally deleting the
    // Namespace object itself -- by the time that last delete lands here,
    // its facet should already be empty. Destroying it outright is a clean,
    // fast GC rather than leaving tombstone rows to accumulate forever.
    const namespaceName = namespaceNameFromKey(key);
    if (namespaceName) deleteNamespaceFacet(this.host, namespaceName);

    return jsonResponse({ revision: id, kv: event.kv, deleted: true });
  }

  private async handleList(prefix: string, limit: number, revision: number): Promise<Response> {
    const {
      revision: rev,
      count,
      kvs,
    } = await storeList(this.sql, this.host, prefix, limit, revision);
    return jsonResponse({ revision: rev, count, kvs });
  }

  async alarm(): Promise<void> {
    this.initialize();
    await runScheduler(this.host, this.sql, this.env);
    await allocateClusterIPs(this.host, this.sql);
    await reconcileEndpoints(this.host, this.sql);
    await reconcileNodeLifecycle(this.host, this.sql);
    // Re-arm the safety net only if there's still live Nodes/Services that
    // need ongoing monitoring; otherwise park (no alarm chain on an idle
    // cluster -- cost invariants #1/#3). A write needing sooner attention
    // than the next safety-net tick pulls this in via wakeSchedulerSoon.
    if (hasPendingSafetyNetWork(this.sql)) {
      this.ctx.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }
  }

  async webSocketMessage(ws: WebSocket, message: string | ArrayBuffer): Promise<void> {
    if (message === "ping") ws.send("pong");
  }

  async webSocketClose(
    ws: WebSocket,
    code: number,
    reason: string,
    wasClean: boolean,
  ): Promise<void> {
    ws.close(code, reason);
  }

  async webSocketError(ws: WebSocket, error: unknown): Promise<void> {
    ws.close(1011, "WebSocket error");
  }
}

// This Worker is never routed to directly -- gateway/apiserver/runtime reach
// Cluster/WatchHub via a script_name Durable Object binding instead. A
// default export is still required so wrangler builds this as a module
// Worker (Durable Objects cannot be exported from a service-worker-format
// Worker).
export default {
  async fetch(): Promise<Response> {
    return new Response("k8flare-storage: not directly routable (Durable Objects only)", {
      status: 404,
    });
  },
};
