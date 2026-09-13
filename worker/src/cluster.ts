import { DurableObject } from "cloudflare:workers";

// Cluster holds one cluster's state: a kine-style revisioned key-value log
// in SQLite. Every write appends a row; the row id is the revision. A
// bootstrap row makes the first revision 1, as in kine, because a resource
// version of 0 is illegal for a list. Watchers are hibernatable WebSockets
// tagged with the key prefix they asked for.
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
      ctx.storage.sql.exec(
        `INSERT INTO kine (name, deleted, value) SELECT '/k8flare/bootstrap', 0, X'' WHERE NOT EXISTS (SELECT 1 FROM kine)`,
      );
    });
  }

  private revision(): number {
    return this.ctx.storage.sql.exec("SELECT COALESCE(MAX(id), 0) AS rev FROM kine").one().rev as number;
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
    const type = deleted ? "deleted" : prev ? "modified" : "created";
    this.restoreWatchers();
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
          ws.close(1011, "send failed");
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
    };
    const since = Number(url.searchParams.get("since") ?? "0");
    const pair = new WebSocketPair();
    const [client, server] = [pair[0], pair[1]];
    this.ctx.acceptWebSocket(server);
    server.serializeAttachment(watcher);
    this.restoreWatchers();
    this.watchers.set(server, watcher);
    if (url.searchParams.get("initial") === "1") {
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

  // The in-memory watcher map does not survive hibernation; the sockets'
  // attachments do, so it is rebuilt from them whenever it is empty.
  private restoreWatchers(): void {
    if (this.watchers.size > 0) return;
    for (const ws of this.ctx.getWebSockets()) {
      const w = ws.deserializeAttachment() as Watcher | null;
      if (w) this.watchers.set(ws, w);
    }
  }

  async webSocketMessage(): Promise<void> {}

  async webSocketClose(ws: WebSocket): Promise<void> {
    this.watchers.delete(ws);
    ws.close();
  }

  async webSocketError(ws: WebSocket): Promise<void> {
    this.watchers.delete(ws);
    ws.close();
  }
}

interface Watcher {
  prefix: string;
  exact: boolean;
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
