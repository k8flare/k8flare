import { AFTER_SQL } from "./schema.ts";
import { rowToEvent } from "./helpers.ts";
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
    const rows = sql.exec(AFTER_SQL, revision).toArray();
    // Filter by prefix in JS instead of SQL LIKE
    for (const row of rows) {
      const name = row.thename;
      const match = prefix.endsWith("/") ? name.startsWith(prefix) : name === prefix;
      if (match) server.send(JSON.stringify({ events: [rowToEvent(row)] }));
    }
  }

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
