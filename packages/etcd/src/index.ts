import { SCHEMA } from "./schema.ts";
import { prefixEnd, base64ToArrayBuffer, jsonResponse, rowToEvent } from "./helpers.ts";
import { LIST_SQL } from "./schema.ts";
import { currentRevision, getCurrent, insert, type SqlExec } from "./queries.ts";
import { handleWebSocket, broadcastEvent, type DurableObjectContext } from "./watch.ts";
import { runScheduler, needsSchedulerAttention } from "./scheduler.ts";
import { allocateClusterIPs, needsServiceIPAttention } from "./serviceip.ts";
import { reconcileEndpoints, needsEndpointsAttention } from "./endpoints.ts";
import { reconcileNodeLifecycle } from "./nodelifecycle.ts";

// The scheduler wakes on-demand (see wakeSchedulerSoon) whenever a write
// needs its attention, so this is only a safety net for a missed trigger
// (e.g. a pod that only becomes schedulable once a node's Ready condition
// flips, without any further pod/node write of its own) — not the primary
// mechanism. Keeping it long keeps idle clusters cheap.
const SAFETY_NET_INTERVAL_MS = 60_000;
// How long to wait after a write that needs scheduling before waking the
// alarm, so a burst of writes coalesces into a single scheduler pass.
const DEBOUNCE_MS = 1_000;

export class Etcd {
  private ctx: DurableObjectContext & {
    storage: { sql: SqlExec; setAlarm(ms: number): void; getAlarm(): Promise<number | null> };
  };
  private env: any;
  private sql: SqlExec;
  private initialized: boolean;

  constructor(ctx: any, env: any) {
    this.ctx = ctx;
    this.env = env;
    this.sql = ctx.storage.sql;
    this.initialized = false;
  }

  private initialize(): void {
    if (this.initialized) return;
    for (const stmt of SCHEMA) {
      this.sql.exec(stmt);
    }
    this.initialized = true;
    // Start the safety-net resync loop.
    this.ctx.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
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
      return handleWebSocket(this.ctx, this.sql, request);
    }

    try {
      if (path === "/revision" && request.method === "GET") {
        return jsonResponse({ revision: currentRevision(this.sql) });
      }

      if (path.startsWith("/key/")) {
        const key = "/" + path.slice(5);
        switch (request.method) {
          case "GET":
            return this.handleGet(key);
          case "PUT":
            return await this.handlePut(key, await request.json());
          case "DELETE":
            return this.handleDelete(key, parseInt(url.searchParams.get("revision") || "0"));
        }
      }

      if (path.startsWith("/list/") && request.method === "GET") {
        let prefix = "/" + path.slice(6);
        if (!prefix.endsWith("/")) prefix += "/";
        return this.handleList(
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

  private handleGet(key: string): Response {
    const { rev, event } = getCurrent(this.sql, key, false);
    if (!event || event.delete) return jsonResponse({ revision: rev, kv: null });
    return jsonResponse({ revision: rev, kv: event.kv });
  }

  private async handlePut(key: string, body: any): Promise<Response> {
    const value = body.value ? base64ToArrayBuffer(body.value) : null;
    const lease = body.lease || 0;
    const revision = body.revision || 0;

    if (revision === 0) {
      const { rev, event } = getCurrent(this.sql, key, true);
      let prevRevision = rev;
      if (event && !event.delete) return jsonResponse({ error: "key already exists" }, 409);
      if (event) prevRevision = event.kv.modRevision;
      const id = insert(this.sql, key, true, false, 0, prevRevision, lease, value, null);
      broadcastEvent(this.ctx, this.sql, key, id);
      if (this.needsControllerAttention(key, value)) await this.wakeSchedulerSoon();
      return jsonResponse({ revision: id }, 201);
    } else {
      const { rev, event } = getCurrent(this.sql, key, false);
      if (!event || event.delete) return jsonResponse({ revision: rev, kv: null, updated: false });
      if (event.kv.modRevision !== revision)
        return jsonResponse({ revision: rev, kv: event.kv, updated: false }, 409);
      const oldValue = body.value ? base64ToArrayBuffer(event.kv.value) : null;
      const id = insert(
        this.sql,
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
      broadcastEvent(this.ctx, this.sql, key, id);
      if (this.needsControllerAttention(key, value)) await this.wakeSchedulerSoon();
      return jsonResponse({ revision: id, kv, updated: true });
    }
  }

  private async handleDelete(key: string, revision: number): Promise<Response> {
    const { rev, event } = getCurrent(this.sql, key, true);
    if (!event) return jsonResponse({ revision: rev, kv: null, deleted: true });
    if (event.delete) return jsonResponse({ revision: rev, kv: event.kv, deleted: true });
    if (revision !== 0 && event.kv.modRevision !== revision)
      return jsonResponse({ revision: rev, kv: event.kv, deleted: false });
    const oldValue = event.kv.value ? base64ToArrayBuffer(event.kv.value) : null;
    const id = insert(
      this.sql,
      key,
      false,
      true,
      event.kv.createRevision,
      event.kv.modRevision,
      0,
      oldValue,
      oldValue,
    );
    broadcastEvent(this.ctx, this.sql, key, id);
    if (this.needsControllerAttention(key, oldValue)) await this.wakeSchedulerSoon();
    return jsonResponse({ revision: id, kv: event.kv, deleted: true });
  }

  private handleList(prefix: string, limit: number, revision: number): Response {
    let rows;
    const end = prefixEnd(prefix);

    if (revision === 0) {
      const q = LIST_SQL("AND mkv.name > ?4") + (limit > 0 ? ` LIMIT ${limit}` : "");
      rows = this.sql.exec(q, prefix, end, 0, "").toArray();
    } else {
      const q = LIST_SQL("AND mkv.id <= ?4") + (limit > 0 ? ` LIMIT ${limit}` : "");
      rows = this.sql.exec(q, prefix, end, 0, revision).toArray();
    }

    const rev = rows.length > 0 ? rows[0].current_rev : currentRevision(this.sql);
    const kvs = rows.map((r) => rowToEvent(r).kv);
    return jsonResponse({ revision: rev, count: kvs.length, kvs });
  }

  async alarm(): Promise<void> {
    this.initialize();
    runScheduler(this.ctx, this.sql, this.env);
    allocateClusterIPs(this.ctx, this.sql);
    reconcileEndpoints(this.ctx, this.sql);
    reconcileNodeLifecycle(this.ctx, this.sql);
    // Re-arm the safety-net resync; a write needing sooner attention will
    // pull this in via wakeSchedulerSoon.
    this.ctx.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
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
