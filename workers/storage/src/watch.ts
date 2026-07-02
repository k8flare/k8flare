import { AFTER_SQL, LIST_SQL } from "./schema.ts";
import { rowToEvent, prefixEnd } from "./helpers.ts";
import { currentRevision } from "./queries.ts";
import type { SqlExec } from "./queries.ts";

export interface DurableObjectContext {
  acceptWebSocket(ws: WebSocket, tags?: string[]): void;
  getWebSockets(tag?: string): WebSocket[];
  getTags(ws: WebSocket): string[];
}

export function handleWebSocket(
  ctx: DurableObjectContext,
  sql: SqlExec,
  request: Request,
): Response {
  const url = new URL(request.url);
  const prefix = url.searchParams.get("prefix") || "/";
  const revision = parseInt(url.searchParams.get("revision") || "0");

  const pair = new WebSocketPair();
  const [client, server] = Object.values(pair);

  ctx.acceptWebSocket(server, [prefix]);
  (server as any).serializeAttachment({ prefix, revision });

  if (revision > 0) {
    // Resuming from a known point: replay only what changed since then.
    const rows = sql.exec(AFTER_SQL, revision).toArray();
    // Filter by prefix in JS instead of SQL LIKE
    for (const row of rows) {
      const name = row.thename;
      const match = prefix.endsWith("/") ? name.startsWith(prefix) : name === prefix;
      if (match) server.send(JSON.stringify({ events: [rowToEvent(row)] }));
    }
  } else {
    // Starting fresh (resourceVersion 0/unset): replay current state as
    // synthetic ADDED events, matching Kubernetes watch semantics for
    // clients that watch without a prior List call.
    const rows = sql.exec(LIST_SQL(""), prefix, prefixEnd(prefix), 0).toArray();
    for (const row of rows) {
      const event = rowToEvent(row);
      event.create = true;
      event.delete = false;
      server.send(JSON.stringify({ events: [event] }));
    }
  }

  // Mark the end of the initial replay so the client's watch reflector can
  // consider its cache synced (Kubernetes watch bookmark semantics) —
  // without this, informers built on client-go's reflector never converge.
  server.send(JSON.stringify({ bookmark: currentRevision(sql) }));

  return new Response(null, { status: 101, webSocket: client });
}

export function broadcastEvent(
  ctx: DurableObjectContext,
  sql: SqlExec,
  key: string,
  revision: number,
): void {
  const sockets = ctx.getWebSockets();
  if (sockets.length === 0) return;

  const rows = sql.exec(AFTER_SQL, revision - 1).toArray();
  if (rows.length === 0) return;

  const events = rows.map(rowToEvent);
  const msg = JSON.stringify({ events });

  for (const ws of sockets) {
    const tags = ctx.getTags(ws);
    const prefix = tags[0] || "/";
    const matches = prefix.endsWith("/") ? key.startsWith(prefix) : key === prefix;
    if (matches) {
      try {
        ws.send(msg);
      } catch (_) {}
    }
  }
}
