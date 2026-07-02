import { AFTER_SQL } from "./schema.ts";
import { rowToEvent } from "./helpers.ts";
import type { KineRow } from "./helpers.ts";
import { classifyKey } from "./keyspace.ts";
import { getFacet, facetFetch, facetJson, type FacetHost } from "./facets.ts";
import { storeReplay, facetRawToKineRow } from "./store.ts";
import type { SqlExec } from "./queries.ts";

export interface DurableObjectContext {
  acceptWebSocket(ws: WebSocket, tags?: string[]): void;
  getWebSockets(tag?: string): WebSocket[];
  getTags(ws: WebSocket): string[];
  facets: {
    get(name: string, factory: () => { class: any }): { fetch(req: Request): Promise<Response> };
    delete(name: string): void;
  };
}

/** Host shape watch.ts needs: facet access (FacetHost) plus the WebSocket-specific ctx methods. */
export interface WatchHost extends FacetHost {
  ctx: DurableObjectContext;
}

/**
 * Handle a WebSocket watch connection. This is Cluster's own /watch
 * endpoint -- WatchHub's single upstream firehose connection (see
 * watchhub.ts) is the only caller now that gateway talks to WatchHub
 * directly, but the contract is unchanged so anything else that connects
 * here (e.g. local debugging) keeps working the same way.
 */
export async function handleWebSocket(
  host: WatchHost,
  sql: SqlExec,
  request: Request,
): Promise<Response> {
  const url = new URL(request.url);
  const prefix = url.searchParams.get("prefix") || "/";
  const revision = parseInt(url.searchParams.get("revision") || "0");

  const pair = new WebSocketPair();
  const [client, server] = Object.values(pair);

  host.ctx.acceptWebSocket(server, [prefix]);
  (server as any).serializeAttachment({ prefix, revision });

  const { events, bookmark } = await storeReplay(sql, host, prefix, revision);
  for (const event of events) {
    server.send(JSON.stringify({ events: [event] }));
  }
  // Mark the end of the initial replay so the client's watch reflector can
  // consider its cache synced (Kubernetes watch bookmark semantics) —
  // without this, informers built on client-go's reflector never converge.
  server.send(JSON.stringify({ bookmark }));

  return new Response(null, { status: 101, webSocket: client });
}

/**
 * Plain-HTTP counterpart of handleWebSocket's initial replay/snapshot, with
 * no WebSocket upgrade. WatchHub calls this once per newly-subscribed client
 * to seed it (see docs/multi-tenancy-and-hosting.md's WatchHub section) --
 * the client then receives live events from WatchHub's own single upstream
 * firehose, so this never needs to be a long-lived connection itself.
 */
export async function handleReplay(
  host: WatchHost,
  sql: SqlExec,
  request: Request,
): Promise<Response> {
  const url = new URL(request.url);
  const prefix = url.searchParams.get("prefix") || "/";
  const revision = parseInt(url.searchParams.get("revision") || "0");
  const { events, bookmark } = await storeReplay(sql, host, prefix, revision);
  return Response.json({ events, bookmark });
}

/**
 * Broadcast the event at `revision` for `key` to matching WebSocket
 * listeners. Called immediately after a write with the revision that write
 * was just assigned; the row is looked up from wherever it actually lives
 * (the parent's own log for cluster-scoped keys, the owning facet for
 * namespaced/events/ca-vault keys) and then defensively filtered down to
 * exactly `revision` -- with facet calls now async, an unrelated write can
 * in principle interleave between this write's insert and its broadcast, so
 * a plain range query alone (as the pre-facet version of this function
 * used) is no longer guaranteed to return only this one row.
 */
export async function broadcastEvent(
  host: WatchHost,
  sql: SqlExec,
  key: string,
  revision: number,
): Promise<void> {
  const sockets = host.ctx.getWebSockets();
  if (sockets.length === 0) return;

  const cls = classifyKey(key);
  let rows: KineRow[];
  if (cls.kind === "cluster") {
    rows = sql.exec(AFTER_SQL, revision - 1).toArray();
  } else {
    const stub = getFacet(host, cls.facet);
    const resp = await facetFetch(stub, new Request(`http://facet.internal/after/${revision - 1}`));
    const body = await facetJson<{ rows?: any[] }>(resp);
    rows = (body.rows || []).map(facetRawToKineRow);
  }

  rows = rows.filter((r) => r.theid === revision);
  if (rows.length === 0) return;

  const events = rows.map(rowToEvent);
  const msg = JSON.stringify({ events });

  for (const ws of sockets) {
    const tags = host.ctx.getTags(ws);
    const prefix = tags[0] || "/";
    const matches = prefix.endsWith("/") ? key.startsWith(prefix) : key === prefix;
    if (matches) {
      try {
        ws.send(msg);
      } catch (_) {}
    }
  }
}
