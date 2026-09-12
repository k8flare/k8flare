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
// Correction (2026-09-12): the two sentences that stood here claimed live
// broadcast never needs to consult a facet because the caller already holds
// the value. It does consult one -- broadcastEvent (watch.ts) re-reads the
// committed row, and for a facet-routed key the parent's own copy is the
// value-trimmed envelope, so it asks the facet. That is what makes a
// broadcast for a write whose facet half failed correctly emit nothing.
// Every read of namespaced data -- points, lists, replays and broadcast --
// goes through the facet.
import { LIST_SQL, AFTER_SQL } from "./schema.ts";
import { prefixEnd, rowToEvent, arrayBufferToBase64, base64ToArrayBuffer } from "./helpers.ts";
import type { KineRow, KineEvent, KineKV } from "./helpers.ts";
import { getCurrent, insert, currentRevision } from "./queries.ts";
import type { SqlExec } from "./queries.ts";
import { classifyKey, classifyPrefix, namespaceFacet, EVENTS_FACET } from "./keyspace.ts";
import { getFacet, facetFetch, facetJson, type FacetHost } from "./facets.ts";

/**
 * Revisions this instance has assigned but not yet heard back about from the
 * owning facet. A Durable Object has exactly one live instance, so an apply
 * can only be in flight from here -- which is what lets a reader tell "the
 * facet has not got it yet" (wait) from "the facet never will" (a write that
 * failed, and whose caller was told so).
 *
 * Keyed by the host object, which the Cluster DO builds once per instance, so
 * two clusters sharing an isolate do not clamp each other's revisions.
 */
const pendingAppliesByHost = new WeakMap<object, Set<number>>();

function pendingApplies(host: FacetHost): Set<number> {
  let pending = pendingAppliesByHost.get(host);
  if (!pending) {
    pending = new Set();
    pendingAppliesByHost.set(host, pending);
  }
  return pending;
}

/**
 * The newest revision a reader may be shown. Everything at or below it has a
 * settled outcome; the first in-flight apply is the ceiling, because handing
 * out a bookmark past a revision that is about to land would make the watcher
 * resume after an event it never received.
 */
function settledRevision(sql: SqlExec, host: FacetHost): number {
  let firstPending = Infinity;
  for (const id of pendingApplies(host)) firstPending = Math.min(firstPending, id);
  const latest = currentRevision(sql);
  return firstPending === Infinity ? latest : Math.min(latest, firstPending - 1);
}

/** Did `id` reach the facet? Asked by exact id -- /key would answer about the key's newest row instead. */
async function facetHasRevision(host: FacetHost, facet: string, id: number): Promise<boolean> {
  const resp = await facetFetch(
    getFacet(host, facet),
    new Request(`http://facet.internal/after/${id - 1}`),
  );
  const body = await facetJson<{ rows?: any[] }>(resp);
  return (body.rows || []).some((raw) => raw.theid === id);
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

  // The revision comes from the parent's own log, never the facet's. It is
  // the compare-and-swap token every caller turns into the next write's
  // prev_revision, and only the parent's is cluster-wide and monotonic: two
  // concurrent creates that both read a *facet* revision pick different
  // prev_revisions, so kine_name_prev_revision_uindex lets both through and
  // the second silently replaces the first.
  for (let attempt = 0; attempt < 8; attempt++) {
    const tip = getCurrent(sql, key, true);
    const resp = await facetFetch(
      getFacet(host, cls.facet),
      new Request(`http://facet.internal/key${key}?includeDeleted=1`),
    );
    const body = await facetJson<{ revision: number; row?: any }>(resp);

    if (tip.event) {
      const tipRevision = tip.event.kv.modRevision;
      if (body.row?.theid !== tipRevision && !pendingApplies(host).has(tipRevision)) {
        // An envelope whose facet half will never arrive. Left in place it
        // pins prev_revision for this key, and every retry of the write that
        // produced it collides with it -- an update retries with the same
        // modRevision forever, so nothing else unwedges the key.
        discardOrphanedEnvelope(sql, key, tipRevision);
        continue;
      }
    }

    const rev = settledRevision(sql, host);
    if (!body.row) return { rev, event: null };
    const event = rowToEvent(facetRawToKineRow(body.row));
    if (event.delete && !includeDeleted) return { rev, event: null };
    return { rev, event };
  }
  throw new Error(`storeGetCurrent: ${key} still has an unresolvable envelope after 8 attempts`);
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
  const pending = pendingApplies(host);
  pending.add(id);
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
    await facetJson(resp); // throws on facet-side error instead of silently leaving the parent's envelope and the facet's copy out of sync
  } catch (err) {
    // A failure here does not mean the facet did not apply: the write may
    // have committed and only the acknowledgement been lost. Ask before
    // undoing anything, and when the question itself cannot be answered,
    // keep the envelope -- a later reader resolves it, by which point no
    // apply for this id can be in flight any more.
    let landed = false;
    try {
      landed = await facetHasRevision(host, cls.facet, id);
    } catch {
      throw err;
    }
    if (landed) return id;
    discardOrphanedEnvelope(sql, key, id);
    throw err;
  } finally {
    pending.delete(id);
  }
  return id;
}

