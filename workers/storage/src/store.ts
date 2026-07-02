// ClusterStore: facet-aware replacements for queries.ts's synchronous
// getCurrent()/insert() plus list/replay helpers, used by index.ts and the
// alarm-driven reconcilers (endpoints.ts, serviceip.ts, scheduler.ts,
// nodelifecycle.ts).
//
// Design (see docs/multi-tenancy-and-hosting.md's honest-correction addendum
// for the full writeup): the parent Cluster DO's own kine table stays the
// sole revision authority and holds full rows for cluster-scoped keys. For a
// namespaced key (or the events-log/ca-vault special cases), the parent
// instead writes a value-trimmed *envelope* row (same columns, value/
// old_value NULL) -- enough to keep resourceVersion ordering, existence, and
// tombstone checks entirely local -- while the actual value lives only in
// the owning facet, written at the exact same id the parent just assigned.
// This is what makes the namespace facet an actual storage offload rather
// than a redundant mirror: a large cluster's Pod/ConfigMap/Secret data lives
// in per-namespace 10GB budgets, not the parent's.
//
// Because the parent always performs the envelope insert *before* calling
// the facet, live broadcast never needs to consult a facet (the caller
// already has the value in hand -- see broadcastEvent in watch.ts). Only
// point reads/lists/replays of namespaced data need a facet round trip.
import { LIST_SQL, AFTER_SQL } from "./schema.ts";
import { prefixEnd, rowToEvent, arrayBufferToBase64, base64ToArrayBuffer } from "./helpers.ts";
import type { KineRow, KineEvent, KineKV } from "./helpers.ts";
import { getCurrent, insert, currentRevision } from "./queries.ts";
import type { SqlExec } from "./queries.ts";
import { classifyKey, classifyPrefix, namespaceFacet, EVENTS_FACET } from "./keyspace.ts";
import { getFacet, facetFetch, facetJson, type FacetHost } from "./facets.ts";

/** Decode a facet's JSON row (base64 value/old_value) back into a real KineRow. */
export function facetRawToKineRow(raw: any): KineRow {
  return {
    current_rev: raw.current_rev,
    compact_rev: null,
    theid: raw.theid,
    thename: raw.thename,
    created: raw.created,
    deleted: raw.deleted,
    create_revision: raw.create_revision,
    prev_revision: raw.prev_revision,
    lease: raw.lease,
    value: base64ToArrayBuffer(raw.value),
    old_value: base64ToArrayBuffer(raw.old_value),
  };
}

function byName(a: KineRow, b: KineRow): number {
  return a.thename < b.thename ? -1 : a.thename > b.thename ? 1 : 0;
}

function prefixMatches(prefix: string, name: string): boolean {
  return prefix.endsWith("/") ? name.startsWith(prefix) : name === prefix;
}

/** List live Namespace names from the parent's own (cluster-scoped) log. */
async function listNamespaces(sql: SqlExec): Promise<string[]> {
  const prefix = "/registry/namespaces/";
  const rows = sql.exec(LIST_SQL("AND mkv.name > ?4"), prefix, prefixEnd(prefix), 0, "").toArray();
  return rows.map((r) => r.thename.slice(prefix.length));
}

async function facetListRaw(
  host: FacetHost,
  facetName: string,
  prefix: string,
  includeDeleted: boolean,
): Promise<KineRow[]> {
  const stub = getFacet(host, facetName);
  const qs = includeDeleted ? "?includeDeleted=1" : "";
  const resp = await facetFetch(stub, new Request(`http://facet.internal/list${prefix}${qs}`));
  const body = await facetJson<{ rows?: any[] }>(resp);
  return (body.rows || []).map(facetRawToKineRow);
}

async function facetList(
  host: FacetHost,
  facetName: string,
  prefix: string,
  limit: number,
  revision: number,
): Promise<{ revision: number; kvs: KineKV[] }> {
  const stub = getFacet(host, facetName);
  const qs = new URLSearchParams();
  if (limit > 0) qs.set("limit", String(limit));
  if (revision > 0) qs.set("revision", String(revision));
  const q = qs.toString();
  const reqUrl = `http://facet.internal/list${prefix}${q ? "?" + q : ""}`;
  const resp = await facetFetch(stub, new Request(reqUrl));
  const body = await facetJson<{ revision: number; rows?: any[] }>(resp);
  const kvs = (body.rows || []).map((r: any) => rowToEvent(facetRawToKineRow(r)).kv);
  return { revision: body.revision, kvs };
}

