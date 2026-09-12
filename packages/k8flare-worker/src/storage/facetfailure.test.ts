// What happens to a namespaced write when the parent's envelope insert
// succeeds and the facet's /apply does not. The parent is the revision
// authority and the facet holds the only copy of the value, so the two
// writes cannot be one transaction -- the question is what the gap leaves
// behind, and whether the cluster recovers by itself.
//
// The facet under test is the real thing: FACET_SOURCE is the module source
// that ships to the loader, evaluated here against node:sqlite rather than
// re-implemented, so a change to its SQL is a change to this test's subject.
import { beforeEach, describe, expect, it, vi } from "vite-plus/test";
import { DatabaseSync } from "node:sqlite";

vi.mock("cloudflare:workers", () => ({ DurableObject: class {} }));

import { SCHEMA } from "./schema.ts";
import { FACET_SOURCE } from "./facets.ts";
import type { FacetHost } from "./facets.ts";
import { Cluster } from "./index.ts";
import { storeGetCurrent, storeInsert, storeReplay } from "./store.ts";
import type { SqlExec } from "./queries.ts";

/** Just enough of DurableObjectStorage for Cluster's write path. */
function clusterStorage(sql: SqlExec) {
  const kv = new Map<string, unknown>();
  return {
    sql,
    setAlarm: () => {},
    getAlarm: async () => null,
    deleteAlarm: async () => {},
    get: async (k: string) => kv.get(k),
    put: async (k: string, v: unknown) => void kv.set(k, v),
    delete: async (k: string) => kv.delete(k),
  };
}

function bindable(v: unknown): unknown {
  if (v instanceof ArrayBuffer) return new Uint8Array(v);
  if (typeof v === "boolean") return v ? 1 : 0;
  return v ?? null;
}

function sqlite(statements: readonly string[]): { sql: SqlExec; db: DatabaseSync } {
  const db = new DatabaseSync(":memory:");
  for (const s of statements) db.exec(s);
  const sql: SqlExec = {
    exec(query: string, ...params: unknown[]) {
      const stmt = db.prepare(query);
      const bound = params.map(bindable);
      const rows = /^\s*(INSERT|UPDATE|DELETE)/i.test(query)
        ? (stmt.run(...(bound as any)), [])
        : (stmt.all(...(bound as any)) as any[]);
      return { one: () => rows[0] as any, toArray: () => rows as any };
    },
  };
  return { sql, db };
}

const FacetClass = new Function(
  "DurableObject",
  FACET_SOURCE.replace('import { DurableObject } from "cloudflare:workers";', "").replace(
    "export class Facet",
    "class Facet",
  ) + "\nreturn Facet;",
)(class {});

class FakeFacet {
  failApply = false;
  /** Apply for real, then lose the acknowledgement the way a reset DO does. */
  loseApplyAck = false;
  /** Apply for real, then fail in a way facetFetch will not retry. */
  loseApplyAckPermanently = false;
  applies = 0;
  private readonly inner: any;
  constructor() {
    const { sql } = sqlite([]);
    this.inner = new FacetClass({ storage: { sql } }, {});
  }
  /** Block applies for one key (or all) until release() -- an apply in flight. */
  hold(key = "*"): void {
    let release!: () => void;
    const blocked = new Promise<void>((resolve) => {
      release = resolve;
    });
    this.holds.set(key, { blocked, release });
  }
  release(key = "*"): void {
    const held = this.holds.get(key);
    this.holds.delete(key);
    held?.release();
  }
  private readonly holds = new Map<string, { blocked: Promise<void>; release: () => void }>();
  async fetch(req: Request): Promise<Response> {
    const isApply = new URL(req.url).pathname === "/apply";
    let forward = req;
    if (isApply) {
      this.applies++;
      // Read the body once and rebuild: cloning a Request tees its stream,
      // and a tee whose other branch nobody drains does not finish here.
      const text = await req.text();
      forward = new Request(req.url, { method: "POST", headers: req.headers, body: text });
      const applyKey = (JSON.parse(text) as { key: string }).key;
      const held = this.holds.get(applyKey) ?? this.holds.get("*");
      if (held) await held.blocked;
      if (this.failApply) throw new Error("facet unavailable");
    }
    const resp = await this.inner.fetch(forward);
    if (isApply && this.loseApplyAck) {
      this.loseApplyAck = false;
      throw new Error("Durable Object storage caused object to be reset");
    }
    if (isApply && this.loseApplyAckPermanently) {
      this.loseApplyAckPermanently = false;
      throw new Error("network connection lost");
    }
    return resp;
  }
}

