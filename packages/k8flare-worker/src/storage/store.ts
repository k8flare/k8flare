// ClusterStore: facet-aware replacements for queries.ts's synchronous
// getCurrent()/insert() plus list/replay helpers, used by index.ts.
// endpoints.ts, scheduler.ts, nodelifecycle.ts, and serviceip.ts (the last
// deleted 2026-07-10 as a v3-design cleanup, once confirmed dead on every
// reachable path) were all deleted, replaced by either the real
// kube-controller-manager running in workers/controllers or a synchronous
// reconcile inside apiserver's Go WASM binary.
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
// Correction (2026-09-13): the paragraph above described the parent's row as
// value-trimmed from the moment it is written. It no longer is. A namespaced
// write now lands in the parent WITH its value and is trimmed only once the
// facet acknowledges it, so a committed row is complete at every instant and
// no reader has to work out what a parent row with no facet row means. That
// question -- unanswerable, because an apply in flight and an apply that never
// happened look identical from one side -- was the cause of every defect this
// file has had (docs/platform-verification.md S67 and its two corrections).
//
// The rule that replaces it, and it holds for every read: PARENT FIRST. If the
// parent's row still has its value, that is the record. If it does not, the
// facet acknowledged before this read began and therefore has it. Reading the
// facet first is never safe: the acknowledgement and the trim can both land in
// between, and the row is then in neither place.
//
// Correction (2026-09-12): the two sentences above claiming live broadcast
// never needs to consult a facet were wrong -- broadcastEvent (watch.ts)
// re-reads the committed row, and follows the same parent-first rule.
import { LIST_SQL, AFTER_SQL, GET_SQL } from "./schema.ts";
import { prefixEnd, rowToEvent, arrayBufferToBase64, base64ToArrayBuffer } from "./helpers.ts";
import type { KineRow, KineEvent, KineKV } from "./helpers.ts";
import { insert, currentRevision } from "./queries.ts";
import type { SqlExec } from "./queries.ts";
import { classifyKey } from "./keyspace.ts";
import { getFacet, facetFetch, facetJson, type FacetHost } from "./facets.ts";

/**
 * Has this row's value been handed over to its facet? Until it has, the
 * parent's copy is the only one, and the row must be read from here.
 *
 * A row that carries neither a value nor a previous one would be
 * indistinguishable from a handed-over one; storeInsert refuses to write one
 * rather than leave that ambiguity in the log.
 */
export function isOffloaded(row: KineRow): boolean {
  return row.value === null && row.old_value === null;
}

/**
 * Replace rows the parent has handed over with the facet's copies, asking
 * each facet for exactly the revisions the parent selected.
 *
 * By revision, never by key or prefix: the facet's own "latest row for this
 * name" is not the row the parent chose. A concurrent update, a LIST at a
 * past revision, or a key whose newest row is a tombstone all make those two
 * disagree, and the disagreement reads as data loss.
 */
async function fillOffloaded(host: FacetHost, rows: KineRow[]): Promise<KineRow[]> {
  const wanted = new Map<string, number[]>();
  for (const row of rows) {
    if (!isOffloaded(row)) continue;
    const cls = classifyKey(row.thename);
    if (cls.kind === "cluster") continue;
    const ids = wanted.get(cls.facet);
    if (ids) ids.push(row.theid);
    else wanted.set(cls.facet, [row.theid]);
  }
  if (wanted.size === 0) return rows;

  const byRevision = new Map<number, KineRow>();
  await Promise.all(
    Array.from(wanted, async ([facetName, ids]) => {
      for (let i = 0; i < ids.length; i += FILL_BATCH) {
        const resp = await facetFetch(
          getFacet(host, facetName),
          new Request("http://facet.internal/rows", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ ids: ids.slice(i, i + FILL_BATCH) }),
          }),
        );
        const body = await facetJson<{ rows?: any[] }>(resp);
        for (const raw of body.rows || []) byRevision.set(raw.theid, facetRawToKineRow(raw));
      }
    }),
  );

  return rows.map((row) => {
    if (!isOffloaded(row)) return row;
    const cls = classifyKey(row.thename);
    if (cls.kind === "cluster") return row;
    const filled = byRevision.get(row.theid);
    if (!filled) {
      // The parent only trims after the facet acknowledges, so this is the
      // facet having lost an acknowledged write. Fail the read rather than
      // serve an object with no value, which is what the caller would get.
      throw new Error(
        `facet ${cls.facet} is missing revision ${row.theid} for ${row.thename}, which the parent handed to it`,
      );
    }
    return filled;
  });
}

