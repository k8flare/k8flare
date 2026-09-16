import { DurableObject } from "cloudflare:workers";

// Cluster holds one cluster's state: a kine-style revisioned key-value log
// in SQLite. Every write appends a row; the row id is the revision. A
// bootstrap row makes the first revision 1, as in kine, because a resource
// version of 0 is illegal for a list. Watchers are hibernatable WebSockets
// tagged with the key prefix they asked for.
const RETAINED_REVISIONS = 1000;
const WATCH_LEASE_MS = 360_000;
const NODE_LEASE_PREFIX = "/registry/leases/kube-node-lease/";
const NODE_LEASE_GRACE_MS = 60_000;
const NODE_LEASE_HOLD_MS = 60_000;
const ALARM_SLACK_MS = 1_000;
const RUN_WINDOW_MS = 300_000;
const RUN_RETRY_MS = 15_000;

type WakeTarget = "scheduler" | "controllers";
const wakeTargets: WakeTarget[] = ["scheduler", "controllers"];

function closeQuietly(ws: WebSocket, reason: string): void {
  try {
    ws.close(1000, reason);
  } catch {}
}

export class Cluster extends DurableObject<Env> {
  private watchers = new Map<WebSocket, Watcher>();

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    ctx.blockConcurrencyWhile(async () => {
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS kine (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        deleted INTEGER NOT NULL DEFAULT 0,
        value BLOB
      )`);
      ctx.storage.sql.exec(`CREATE INDEX IF NOT EXISTS kine_name_id ON kine (name, id)`);
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value INTEGER NOT NULL)`);
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS wakes (target TEXT NOT NULL, at INTEGER NOT NULL, hold INTEGER NOT NULL, insured INTEGER NOT NULL DEFAULT 0, reset INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (target, at))`);
      for (const column of ["insured", "reset"]) {
        try {
          ctx.storage.sql.exec(`ALTER TABLE wakes ADD COLUMN ${column} INTEGER NOT NULL DEFAULT 0`);
        } catch {}
      }
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS node_leases (node TEXT PRIMARY KEY, seen INTEGER NOT NULL, expired INTEGER NOT NULL DEFAULT 0)`);
      ctx.storage.sql.exec(
        `INSERT INTO kine (name, deleted, value) SELECT '/k8flare/bootstrap', 0, X'' WHERE NOT EXISTS (SELECT 1 FROM kine)`,
      );
    });
  }

  private revision(): number {
    return this.ctx.storage.sql.exec("SELECT COALESCE(MAX(id), 0) AS rev FROM kine").one().rev as number;
  }

  private compactRevision(): number {
    const rows = this.ctx.storage.sql.exec("SELECT value FROM meta WHERE key = 'compact_revision'").toArray();
    return rows.length === 0 ? 0 : (rows[0].value as number);
  }

  private compactBefore(target: number): void {
    this.ctx.storage.sql.exec(
      "DELETE FROM kine WHERE id <= ? AND (deleted = 1 OR id NOT IN (SELECT MAX(id) FROM kine GROUP BY name))",
      target,
    );
    this.ctx.storage.sql.exec(
      "INSERT INTO meta (key, value) VALUES ('compact_revision', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
      target,
    );
  }

  private current(name: string): KV | null {
    const rows = this.ctx.storage.sql
      .exec("SELECT id, name, deleted, value FROM kine WHERE id = (SELECT MAX(id) FROM kine WHERE name = ?)", name)
      .toArray();
    if (rows.length === 0 || rows[0].deleted) return null;
    return rowToKV(rows[0]);
  }

  private latest(prefix: string, exact: boolean, from: string, limit: number): KV[] {
    return this.ctx.storage.sql
      .exec(
        `SELECT kv.id, kv.name, kv.deleted, kv.value FROM kine AS kv
         JOIN (SELECT MAX(id) AS id FROM kine WHERE name >= ? AND name < ? GROUP BY name) AS latest ON latest.id = kv.id
         WHERE kv.deleted = 0 ORDER BY kv.name ASC LIMIT ?`,
        from > prefix ? from : prefix,
        rangeEnd(prefix, exact),
        limit,
      )
      .toArray()
      .map(rowToKV);
  }

  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/watch" && request.headers.get("Upgrade") === "websocket") {
      return this.watch(url);
    }
    switch (`${request.method} ${url.pathname}`) {
      case "GET /revision":
        return Response.json({ revision: this.revision() });
      case "GET /stats":
        return Response.json({
          revision: this.revision(),
          compactRevision: this.compactRevision(),
          rows: this.ctx.storage.sql.exec("SELECT COUNT(*) AS n FROM kine").one().n as number,
          watchers: this.ctx.getWebSockets().length,
        });
      case "GET /kv": {
        const kv = this.current(url.searchParams.get("key") ?? "");
        return Response.json({ revision: this.revision(), kv: kv && encodeKV(kv) });
      }
      case "PUT /kv": {
        const body = (await request.json()) as { key: string; value: string; revision: number };
        const cur = this.current(body.key);
        if (body.revision === 0 && cur) return conflict(this.revision(), "exists");
        if (body.revision !== 0 && !cur) return Response.json({ revision: this.revision() }, { status: 404 });
        if (body.revision !== 0 && cur!.modRevision !== body.revision) return conflict(this.revision(), "conflict");
        const rev = this.insert(body.key, 0, fromBase64(body.value), cur);
        return Response.json({ revision: rev }, { status: body.revision === 0 ? 201 : 200 });
      }
      case "DELETE /kv": {
        const body = (await request.json()) as { key: string; revision: number };
        const cur = this.current(body.key);
        if (!cur) return Response.json({ revision: this.revision() }, { status: 404 });
        if (body.revision !== 0 && cur.modRevision !== body.revision) return conflict(this.revision(), "conflict");
        return Response.json({ revision: this.insert(body.key, 1, cur.value, cur) });
      }
      case "POST /wake": {
        const body = (await request.json()) as { target: WakeTarget; delayMs: number; holdMs?: number; insured?: boolean; reset?: boolean };
        if (!wakeTargets.includes(body.target)) return new Response("unknown target", { status: 400 });
        await this.scheduleWake(body.target, Date.now() + body.delayMs, body.holdMs ?? 0, body.insured === true, body.reset === true);
        return Response.json({ ok: true });
      }
      case "POST /wake/settle": {
        const body = (await request.json()) as { target: WakeTarget };
        if (!wakeTargets.includes(body.target)) return new Response("unknown target", { status: 400 });
        this.ctx.storage.sql.exec("DELETE FROM wakes WHERE target = ? AND insured = 1", body.target);
        await this.rearm();
        return Response.json({ ok: true });
      }
      case "GET /list": {
        const prefix = url.searchParams.get("prefix") ?? "";
        const limit = Number(url.searchParams.get("limit") ?? "0");
        const kvs = this.latest(prefix, false, url.searchParams.get("from") ?? prefix, limit > 0 ? limit + 1 : -1);
        const more = limit > 0 && kvs.length > limit;
        return Response.json({ revision: this.revision(), kvs: (more ? kvs.slice(0, limit) : kvs).map(encodeKV), more });
      }
    }
    return new Response("not found", { status: 404 });
  }

  private insert(name: string, deleted: number, value: Uint8Array, prev: KV | null): number {
    const rev = this.ctx.storage.sql
      .exec(
        "INSERT INTO kine (name, deleted, value) VALUES (?, ?, ?) RETURNING id",
        name,
        deleted,
        value.buffer.slice(value.byteOffset, value.byteOffset + value.byteLength),
      )
      .one().id as number;
    if (rev - this.compactRevision() >= RETAINED_REVISIONS) this.compactBefore(rev - RETAINED_REVISIONS);
    if (name.startsWith(NODE_LEASE_PREFIX)) this.ctx.waitUntil(this.noteNodeLease(name.slice(NODE_LEASE_PREFIX.length), deleted === 1));
    const type = deleted ? "deleted" : prev ? "modified" : "created";
    this.restoreWatchers();
    this.expireWatchers();
    const targets = [...this.watchers].filter(([, w]) => (w.exact ? name === w.prefix : name.startsWith(w.prefix)));
    if (targets.length > 0) {
      const msg = JSON.stringify({
        rev,
        type,
        key: name,
        value: toBase64(value),
        prev: type === "modified" ? toBase64(prev!.value) : "",
      });
      for (const [ws] of targets) {
        try {
          ws.send(msg);
        } catch {
          closeQuietly(ws, "send failed");
        }
      }
    }
    return rev;
  }

  // A watcher is first caught up from `since`, or given the current state
  // when `initial` is set (followed by a snapshot-end marker), then receives
  // every later write.
  private watch(url: URL): Response {
    const watcher: Watcher = {
      prefix: url.searchParams.get("prefix") ?? "/",
      exact: url.searchParams.get("exact") === "1",
      openedAt: Date.now(),
    };
    const since = Number(url.searchParams.get("since") ?? "0");
    const initial = url.searchParams.get("initial") === "1";
    const pair = new WebSocketPair();
    const [client, server] = [pair[0], pair[1]];
    const compacted = this.compactRevision();
    if (!initial && since > 0 && since < compacted) {
      server.accept();
      server.send(JSON.stringify({ rev: compacted, type: "compacted", key: "", value: "", prev: "" }));
      server.close(1000, "compacted");
      return new Response(null, { status: 101, webSocket: client });
    }
    this.ctx.acceptWebSocket(server);
    server.serializeAttachment(watcher);
    this.restoreWatchers();
    this.expireWatchers();
    this.watchers.set(server, watcher);
    if (initial) {
      for (const kv of this.latest(watcher.prefix, watcher.exact, watcher.prefix, -1)) {
        server.send(JSON.stringify({ rev: kv.modRevision, type: "created", key: kv.key, value: toBase64(kv.value), prev: "" }));
      }
      server.send(JSON.stringify({ rev: this.revision(), type: "snapshot-end", key: "", value: "", prev: "" }));
    } else {
      const rows = this.ctx.storage.sql
        .exec(
          `SELECT id, name, deleted, value, prev FROM (
             SELECT id, name, deleted, value, LAG(value) OVER (PARTITION BY name ORDER BY id) AS prev
             FROM kine WHERE name >= ? AND name < ?)
           WHERE id > ? ORDER BY id ASC`,
          watcher.prefix,
          rangeEnd(watcher.prefix, watcher.exact),
          since,
        )
        .toArray();
      for (const r of rows) {
        const kv = rowToKV(r);
        const type = r.deleted ? "deleted" : r.prev ? "modified" : "created";
        const prev = type === "modified" ? toBase64(new Uint8Array(r.prev as ArrayBuffer)) : "";
        server.send(JSON.stringify({ rev: kv.modRevision, type, key: kv.key, value: toBase64(kv.value), prev }));
      }
    }
    return new Response(null, { status: 101, webSocket: client });
  }

  private expireWatchers(): void {
    const cutoff = Date.now() - WATCH_LEASE_MS;
    for (const [ws, w] of this.watchers) {
      if (w.openedAt < cutoff) {
        this.watchers.delete(ws);
        closeQuietly(ws, "lease");
      }
    }
  }

  // The in-memory watcher map does not survive hibernation; the sockets'
  // attachments do, so it is rebuilt from them whenever it is empty.
  private restoreWatchers(): void {
    if (this.watchers.size > 0) return;
    for (const ws of this.ctx.getWebSockets()) {
      const w = ws.deserializeAttachment() as Watcher | null;
      if (w) this.watchers.set(ws, w);
    }
  }

  private async noteNodeLease(node: string, deleted: boolean): Promise<void> {
    if (deleted) {
      this.ctx.storage.sql.exec("DELETE FROM node_leases WHERE node = ?", node);
    } else {
      this.ctx.storage.sql.exec(
        "INSERT INTO node_leases (node, seen, expired) VALUES (?, ?, 0) ON CONFLICT(node) DO UPDATE SET seen = excluded.seen, expired = 0",
        node,
        Date.now(),
      );
    }
    await this.rearm();
  }

  private async scheduleWake(target: WakeTarget, at: number, holdMs: number, insured = false, reset = false): Promise<void> {
    const covered = this.ctx.storage.sql
      .exec("SELECT 1 FROM wakes WHERE target = ? AND at > ? AND at <= ? AND hold >= ? AND reset >= ? LIMIT 1", target, Date.now(), at, holdMs, reset ? 1 : 0)
      .toArray();
    if (covered.length > 0) return;
    this.ctx.storage.sql.exec(
      "INSERT INTO wakes (target, at, hold, insured, reset) VALUES (?, ?, ?, ?, ?) ON CONFLICT(target, at) DO UPDATE SET hold = MAX(wakes.hold, excluded.hold), insured = MIN(wakes.insured, excluded.insured), reset = MAX(wakes.reset, excluded.reset)",
      target,
      at,
      holdMs,
      insured ? 1 : 0,
      reset ? 1 : 0,
    );
    console.log(`wake ${target} in ${Math.max(0, at - Date.now())}ms${insured ? " (insured)" : ""}`);
    await this.rearm();
  }

  private nextLeaseExpiry(): number | null {
    const row = this.ctx.storage.sql.exec("SELECT MIN(seen) AS seen FROM node_leases WHERE expired = 0").one();
    return row.seen === null ? null : (row.seen as number) + NODE_LEASE_GRACE_MS;
  }

  private async rearm(): Promise<void> {
    const times = this.ctx.storage.sql.exec("SELECT at FROM wakes").toArray().map((r) => r.at as number);
    const expiry = this.nextLeaseExpiry();
    if (expiry !== null) times.push(expiry);
    if (times.length === 0) {
      await this.ctx.storage.deleteAlarm();
      return;
    }
    await this.ctx.storage.setAlarm(Math.min(...times));
  }

  async alarm(): Promise<void> {
    const now = Date.now() + ALARM_SLACK_MS;
    const due = new Map<WakeTarget, { hold: number; reset: boolean }>();
    for (const row of this.ctx.storage.sql.exec("SELECT target, MAX(hold) AS hold, MAX(reset) AS reset FROM wakes WHERE at <= ? GROUP BY target", now).toArray()) {
      due.set(row.target as WakeTarget, { hold: row.hold as number, reset: (row.reset as number) === 1 });
    }
    this.ctx.storage.sql.exec("DELETE FROM wakes WHERE at <= ?", now);
    const stale = this.ctx.storage.sql
      .exec("SELECT node FROM node_leases WHERE expired = 0 AND seen + ? <= ?", NODE_LEASE_GRACE_MS, now)
      .toArray();
    if (stale.length > 0) {
      for (const row of stale) this.ctx.storage.sql.exec("UPDATE node_leases SET expired = 1 WHERE node = ?", row.node);
      console.log(`lease expired: ${stale.map((r) => r.node).join(",")}`);
      const current = due.get("controllers");
      due.set("controllers", { hold: Math.max(current?.hold ?? 0, NODE_LEASE_HOLD_MS), reset: current?.reset ?? false });
    }
    await this.rearm();
    if (due.size > 0) console.log(`wake ${[...due.keys()].join(",")}`);
    await Promise.all([...due].map(([target, d]) => this.runTarget(target, d.hold, d.reset)));
  }

  private async runTarget(target: WakeTarget, holdMs: number, reset: boolean): Promise<void> {
    const entrypoint = target === "scheduler" ? this.env.SCHEDULER : this.env.CONTROLLERS;
    try {
      await entrypoint.run(RUN_WINDOW_MS, holdMs, reset);
    } catch (err) {
      console.error(`wake ${target}:`, err);
      await this.scheduleWake(target, Date.now() + RUN_RETRY_MS, holdMs);
    }
  }

  async webSocketMessage(): Promise<void> {}

  async webSocketClose(ws: WebSocket): Promise<void> {
    this.watchers.delete(ws);
    closeQuietly(ws, "peer");
  }

  async webSocketError(ws: WebSocket): Promise<void> {
    this.watchers.delete(ws);
    closeQuietly(ws, "error");
  }
}

interface Watcher {
  prefix: string;
  exact: boolean;
  openedAt: number;
}

interface KV {
  key: string;
  value: Uint8Array;
  modRevision: number;
}

function rowToKV(row: Record<string, SqlStorageValue>): KV {
  return { key: row.name as string, value: new Uint8Array(row.value as ArrayBuffer), modRevision: row.id as number };
}

function encodeKV(kv: KV) {
  return { key: kv.key, value: toBase64(kv.value), modRevision: kv.modRevision };
}

// rangeEnd is the exclusive upper bound of the key range for a prefix, or
// for one exact key.
function rangeEnd(prefix: string, exact: boolean): string {
  if (exact) return prefix + "\u0000";
  return prefix.slice(0, -1) + String.fromCharCode(prefix.charCodeAt(prefix.length - 1) + 1);
}

function toBase64(bytes: Uint8Array): string {
  let bin = "";
  for (let i = 0; i < bytes.length; i += 8192) {
    bin += String.fromCharCode.apply(null, bytes.subarray(i, i + 8192) as unknown as number[]);
  }
  return btoa(bin);
}

function fromBase64(s: string): Uint8Array {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function conflict(revision: number, error: string): Response {
  return Response.json({ revision, error }, { status: 409 });
}