/** Facet-aware replacement for queries.ts's getCurrent(). */
export async function storeGetCurrent(
  sql: SqlExec,
  host: FacetHost,
  key: string,
  includeDeleted = false,
): Promise<{ rev: number; event: KineEvent | null }> {
  const cls = classifyKey(key);
  if (cls.kind === "cluster") return getCurrent(sql, key, includeDeleted);

  const stub = getFacet(host, cls.facet);
  const resp = await facetFetch(
    stub,
    new Request(`http://facet.internal/key${key}?includeDeleted=${includeDeleted ? 1 : 0}`),
  );
  const body = await facetJson<{ revision: number; row?: any }>(resp);
  if (!body.row) return { rev: body.revision, event: null };
  return { rev: body.revision, event: rowToEvent(facetRawToKineRow(body.row)) };
}

/**
 * Facet-aware replacement for queries.ts's insert(). Cluster-scoped keys are
 * inserted locally exactly as before. Facet-routed keys get a value-trimmed
 * envelope row in the parent (to assign the id and keep ordering/tombstone
 * checks local) plus a full mirrored row in the owning facet at that same
 * id, wrapped in the S1-required retry-on-transient-reset (see facets.ts).
 */
export async function storeInsert(
  sql: SqlExec,
  host: FacetHost,
  key: string,
  create: boolean,
  del: boolean,
  createRevision: number,
  prevRevision: number,
  lease: number,
  value: ArrayBuffer | null,
  oldValue: ArrayBuffer | string | null,
): Promise<number> {
  const cls = classifyKey(key);
  if (cls.kind === "cluster") {
    return insert(sql, key, create, del, createRevision, prevRevision, lease, value, oldValue);
  }

  const id = insert(sql, key, create, del, createRevision, prevRevision, lease, null, null);
  const stub = getFacet(host, cls.facet);
  const resp = await facetFetch(
    stub,
    new Request("http://facet.internal/apply", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        id,
        key,
        created: create,
        deleted: del,
        createRevision,
        prevRevision,
        lease,
        value: arrayBufferToBase64(value),
        oldValue: arrayBufferToBase64(oldValue),
      }),
    }),
  );
  await facetJson(resp); // throws on facet-side error instead of silently leaving the parent's envelope and the facet's copy out of sync
  return id;
}

/** Facet-aware, HTTP-list-shaped read (matches index.ts's handleList response: always excludes tombstones). */
export async function storeList(
  sql: SqlExec,
  host: FacetHost,
  prefix: string,
  limit: number,
  revision: number,
): Promise<{ revision: number; count: number; kvs: KineKV[] }> {
  const cls = classifyPrefix(prefix);

  if (cls.kind === "cluster") {
    const end = prefixEnd(prefix);
    let rows: KineRow[];
    if (revision === 0) {
      const q = LIST_SQL("AND mkv.name > ?4") + (limit > 0 ? ` LIMIT ${limit}` : "");
      rows = sql.exec(q, prefix, end, 0, "").toArray();
    } else {
      const q = LIST_SQL("AND mkv.id <= ?4") + (limit > 0 ? ` LIMIT ${limit}` : "");
      rows = sql.exec(q, prefix, end, 0, revision).toArray();
    }
    const rev = rows.length > 0 ? rows[0].current_rev : currentRevision(sql);
    const kvs = rows.map((r) => rowToEvent(r).kv);
    return { revision: rev, count: kvs.length, kvs };
  }

  if (cls.kind === "namespace" || cls.kind === "events" || cls.kind === "ca-vault") {
    const { revision: rev, kvs } = await facetList(host, cls.facet, prefix, limit, revision);
    return { revision: rev, count: kvs.length, kvs };
  }

  // all-namespaces fan-out ("root" never reaches here -- it's only used
  // internally for the WatchHub firehose, via storeListRaw/storeReplay).
  const namespaces = await listNamespaces(sql);
  const perNs = await Promise.all(
    namespaces.map((ns) => facetList(host, namespaceFacet(ns), prefix, limit, revision)),
  );
  let kvs = perNs.flatMap((r) => r.kvs);
  kvs.sort((a, b) => (a.key < b.key ? -1 : a.key > b.key ? 1 : 0));
  if (limit > 0) kvs = kvs.slice(0, limit);
  const rev = perNs.reduce((m, r) => Math.max(m, r.revision), 0) || currentRevision(sql);
  return { revision: rev, count: kvs.length, kvs };
}