/**
 * Undo the parent's half of a write the facet is known not to hold. Left in
 * place the envelope pins prev_revision for its key, so the write that
 * produced it can never be retried -- kine_name_prev_revision_uindex turns
 * every attempt into a spurious 409 while GET says the object is not there.
 *
 * Callers must have established that the facet will never apply this id:
 * either it answered that it does not have it while no apply was in flight,
 * or it answered a probe after the apply had definitively ended. "The facet
 * did not answer" is not that, and neither is "the fetch threw".
 *
 * Only when the envelope is still the newest row for its key: a later write
 * chains its prev_revision to this id, and removing a link mid-chain is worse
 * than leaving it (replayDelta drops what the facet cannot fill).
 *
 * The id is not reused -- the parent's kine.id is AUTOINCREMENT, so
 * sqlite_sequence keeps handing out higher ids after the delete.
 */
function discardOrphanedEnvelope(sql: SqlExec, key: string, id: number): void {
  sql.exec(
    "DELETE FROM kine WHERE id = ?1 AND NOT EXISTS (SELECT 1 FROM kine WHERE name = ?2 AND id > ?1)",
    id,
    key,
  );
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
    const kvs = rows.map((r) => rowToEvent(r).kv);
    return { revision: settledRevision(sql, host), count: kvs.length, kvs };
  }

  if (cls.kind === "namespace" || cls.kind === "events" || cls.kind === "ca-vault") {
    // The facet's own MAX(id) is the newest revision that happened to touch
    // this namespace, which is not a revision the cluster can be watched
    // from. A client LISTs and then watches from what the LIST reported.
    const { kvs } = await facetList(host, cls.facet, prefix, limit, revision);
    return { revision: settledRevision(sql, host), count: kvs.length, kvs };
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
  return { revision: settledRevision(sql, host), count: kvs.length, kvs };
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
  // Not currentRevision: a bookmark past an apply still in flight is the one
  // number a watcher must never be given. It resumes from the bookmark, and
  // the revision it skipped lands a moment later with nothing to redeliver it.
  const bookmark = settledRevision(sql, host);

  if (sinceRevision > 0) {
    const events = await replayDelta(sql, host, prefix, sinceRevision, bookmark);
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
  ceiling: number,
): Promise<KineEvent[]> {
  const rows = sql.exec(AFTER_SQL, sinceRevision).toArray();
  // Stopping at the ceiling rather than skipping the in-flight revision:
  // a later write to the same key chains its prev_revision to it, so passing
  // it on alone would hand the watcher a modification of a state it never saw.
  const matched = rows.filter((r) => r.theid <= ceiling && prefixMatches(prefix, r.thename));

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

  const filled: KineRow[] = [];
  for (const r of matched) {
    const cls = classifyKey(r.thename);
    if (cls.kind === "cluster") {
      filled.push(r);
      continue;
    }
    const raw = rowsByFacet.get(cls.facet)?.get(r.theid);
    if (raw) {
      filled.push(facetRawToKineRow(raw));
      continue;
    }
    console.warn(
      `replayDelta: dropping revision ${r.theid} for ${r.thename}: facet ${cls.facet} has no row. ` +
        "It is at or below the settled ceiling, so no apply for it is in flight and none can start.",
    );
  }
  return filled.map(rowToEvent);
}

// Namespace deletion deliberately does NOT destroy its facet -- see the long
// comment on index.ts's handleDelete for the reproduced platform issue this
// avoids. facets.ts still exports deleteFacet as a primitive for whoever
// picks this back up (e.g. once facet names are suffixed with the
// Namespace's own UID, so a reused namespace *name* never reuses a facet
// *name*): the one-liner needed here would be
// `deleteFacet(host, namespaceFacet(namespace))`.
