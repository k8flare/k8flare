import { DurableObject } from "cloudflare:workers";

// Cluster holds one cluster's state: a kine-style revisioned key-value log
// in SQLite. Every write appends a row; the row id is the revision.
export class Cluster extends DurableObject<Env> {
  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    ctx.blockConcurrencyWhile(async () => {
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS kine (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        deleted INTEGER NOT NULL DEFAULT 0,
        create_revision INTEGER NOT NULL,
        value BLOB
      )`);
      ctx.storage.sql.exec(`CREATE INDEX IF NOT EXISTS kine_name_id ON kine (name, id)`);
    });
  }

  private revision(): number {
    const row = this.ctx.storage.sql.exec("SELECT COALESCE(MAX(id), 0) AS rev FROM kine").one();
    return row.rev as number;
  }

  private current(name: string): KV | null {
    const rows = this.ctx.storage.sql
      .exec(
        "SELECT id, name, deleted, create_revision, value FROM kine WHERE id = (SELECT MAX(id) FROM kine WHERE name = ?)",
        name,
      )
      .toArray();
    if (rows.length === 0 || rows[0].deleted) return null;
    return rowToKV(rows[0]);
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
        const value = Uint8Array.from(atob(body.value), (c) => c.charCodeAt(0));
        const rev = this.insert(body.key, 0, cur ? cur.createRevision : 0, value);
        return Response.json({ revision: rev }, { status: body.revision === 0 ? 201 : 200 });
      }
      case "DELETE /kv": {
        const body = (await request.json()) as { key: string; revision: number };
        const cur = this.current(body.key);
        if (!cur) return Response.json({ revision: this.revision() }, { status: 404 });
        if (body.revision !== 0 && cur.modRevision !== body.revision) return conflict(this.revision(), "conflict");
        const rev = this.insert(body.key, 1, cur.createRevision, cur.value);
        return Response.json({ revision: rev });
      }
      case "GET /list": {
        const prefix = url.searchParams.get("prefix") ?? "";
        const from = url.searchParams.get("from") ?? prefix;
        const limit = Number(url.searchParams.get("limit") ?? "0");
        const end = prefixEnd(prefix);
        const rows = this.ctx.storage.sql
          .exec(
            `SELECT kv.id, kv.name, kv.deleted, kv.create_revision, kv.value FROM kine AS kv
             JOIN (SELECT MAX(id) AS id FROM kine WHERE name >= ? AND name < ? GROUP BY name) AS latest ON latest.id = kv.id
             WHERE kv.deleted = 0 ORDER BY kv.name ASC ${limit > 0 ? "LIMIT ?" : ""}`,
            ...(limit > 0 ? [from > prefix ? from : prefix, end, limit + 1] : [from > prefix ? from : prefix, end]),
          )
          .toArray();
        const more = limit > 0 && rows.length > limit;
        const kvs = (more ? rows.slice(0, limit) : rows).map((r) => encodeKV(rowToKV(r)));
        return Response.json({ revision: this.revision(), kvs, more });
      }
    }
    return new Response("not found", { status: 404 });
  }

  private insert(name: string, deleted: number, createRevision: number, value: Uint8Array): number {
    const sql = this.ctx.storage.sql;
    const prev = this.current(name);
    sql.exec(
      "INSERT INTO kine (name, deleted, create_revision, value) VALUES (?, ?, ?, ?)",
      name,
      deleted,
      createRevision,
      value.buffer.slice(value.byteOffset, value.byteOffset + value.byteLength),
    );
    const rev = sql.exec("SELECT last_insert_rowid() AS id").one().id as number;
    if (createRevision === 0) {
      sql.exec("UPDATE kine SET create_revision = ? WHERE id = ?", rev, rev);
    }
    const type = deleted ? "deleted" : createRevision === 0 ? "created" : "modified";
    this.broadcast({ rev, type, key: name, value: b64(value), prev: prev && type === "modified" ? b64(prev.value) : "" });
    return rev;
  }

  // Watchers are hibernatable WebSockets tagged with the key prefix they
  // asked for. Each is first caught up from `since` (or given the current
  // state when `initial` is set), then receives every later write.
  private watch(url: URL): Response {
    const prefix = url.searchParams.get("prefix") ?? "/";
    const exact = url.searchParams.get("exact") === "1";
    const since = Number(url.searchParams.get("since") ?? "0");
    const initial = url.searchParams.get("initial") === "1";
    const pair = new WebSocketPair();
    const [client, server] = [pair[0], pair[1]];
    this.ctx.acceptWebSocket(server);
    server.serializeAttachment({ prefix, exact });
    const sql = this.ctx.storage.sql;
    const end = prefixEnd(prefix);
    if (initial) {
      const rows = sql
        .exec(
          `SELECT kv.id, kv.name, kv.deleted, kv.create_revision, kv.value FROM kine AS kv
           JOIN (SELECT MAX(id) AS id FROM kine WHERE name >= ? AND name < ? GROUP BY name) AS latest ON latest.id = kv.id
           WHERE kv.deleted = 0 ORDER BY kv.id ASC`,
          exact ? prefix : prefix,
          exact ? prefix + "\u0000" : end,
        )
        .toArray();
      for (const r of rows) {
        const kv = rowToKV(r);
        server.send(JSON.stringify({ rev: kv.modRevision, type: "created", key: kv.key, value: b64(kv.value), prev: "" }));
      }
      server.send(JSON.stringify({ rev: this.revision(), type: "snapshot-end", key: "", value: "", prev: "" }));
    } else {
      const rows = sql
        .exec(
          "SELECT id, name, deleted, create_revision, value FROM kine WHERE id > ? AND name >= ? AND name < ? ORDER BY id ASC",
          since,
          prefix,
          exact ? prefix + "\u0000" : end,
        )
        .toArray();
      for (const r of rows) {
        const kv = rowToKV(r);
        const type = r.deleted ? "deleted" : kv.createRevision === kv.modRevision ? "created" : "modified";
        let prev = "";
        if (type === "modified") {
          const p = sql
            .exec("SELECT value FROM kine WHERE name = ? AND id < ? ORDER BY id DESC LIMIT 1", kv.key, kv.modRevision)
            .toArray();
          if (p.length) prev = b64(new Uint8Array(p[0].value as ArrayBuffer));
        }
        server.send(JSON.stringify({ rev: kv.modRevision, type, key: kv.key, value: b64(kv.value), prev }));
      }
    }
    return new Response(null, { status: 101, webSocket: client });
  }

  private broadcast(ev: { rev: number; type: string; key: string; value: string; prev: string }) {
    const msg = JSON.stringify(ev);
    for (const ws of this.ctx.getWebSockets()) {
      const att = ws.deserializeAttachment() as { prefix: string; exact: boolean } | null;
      if (!att) continue;
      if (att.exact ? ev.key === att.prefix : ev.key.startsWith(att.prefix)) {
        try {
          ws.send(msg);
        } catch {
          ws.close(1011, "send failed");
        }
      }
    }
  }

  async webSocketMessage(): Promise<void> {}

  async webSocketClose(ws: WebSocket): Promise<void> {
    ws.close();
  }

  async webSocketError(ws: WebSocket): Promise<void> {
    ws.close();
  }
}

function prefixEnd(prefix: string): string {
  return prefix.slice(0, -1) + String.fromCharCode(prefix.charCodeAt(prefix.length - 1) + 1);
}

function b64(bytes: Uint8Array): string {
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin);
}

interface KV {
  key: string;
  value: Uint8Array;
  createRevision: number;
  modRevision: number;
}

function rowToKV(row: Record<string, SqlStorageValue>): KV {
  return {
    key: row.name as string,
    value: new Uint8Array(row.value as ArrayBuffer),
    createRevision: row.create_revision as number,
    modRevision: row.id as number,
  };
}

function encodeKV(kv: KV) {
  return { key: kv.key, value: b64(kv.value), createRevision: kv.createRevision, modRevision: kv.modRevision };
}

function conflict(revision: number, error: string): Response {
  return Response.json({ revision, error }, { status: 409 });
}
