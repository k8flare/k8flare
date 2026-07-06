import { AFTER_SQL } from "./schema.ts";
import { rowToEvent } from "./helpers.ts";
import type { KineRow } from "./helpers.ts";
import { classifyKey } from "./keyspace.ts";
import { getFacet, facetFetch, facetJson, type FacetHost } from "./facets.ts";
import { storeReplay, facetRawToKineRow } from "./store.ts";
import type { SqlExec } from "./queries.ts";

interface DurableObjectNamespaceLike {
  idFromName(name: string): unknown;
  get(id: unknown): { fetch(req: Request): Promise<Response> };
}

/** Host shape watch.ts needs: facet access (FacetHost) plus a WATCHHUB
 * binding to push events to, and the owning Cluster DO's own instance
 * name (multi-cluster: the WatchHub instance shares it). */
export interface WatchHost extends FacetHost {
  env: FacetHost["env"] & { WATCHHUB?: DurableObjectNamespaceLike };
  doName?: string;
}

/**
 * Plain-HTTP initial replay/snapshot for a watch subscriber (no WebSocket
 * upgrade). WatchHub calls this once per newly-subscribed client to seed it
 * (see docs/multi-tenancy-and-hosting.md's WatchHub section) -- the client
 * then receives live events pushed from broadcastEvent below, so this never
 * needs to be a long-lived connection itself.
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
 * Push the event at `revision` for `key` to WatchHub, which fans it out to
 * matching client WebSockets. Called immediately after a write, awaited
 * before that write's own HTTP response returns, so at most one push is
 * ever in flight -- this is what keeps WatchHub's fan-out in the same order
 * Cluster committed the writes.
 *
 * The row is looked up from wherever it actually lives (the parent's own
 * log for cluster-scoped keys, the owning facet for namespaced/events/
 * ca-vault keys) and defensively filtered down to exactly `revision` --
 * with facet calls now async, an unrelated write can in principle
 * interleave between this write's insert and its broadcast, so a plain
 * range query alone is not guaranteed to return only this one row.
 *
 * There is deliberately no WebSocket between Cluster and WatchHub (see
 * watchhub.ts's module comment for why: a DO cannot ctx.acceptWebSocket() a
 * socket obtained from another DO's fetch() response). A plain fetch() push
 * is simpler and keeps both DOs hibernation-eligible between events.
 */
export async function broadcastEvent(
  host: WatchHost,
  sql: SqlExec,
  key: string,
  revision: number,
): Promise<void> {
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
  const watchhub = host.env.WATCHHUB;
  if (!watchhub) return; // defensive: hosts without a WATCHHUB binding just skip fan-out

  try {
    const stub = watchhub.get(watchhub.idFromName(host.doName ?? "default"));
    const pushResp = await stub.fetch(
      new Request("http://watchhub.internal/push", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ events, bookmark: revision }),
      }),
    );
    if (!pushResp.ok) {
      console.error(
        "broadcastEvent: push to WatchHub failed:",
        pushResp.status,
        await pushResp.text(),
      );
    }
  } catch (err) {
    // Watch delivery is best-effort -- a push failure must not fail the
    // write that produced it. A client that misses this event still
    // recovers via its own replay/resume on reconnect.
    console.error("broadcastEvent: push to WatchHub failed:", err);
  }
}