/**
 * Facet-aware raw-row read used by the alarm-driven reconcilers (which need
 * the `deleted` flag and full row shape, not the trimmed HTTP KV shape).
 * Always "current state" (no point-in-time revision parameter -- no
 * reconciler call site has ever needed one).
 */
export async function storeListRaw(
  sql: SqlExec,
  host: FacetHost,
  prefix: string,
  includeDeleted: boolean,
): Promise<KineRow[]> {
  const cls = classifyPrefix(prefix);

  if (cls.kind === "cluster") {
    const q = LIST_SQL("AND mkv.name > ?4");
    return sql.exec(q, prefix, prefixEnd(prefix), includeDeleted ? 1 : 0, "").toArray();
  }
  if (cls.kind === "namespace" || cls.kind === "events" || cls.kind === "ca-vault") {
    return facetListRaw(host, cls.facet, prefix, includeDeleted);
  }
  if (cls.kind === "root") {
    const localRows = sql
      .exec(LIST_SQL("AND mkv.name > ?4"), "/", prefixEnd("/"), includeDeleted ? 1 : 0, "")
      .toArray();
    const namespaces = await listNamespaces(sql);
    const facetNames = [...namespaces.map(namespaceFacet), EVENTS_FACET];
    const perFacet = await Promise.all(
      facetNames.map((fn) => facetListRaw(host, fn, "/", includeDeleted)),
    );
    return [...localRows, ...perFacet.flat()].sort(byName);
  }
  // all-namespaces
  const namespaces = await listNamespaces(sql);
  const perNs = await Promise.all(
    namespaces.map((ns) => facetListRaw(host, namespaceFacet(ns), prefix, includeDeleted)),
  );
  return perNs.flat().sort(byName);
}

/**
 * Initial watch batch for `prefix` (used both by Cluster's own /watch --
 * WatchHub's single upstream connection -- and by /replay, used by WatchHub
 * to seed each newly-subscribed client). sinceRevision === 0 replays current
 * live state as synthetic ADDED events (matching handleWebSocket's existing
 * fresh-watch semantics); sinceRevision > 0 replays the actual event history
 * since that revision.
 */
export async function storeReplay(
  sql: SqlExec,
  host: FacetHost,
  prefix: string,
  sinceRevision: number,
): Promise<{ events: KineEvent[]; bookmark: number }> {
  const bookmark = currentRevision(sql);

  if (sinceRevision > 0) {
    const events = await replayDelta(sql, host, prefix, sinceRevision);
    return { events, bookmark };
  }

  const rows = await storeListRaw(sql, host, prefix, false);
  const events = rows.map((r) => {
    const event = rowToEvent(r);
    event.create = true;
    event.delete = false;
    return event;
  });
  return { events, bookmark };
}

async function replayDelta(
  sql: SqlExec,
  host: FacetHost,
  prefix: string,
  sinceRevision: number,
): Promise<KineEvent[]> {
  const rows = sql.exec(AFTER_SQL, sinceRevision).toArray();
  const matched = rows.filter((r) => prefixMatches(prefix, r.thename));

  const facetNames = new Set<string>();
  for (const r of matched) {
    const cls = classifyKey(r.thename);
    if (cls.kind !== "cluster") facetNames.add(cls.facet);
  }

  const rowsByFacet = new Map<string, Map<number, any>>();
  await Promise.all(
    Array.from(facetNames).map(async (facetName) => {
      const stub = getFacet(host, facetName);
      const resp = await facetFetch(
        stub,
        new Request(`http://facet.internal/after/${sinceRevision}`),
      );
      const body = await facetJson<{ rows?: any[] }>(resp);
      const byId = new Map<number, any>();
      for (const raw of body.rows || []) byId.set(raw.theid, raw);
      rowsByFacet.set(facetName, byId);
    }),
  );

  const filled: KineRow[] = matched.map((r) => {
    const cls = classifyKey(r.thename);
    if (cls.kind === "cluster") return r;
    const raw = rowsByFacet.get(cls.facet)?.get(r.theid);
    return raw ? facetRawToKineRow(raw) : r; // fall back to the (value-less) envelope if somehow missing
  });
  return filled.map(rowToEvent);
}

// Namespace deletion deliberately does NOT destroy its facet -- see the long
// comment on index.ts's handleDelete for the reproduced platform issue this
// avoids. facets.ts still exports deleteFacet as a primitive for whoever
// picks this back up (e.g. once facet names are suffixed with the
// Namespace's own UID, so a reused namespace *name* never reuses a facet
// *name*): the one-liner needed here would be
// `deleteFacet(host, namespaceFacet(namespace))`.
