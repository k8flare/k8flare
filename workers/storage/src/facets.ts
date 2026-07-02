// Facet class delivery: build-time bundled string + env.LOADER.get(...) ->
// getDurableObjectClass() -> ctx.facets.get(name, () => ({ class })).
//
// This is the pattern verified empirically against a real deployment in
// commit 46df0c0 and spikes/s1-facets: a facet's `class` MUST come from the
// Dynamic Workers loader -- a plain statically-imported DurableObject
// subclass fails at runtime with a StartupOptions TypeError. So the facet
// class below is authored as a self-contained plain-JS source string (no
// imports, no TypeScript) and loaded dynamically even though it never
// changes at runtime.
//
// The facet is a minimal, generic kine-shaped key-value log: (id, name,
// created, deleted, create_revision, prev_revision, lease, value,
// old_value), with `id` always supplied explicitly by the parent (the
// revision authority -- see store.ts) rather than autoincremented locally.
// One facet class serves all three facet kinds (ns/<namespace>, events-log,
// ca-vault); only the *name* passed to ctx.facets.get() differs.
//
// Row field names deliberately match helpers.ts's KineRow (current_rev,
// theid, thename, create_revision, prev_revision, old_value, ...) so the
// parent can feed a decoded facet row straight into the existing
// rowToEvent() without a parallel reimplementation. value/old_value cross
// the fetch() JSON boundary as base64 strings (ArrayBuffer isn't
// JSON-serializable); the parent decodes with base64ToArrayBuffer() before
// calling rowToEvent(), which re-encodes for its own JSON response --
// slightly round-about but reuses tested logic instead of duplicating it.
//
// Per S2 (spikes/s2-loader/FINDINGS.md item 3a): a loaded worker's `env`
// cannot carry a DurableObjectNamespace/Stub (DataCloneError), so facets
// cannot reach any other DO on their own -- this facet class exposes only a
// plain fetch() contract read/written by its parent DO, never anyone else.
const FACET_SOURCE = `
import { DurableObject } from "cloudflare:workers";

function b64encode(buf) {
  if (buf === null || buf === undefined) return null;
  const bytes = new Uint8Array(buf);
  let binary = "";
  for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i]);
  return btoa(binary);
}

function prefixEnd(prefix) {
  if (prefix.length === 0) return "\\xff";
  const last = prefix.charCodeAt(prefix.length - 1);
  return prefix.slice(0, -1) + String.fromCharCode(last + 1);
}

function rowToRaw(row) {
  return {
    current_rev: row.current_rev,
    theid: row.theid,
    thename: row.thename,
    created: row.created,
    deleted: row.deleted,
    create_revision: row.create_revision,
    prev_revision: row.prev_revision,
    lease: row.lease,
    value: row.value ? b64encode(row.value) : null,
    old_value: row.old_value ? b64encode(row.old_value) : null,
  };
}

const SCHEMA = [
  "CREATE TABLE IF NOT EXISTS kine (id INTEGER PRIMARY KEY, name TEXT, created INTEGER, deleted INTEGER, create_revision INTEGER, prev_revision INTEGER, lease INTEGER, value BLOB, old_value BLOB)",
  "CREATE INDEX IF NOT EXISTS kine_name_index ON kine (name)",
  "CREATE INDEX IF NOT EXISTS kine_name_id_index ON kine (name,id)",
];

function getSql(includeDeleted) {
  return "SELECT (SELECT MAX(id) FROM kine) AS current_rev, id AS theid, name AS thename, created, deleted, create_revision, prev_revision, lease, value, old_value FROM kine WHERE id = (SELECT MAX(id) FROM kine WHERE name = ?1) AND (deleted = 0 OR " + (includeDeleted ? 1 : 0) + ")";
}

function listSql(extraCondition) {
  return "SELECT * FROM (SELECT (SELECT MAX(id) FROM kine) AS current_rev, id AS theid, name AS thename, created, deleted, create_revision, prev_revision, lease, value, old_value FROM kine AS kv JOIN (SELECT MAX(mkv.id) AS id FROM kine AS mkv WHERE mkv.name >= ?1 AND mkv.name < ?2 " + extraCondition + " GROUP BY mkv.name) AS maxkv ON maxkv.id = kv.id WHERE kv.deleted = 0 OR ?3) AS lkv ORDER BY lkv.thename ASC";
}

const AFTER_SQL = "SELECT (SELECT MAX(id) FROM kine) AS current_rev, id AS theid, name AS thename, created, deleted, create_revision, prev_revision, lease, value, old_value FROM kine WHERE id > ?1 ORDER BY id ASC";

const APPLY_SQL = "INSERT INTO kine(id, name, created, deleted, create_revision, prev_revision, lease, value, old_value) VALUES(?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)";

function b64decode(b64) {
  if (!b64) return null;
  const binary = atob(b64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes.buffer;
}

export class Facet extends DurableObject {
  constructor(ctx, env) {
    super(ctx, env);
    this.sql = ctx.storage.sql;
    this.initialized = false;
  }

  init() {
    if (this.initialized) return;
    for (const stmt of SCHEMA) this.sql.exec(stmt);
    this.initialized = true;
  }

  async fetch(request) {
    this.init();
    const url = new URL(request.url);
    const path = url.pathname;

    try {
      if (path === "/apply" && request.method === "POST") {
        const body = await request.json();
        this.sql.exec(
          APPLY_SQL,
          body.id,
          body.key,
          body.created ? 1 : 0,
          body.deleted ? 1 : 0,
          body.createRevision,
          body.prevRevision,
          body.lease || 0,
          b64decode(body.value),
          b64decode(body.oldValue),
        );
        return Response.json({ ok: true });
      }

      if (path.startsWith("/key/") && request.method === "GET") {
        const key = "/" + path.slice(5);
        const includeDeleted = url.searchParams.get("includeDeleted") === "1";
        const rows = this.sql.exec(getSql(includeDeleted), key).toArray();
        if (rows.length === 0) {
          const revRow = this.sql.exec("SELECT MAX(id) AS rev FROM kine").one();
          return Response.json({ revision: revRow.rev || 0, row: null });
        }
        return Response.json({ revision: rows[0].current_rev, row: rowToRaw(rows[0]) });
      }

      if (path.startsWith("/list/") && request.method === "GET") {
        let prefix = "/" + path.slice(6);
        if (prefix.charAt(prefix.length - 1) !== "/") prefix += "/";
        const end = prefixEnd(prefix);
        const limit = parseInt(url.searchParams.get("limit") || "0");
        const revision = parseInt(url.searchParams.get("revision") || "0");
        const includeDeleted = url.searchParams.get("includeDeleted") === "1" ? 1 : 0;
        let q, rows2;
        if (revision === 0) {
          q = listSql("AND mkv.name > ?4") + (limit > 0 ? " LIMIT " + limit : "");
          rows2 = this.sql.exec(q, prefix, end, includeDeleted, "").toArray();
        } else {
          q = listSql("AND mkv.id <= ?4") + (limit > 0 ? " LIMIT " + limit : "");
          rows2 = this.sql.exec(q, prefix, end, includeDeleted, revision).toArray();
        }
        const revRow2 = this.sql.exec("SELECT MAX(id) AS rev FROM kine").one();
        const rev2 = rows2.length > 0 ? rows2[0].current_rev : (revRow2.rev || 0);
        return Response.json({ revision: rev2, rows: rows2.map(rowToRaw) });
      }

      if (path.startsWith("/after/")) {
        const sinceRev = parseInt(path.slice(7) || "0");
        const rows3 = this.sql.exec(AFTER_SQL, sinceRev).toArray();
        return Response.json({ rows: rows3.map(rowToRaw) });
      }

      return Response.json({ ok: false, error: "unknown path " + path }, { status: 404 });
    } catch (err) {
      return Response.json({ ok: false, error: String((err && err.message) || err) }, { status: 500 });
    }
  }
}
`;