/** SQLite takes a bounded number of bound parameters; one round trip per batch. */
const FILL_BATCH = 400;

/** Fill rows the caller already read from the parent. */
export function fillOffloadedRows(host: FacetHost, rows: KineRow[]): Promise<KineRow[]> {
  return fillOffloaded(host, rows);
}

/**
 * The parent's own view of a prefix: one row per key, newest first, tombstones
 * excluded unless asked for. The parent has a row for every key in the
 * cluster, so this is the authoritative key set for any prefix -- including
 * prefixes that span namespaces, which no longer need a fan-out to discover
 * which facets exist.
 */
function parentRows(
  sql: SqlExec,
  prefix: string,
  limit: number,
  revision: number,
  includeDeleted: boolean,
): KineRow[] {
  const deleted = includeDeleted ? 1 : 0;
  const q =
    (revision > 0 ? LIST_SQL("AND mkv.id <= ?4") : LIST_SQL("AND mkv.name > ?4")) +
    (limit > 0 ? ` LIMIT ${limit}` : "");
  return sql.exec(q, prefix, prefixEnd(prefix), deleted, revision > 0 ? revision : "").toArray();
}

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

function prefixMatches(prefix: string, name: string): boolean {
  return prefix.endsWith("/") ? name.startsWith(prefix) : name === prefix;
}

/** Facet-aware replacement for queries.ts's getCurrent(). */
export async function storeGetCurrent(
  sql: SqlExec,
  host: FacetHost,
  key: string,
  includeDeleted = false,
): Promise<{ rev: number; event: KineEvent | null }> {
  const rows = sql.exec(GET_SQL(includeDeleted), key).toArray();
  const rev = rows.length > 0 ? rows[0].current_rev : currentRevision(sql);
  if (rows.length === 0) return { rev, event: null };
  const [filled] = await fillOffloaded(host, rows);
  return { rev, event: rowToEvent(filled) };
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

  if (value === null && oldValue === null) {
    throw new Error(
      `storeInsert: ${key} carries neither a value nor a previous one; such a row cannot be told apart from one whose value has been handed to its facet`,
    );
  }

  const id = insert(sql, key, create, del, createRevision, prevRevision, lease, value, oldValue);
  try {
    const resp = await facetFetch(
      getFacet(host, cls.facet),
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
    await facetJson(resp);
  } catch (err) {
    // The write stands. It is durable in the parent and every read path
    // serves it from there, so reporting failure would be a lie that costs
    // the caller a retry and, for a create, a spurious 409. What is lost is
    // the storage offload for this row until something repairs it, which is
    // why this is loud.
    console.warn(
      `storeInsert: ${key} at revision ${id} stays in the parent: ${String(err)}. ` +
        "Reads are unaffected; the namespace facet does not have this revision.",
    );
    return id;
  }
  sql.exec("UPDATE kine SET value = NULL, old_value = NULL WHERE id = ?1", id);
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
  const rows = parentRows(sql, prefix, limit, revision, false);
  const rev = rows.length > 0 ? rows[0].current_rev : currentRevision(sql);
  const filled = await fillOffloaded(host, rows);
  const kvs = filled.map((r) => rowToEvent(r).kv);
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
  const rows = parentRows(sql, prefix, 0, 0, includeDeleted);
  return fillOffloaded(host, rows);
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
  // Taken before anything is awaited. A bookmark below the content a client
  // is handed makes it re-receive events it already has, which an informer
  // absorbs; a bookmark above makes it skip one, which nothing recovers.
  const bookmark = currentRevision(sql);

  if (sinceRevision > 0) {
    const rows = sql
      .exec(AFTER_SQL, sinceRevision)
      .toArray()
      .filter((r) => prefixMatches(prefix, r.thename));
    const filled = await fillOffloaded(host, rows);
    return { events: filled.map(rowToEvent), bookmark };
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

// Namespace deletion deliberately does NOT destroy its facet -- see the long
// comment on index.ts's handleDelete for the reproduced platform issue this
// avoids. facets.ts still exports deleteFacet as a primitive for whoever
// picks this back up (e.g. once facet names are suffixed with the
// Namespace's own UID, so a reused namespace *name* never reuses a facet
// *name*): the one-liner needed here would be
// `deleteFacet(host, namespaceFacet(namespace))`.