describe("a namespaced write whose facet half fails", () => {
  let sql: SqlExec;
  let facet: FakeFacet;
  let facetsUnavailable: boolean;
  let host: FacetHost;
  const key = "/registry/pods/default/web";

  beforeEach(() => {
    ({ sql } = sqlite(SCHEMA));
    facet = new FakeFacet();
    facetsUnavailable = false;
    host = {
      env: { LOADER: { get: () => ({ getDurableObjectClass: () => FacetClass }) } },
      ctx: {
        facets: {
          get: () => {
            if (facetsUnavailable) throw new Error("facet loader unavailable");
            return facet;
          },
          delete: () => {},
        },
      },
    } as unknown as FacetHost;
  });

  async function declareNamespace(): Promise<void> {
    // storeListRaw fans out over the Namespaces the parent's own log knows
    // about, so a facet nobody declared is a facet nobody reads.
    await storeInsert(
      sql,
      host,
      "/registry/namespaces/default",
      true,
      false,
      0,
      0,
      0,
      new TextEncoder().encode("{}").buffer as ArrayBuffer,
      null,
    );
  }

  // Replays what index.ts's handlePut does for a create, which is where the
  // prevRevision a retry will use comes from.
  async function create(value: string): Promise<number> {
    const { event } = await storeGetCurrent(sql, host, key, true);
    const prevRevision = event ? event.kv.modRevision : 0;
    return storeInsert(
      sql,
      host,
      key,
      true,
      false,
      0,
      prevRevision,
      0,
      new TextEncoder().encode(value).buffer as ArrayBuffer,
      null,
    );
  }

  it("reports the failure to the caller", async () => {
    facet.failApply = true;
    await expect(create("v1")).rejects.toThrow();
  });

  it("leaves the object absent, as the caller was told", async () => {
    facet.failApply = true;
    await expect(create("v1")).rejects.toThrow();
    const { event } = await storeGetCurrent(sql, host, key, true);
    expect(event).toBeNull();
  });

  it("lets the client retry the same create", async () => {
    facet.failApply = true;
    await expect(create("v1")).rejects.toThrow();
    facet.failApply = false;
    const id = await create("v1");
    expect(id).toBeGreaterThan(0);
    const { event } = await storeGetCurrent(sql, host, key, false);
    expect(event?.kv.value).toBe(btoa("v1"));
  });

  it("never replays the failed write as a value-less event", async () => {
    const first = await create("v1");
    facet.failApply = true;
    await expect(
      storeInsert(
        sql,
        host,
        key,
        false,
        false,
        first,
        first,
        0,
        new TextEncoder().encode("v2").buffer as ArrayBuffer,
        null,
      ),
    ).rejects.toThrow();
    facet.failApply = false;

    // sinceRevision > 0 is the delta path -- the one that reads the parent's
    // log and asks each facet to fill in the values it trimmed.
    const { events } = await storeReplay(sql, host, "/registry/pods/", first);
    for (const e of events) expect(e.kv.value).not.toBeNull();
  });

  it("still replays the writes that did land", async () => {
    const first = await create("v1");
    facet.failApply = true;
    await expect(
      storeInsert(
        sql,
        host,
        key,
        false,
        false,
        first,
        first,
        0,
        new TextEncoder().encode("v2").buffer as ArrayBuffer,
        null,
      ),
    ).rejects.toThrow();
    facet.failApply = false;
    const third = await storeInsert(
      sql,
      host,
      key,
      false,
      false,
      first,
      (await storeGetCurrent(sql, host, key, true)).event!.kv.modRevision,
      0,
      new TextEncoder().encode("v3").buffer as ArrayBuffer,
      null,
    );

    const { events } = await storeReplay(sql, host, "/registry/pods/", first);
    expect(events.map((e) => e.kv.modRevision)).toEqual([third]);
    expect(events[0].kv.value).toBe(btoa("v3"));
  });

  it("serves a fresh watch from the facet, which never saw the failed write", async () => {
    await declareNamespace();
    facet.failApply = true;
    await expect(create("v1")).rejects.toThrow();
    facet.failApply = false;
    const { events } = await storeReplay(sql, host, "/registry/pods/", 0);
    expect(events).toEqual([]);
  });

  it("survives a retry of an apply that had already committed", async () => {
    // facetFetch retries a reset the platform reports after the write is
    // durable (S1). A second apply of the same id must not be an error, or a
    // write that landed is reported to the client as having failed.
    facet.loseApplyAck = true;
    const id = await create("v1");
    expect(facet.applies).toBe(2);
    const { event } = await storeGetCurrent(sql, host, key, false);
    expect(event?.kv.modRevision).toBe(id);
    expect(event?.kv.value).toBe(btoa("v1"));
  });

  it("drops a residue envelope it could not remove", async () => {
    // The envelope is kept deliberately when a later write for the same key
    // already chains to it, and a crash between the facet failure and the
    // cleanup leaves one too. Either way the reader must not turn it into an
    // event carrying no value.
    const first = await create("v1");
    sql.exec(
      "INSERT INTO kine(name, created, deleted, create_revision, prev_revision, lease, value, old_value)" +
        " VALUES(?1, 0, 0, ?2, ?3, 0, NULL, NULL)",
      key,
      first,
      first,
    );
    const { events } = await storeReplay(sql, host, "/registry/pods/", first);
    expect(events).toEqual([]);
  });

  it("reports success for a write that landed and lost only its acknowledgement", async () => {
    // The counterexample that killed the first version of this fix: a
    // transport failure after the facet committed is indistinguishable from
    // one before it, and undoing the parent's half on the guess deletes a
    // write that exists -- GET would still serve it while delta replay never
    // would again.
    const first = await create("v1");
    facet.loseApplyAckPermanently = true;
    const id = await storeInsert(
      sql,
      host,
      key,
      false,
      false,
      first,
      first,
      0,
      new TextEncoder().encode("v2").buffer as ArrayBuffer,
      null,
    );
    const { event } = await storeGetCurrent(sql, host, key, false);
    expect(event?.kv.modRevision).toBe(id);
    expect(event?.kv.value).toBe(btoa("v2"));
    const { events } = await storeReplay(sql, host, "/registry/pods/", first);
    expect(events.map((e) => e.kv.modRevision)).toEqual([id]);
  });

  it("does not hand out a bookmark past an apply still in flight", async () => {
    const first = await create("v1");
    facet.hold();
    const inflight = storeInsert(
      sql,
      host,
      key,
      false,
      false,
      first,
      first,
      0,
      new TextEncoder().encode("v2").buffer as ArrayBuffer,
      null,
    );

    const during = await storeReplay(sql, host, "/registry/pods/", first);
    expect(during.events).toEqual([]);
    expect(during.bookmark).toBe(first);

    facet.release();
    const second = await inflight;
    const after = await storeReplay(sql, host, "/registry/pods/", first);
    expect(after.events.map((e) => e.kv.modRevision)).toEqual([second]);
    expect(after.bookmark).toBe(second);
  });

  it("does not hand a fresh watch a bookmark past an apply still in flight", async () => {
    await declareNamespace();
    const first = await create("v1");
    facet.hold();
    const inflight = storeInsert(
      sql,
      host,
      key,
      false,
      false,
      first,
      first,
      0,
      new TextEncoder().encode("v2").buffer as ArrayBuffer,
      null,
    );

    const fresh = await storeReplay(sql, host, "/registry/pods/", 0);
    expect(fresh.bookmark).toBe(first);
    expect(fresh.events.map((e) => e.kv.value)).toEqual([btoa("v1")]);

    facet.release();
    await inflight;
  });

  it("rejects a concurrent create of a key whose first create is in flight", async () => {
    // The second create must not be let through by a revision the facet
    // happened to advance on someone else's behalf: only the parent's log
    // knows this key already has a create pending at that prev_revision.
    facet.hold(key);
    const inflight = create("v1");
    await storeInsert(
      sql,
      host,
      "/registry/pods/default/other",
      true,
      false,
      0,
      0,
      0,
      new TextEncoder().encode("other").buffer as ArrayBuffer,
      null,
    );

    await expect(create("also-v1")).rejects.toThrow(/UNIQUE constraint/);
    facet.release(key);
    await inflight;
  });

  it("recovers when the facet stub itself could not be obtained", async () => {
    facetsUnavailable = true;
    await expect(create("v1")).rejects.toThrow();
    facetsUnavailable = false;
    const id = await create("v1");
    const { event } = await storeGetCurrent(sql, host, key, false);
    expect(event?.kv.modRevision).toBe(id);
  });

  it("unwedges an update whose envelope outlived the isolate that wrote it", async () => {
    // A create self-heals because its retry re-reads the revision, but an
    // update retries with the object's own modRevision, which never moves --
    // so nothing but removing the envelope lets it through.
    const first = await create("v1");
    sql.exec(
      "INSERT INTO kine(name, created, deleted, create_revision, prev_revision, lease, value, old_value)" +
        " VALUES(?1, 0, 0, ?2, ?3, 0, NULL, NULL)",
      key,
      first,
      first,
    );
    const { event } = await storeGetCurrent(sql, host, key, false);
    expect(event?.kv.modRevision).toBe(first);
    const id = await storeInsert(
      sql,
      host,
      key,
      false,
      false,
      first,
      event!.kv.modRevision,
      0,
      new TextEncoder().encode("v2").buffer as ArrayBuffer,
      null,
    );
    expect(id).toBeGreaterThan(first);
  });

  it("never replays an event above the bookmark it hands out", async () => {
    // A cluster-scoped key needs no facet, so it commits at a revision above
    // one that is still in flight. Emitting it while withholding the lower
    // revision would put the watcher ahead of its own resume point.
    const first = await create("v1");
    facet.hold();
    const inflight = storeInsert(
      sql,
      host,
      key,
      false,
      false,
      first,
      first,
      0,
      new TextEncoder().encode("v2").buffer as ArrayBuffer,
      null,
    );
    await storeInsert(
      sql,
      host,
      "/registry/namespaces/other",
      true,
      false,
      0,
      0,
      0,
      new TextEncoder().encode("{}").buffer as ArrayBuffer,
      null,
    );

    const { events, bookmark } = await storeReplay(sql, host, "/registry/", first);
    for (const e of events) expect(e.kv.modRevision).toBeLessThanOrEqual(bookmark);

    facet.release();
    await inflight;
  });

  it("admits exactly one of two concurrent creates, through the real write path", async () => {
    // Driven through Cluster.fetch rather than a stand-in for handlePut, so
    // the prev_revision a create actually uses is the one under test.
    const { sql: clusterSql } = sqlite(SCHEMA);
    const cluster = new Cluster(
      {
        id: { name: "default" },
        facets: { get: () => facet, delete: () => {} },
        storage: clusterStorage(clusterSql),
      },
      { LOADER: { get: () => ({ getDurableObjectClass: () => FacetClass }) } },
    ) as unknown as { fetch(req: Request): Promise<Response> };

    const put = () =>
      cluster.fetch(
        new Request(`http://do.internal/key${key}`, {
          method: "PUT",
          body: JSON.stringify({ value: btoa("v1") }),
        }),
      );

    facet.hold(key);
    const first = put();
    await storeInsert(
      clusterSql,
      host,
      "/registry/pods/default/other",
      true,
      false,
      0,
      0,
      0,
      new TextEncoder().encode("other").buffer as ArrayBuffer,
      null,
    );
    const second = await put();
    expect(second.status).toBe(409);

    facet.release(key);
    expect((await first).status).toBe(201);
  });

  it("keeps a create's compare-and-swap token still while another apply resolves", async () => {
    // Both racers must derive the SAME prev_revision. Anything read from the
    // store moves: here an unrelated apply resolves between the two reads, so
    // a create keyed on the current revision lets the second racer in and it
    // silently replaces the first.
    const { sql: clusterSql } = sqlite(SCHEMA);
    const cluster = new Cluster(
      {
        id: { name: "default" },
        facets: { get: () => facet, delete: () => {} },
        storage: clusterStorage(clusterSql),
      },
      { LOADER: { get: () => ({ getDurableObjectClass: () => FacetClass }) } },
    ) as unknown as { fetch(req: Request): Promise<Response> };
    const put = (k: string) =>
      cluster.fetch(
        new Request(`http://do.internal/key${k}`, {
          method: "PUT",
          body: JSON.stringify({ value: btoa("v1") }),
        }),
      );

    await put("/registry/namespaces/default");

    const other = "/registry/pods/default/other";
    facet.hold(other);
    const otherWrite = put(other);

    facet.hold(key);
    const first = put(key);

    facet.release(other);
    expect((await otherWrite).status).toBe(201);

    // Raced against a timer because the failure mode is a create that was
    // admitted: its apply then queues behind the first one and never answers.
    const second = await Promise.race([
      put(key),
      new Promise<Response>((_, reject) =>
        setTimeout(
          () =>
            reject(
              new Error("the second create was admitted; its apply is queued behind the first"),
            ),
          1000,
        ),
      ),
    ]);
    expect(second.status).toBe(409);

    facet.release(key);
    expect((await first).status).toBe(201);
  });

  it("reports the cluster's revision on a namespaced read, not the facet's", async () => {
    // A facet's MAX(id) is whatever revision last touched that namespace. A
    // client that LISTs and then watches from what it was told needs a number
    // from the log that assigns them.
    const first = await create("v1");
    await storeInsert(
      sql,
      host,
      "/registry/namespaces/later",
      true,
      false,
      0,
      0,
      0,
      new TextEncoder().encode("{}").buffer as ArrayBuffer,
      null,
    );
    const { rev } = await storeGetCurrent(sql, host, key, false);
    expect(rev).toBeGreaterThan(first);
    expect(rev).toBe((await storeReplay(sql, host, "/registry/pods/", first)).bookmark);
  });
});