/** Cheap, stable, non-cryptographic string hash (djb2) for a content-hash loader key. */
function hashSource(src: string): string {
  let h = 5381;
  for (let i = 0; i < src.length; i++) {
    h = (h * 33) ^ src.charCodeAt(i);
  }
  return (h >>> 0).toString(16);
}

const FACET_SOURCE_HASH = hashSource(FACET_SOURCE);
const FACET_LOADER_KEY = `k8flare/facet/kine-store@${FACET_SOURCE_HASH}`;

/** Errors S1 found to be rare-but-retryable during mass facet creation. */
function isRetryableFacetError(err: unknown): boolean {
  const message = err instanceof Error ? err.message : String(err);
  return /storage caused object to be reset/i.test(message);
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Retry wrapper for facet creation (S1: mass facet creation can hit a rare
 * transient "DO storage reset" error, ~2 of 20+ bursts; immediate retry with
 * a fresh attempt succeeds). Applied around every facet fetch() since the
 * loader's `.get(id, factory)` runs the factory lazily on first real use, so
 * the failure surfaces on whichever call happens to be first for a given
 * facet name -- not necessarily a call this module can identify in advance.
 */
export async function facetFetch(
  stub: { fetch(req: Request): Promise<Response> },
  req: Request,
  maxAttempts = 3,
): Promise<Response> {
  let lastErr: unknown;
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      return await stub.fetch(req.clone());
    } catch (err) {
      lastErr = err;
      if (!isRetryableFacetError(err) || attempt === maxAttempts) throw err;
      await sleep(50 * attempt);
    }
  }
  throw lastErr;
}

/** Minimal structural shape this module needs from the DO context/env -- see watch.ts's DurableObjectContext for why this isn't the official workers-types shape (facets predate that package's pinned version; see spikes/s1-facets/FINDINGS.md item 0). */
export interface FacetHost {
  env: { LOADER: any };
  ctx: {
    facets: {
      get(name: string, factory: () => { class: any }): { fetch(req: Request): Promise<Response> };
      delete(name: string): void;
    };
  };
}

let cachedFacetWorkerStub: any = null;

function getFacetClass(host: FacetHost): any {
  if (!cachedFacetWorkerStub) {
    cachedFacetWorkerStub = host.env.LOADER.get(FACET_LOADER_KEY, () => ({
      compatibilityDate: "2026-03-24",
      mainModule: "facet.js",
      modules: { "facet.js": FACET_SOURCE },
    }));
  }
  return cachedFacetWorkerStub.getDurableObjectClass("Facet");
}

/** Get (creating on first use) the facet stub for `name`. */
export function getFacet(
  host: FacetHost,
  name: string,
): { fetch(req: Request): Promise<Response> } {
  const cls = getFacetClass(host);
  return host.ctx.facets.get(name, () => ({ class: cls }));
}

/** Delete a facet (namespace deletion cleanup). No-op if it never existed (S1 item 4). */
export function deleteFacet(host: FacetHost, name: string): void {
  host.ctx.facets.delete(name);
}
