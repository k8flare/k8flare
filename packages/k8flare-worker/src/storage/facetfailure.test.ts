// What a namespaced write does when its facet half is slow, fails, or
// outlives the instance that started it.
//
// A namespaced write commits to two storage systems with no transaction
// across them: the parent Cluster DO's log assigns the revision, the
// namespace facet takes the value. Every defect this path has had came from
// a reader deciding what a parent row with no facet row means -- a question
// with no answer, because an apply in flight and an apply that never
// happened look the same from one side. The write now lands in the parent
// WITH its value and is trimmed only once the facet acknowledges, so these
// tests assert outcomes a client can observe rather than the mechanism.
//
// The facet under test is the real thing: FACET_SOURCE is the module source
// that ships to the loader, evaluated here against node:sqlite rather than
// re-implemented. Writes go through Cluster.fetch, not a stand-in for
// handlePut, because a stand-in that drifts is exactly how a prev_revision
// defect stayed hidden through two reviews.
import { beforeEach, describe, expect, it, vi } from "vite-plus/test";
import { DatabaseSync } from "node:sqlite";

vi.mock("cloudflare:workers", () => ({ DurableObject: class {} }));

import { SCHEMA } from "./schema.ts";
import { FACET_SOURCE } from "./facets.ts";
import type { FacetHost } from "./facets.ts";
import { storeInsert } from "./store.ts";
import type { SqlExec } from "./queries.ts";
import { Cluster } from "./index.ts";

function bindable(v: unknown): unknown {
  if (v instanceof ArrayBuffer) return new Uint8Array(v);
  if (typeof v === "boolean") return v ? 1 : 0;
  return v ?? null;
}

function sqlite(statements: readonly string[]): SqlExec {
  const db = new DatabaseSync(":memory:");
  for (const s of statements) db.exec(s);
  return {
    exec(query: string, ...params: unknown[]) {
      const stmt = db.prepare(query);
      const bound = params.map(bindable);
      const rows = /^\s*(INSERT|UPDATE|DELETE)/i.test(query)
        ? (stmt.run(...(bound as any)), [])
        : (stmt.all(...(bound as any)) as any[]);
      return { one: () => rows[0] as any, toArray: () => rows as any };
    },
  };
}

const FacetClass = new Function(
  "DurableObject",
  FACET_SOURCE.replace('import { DurableObject } from "cloudflare:workers";', "").replace(
    "export class Facet",
    "class Facet",
  ) + "\nreturn Facet;",
)(class {});

class FakeFacet {
  /** Refuse applies outright, the way an unreachable facet does. */
  failApply = false;
  /** Apply for real, then lose the acknowledgement to a retryable reset. */
  loseApplyAck = false;
  /** Apply for real, then lose it to something facetFetch will not retry. */
  loseApplyAckPermanently = false;
  applies = 0;
  /** Every revision this facet was asked to supply, in order. */
  rowsRequested: number[][] = [];
  private readonly holds = new Map<string, { blocked: Promise<void>; release: () => void }>();
  private readonly inner: any;

  constructor() {
    this.inner = new FacetClass({ storage: { sql: sqlite([]) } }, {});
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
    const held = this.holds.get(key) ?? this.taken.get(key);
    this.holds.delete(key);
    this.taken.delete(key);
    held?.release();
  }
  private readonly taken = new Map<string, { blocked: Promise<void>; release: () => void }>();

  /** Resolves once the write for `key` has reached its apply, so its row is committed. */
  whenApplyStarted(key: string): Promise<void> {
    return new Promise((resolve) => this.started.set(key, resolve));
  }
  private readonly started = new Map<string, () => void>();

  /** Forget an acknowledged write, to prove a read fails loudly rather than serving nothing. */
  forgetAll(): void {
    this.inner.sql.exec("DELETE FROM kine");
  }

  async fetch(req: Request): Promise<Response> {
    const path = new URL(req.url).pathname;
    if (path === "/rows") {
      const text = await req.text();
      this.rowsRequested.push((JSON.parse(text) as { ids: number[] }).ids);
      // Holds the FIRST fill only, so a later request in the same test can
      // still get through and create the interleaving under test.
      const held = this.holds.get("/rows");
      if (held) {
        this.holds.delete("/rows");
        this.taken.set("/rows", held);
        await held.blocked;
      }
      return this.inner.fetch(
        new Request(req.url, { method: "POST", headers: req.headers, body: text }),
      );
    }
    const isApply = path === "/apply";
    let forward = req;
    if (isApply) {
      this.applies++;
      // Read the body once and rebuild: cloning a Request tees its stream,
      // and a tee whose other branch nobody drains does not finish here.
      const text = await req.text();
      forward = new Request(req.url, { method: "POST", headers: req.headers, body: text });
      const applyKey = (JSON.parse(text) as { key: string }).key;
      this.started.get(applyKey)?.();
      this.started.delete(applyKey);
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

interface Fetcher {
  fetch(req: Request): Promise<Response>;
}

const POD = "/registry/pods/default/web";
const PREFIX = "/registry/pods/";

describe("a namespaced write whose facet half is slow, fails, or outlives its instance", () => {
  let sql: SqlExec;
  let facet: FakeFacet;
  let cluster: Fetcher;
  let host: FacetHost;

  /** Build a Cluster over the given log. A second one over the same log is a restart. */
  function attach(): Fetcher {
    const ctx = {
      id: { name: "default" },
      facets: { get: () => facet, delete: () => {} },
      storage: clusterStorage(sql),
    };
    const env = { LOADER: { get: () => ({ getDurableObjectClass: () => FacetClass }) } };
    host = { env, ctx } as unknown as FacetHost;
    return new Cluster(ctx, env) as unknown as Fetcher;
  }

  beforeEach(() => {
    sql = sqlite(SCHEMA);
    facet = new FakeFacet();
    cluster = attach();
  });

  const body = (v: string) => JSON.stringify({ value: btoa(v) });

  function create(key: string, value: string, on: Fetcher = cluster): Promise<Response> {
    return on.fetch(
      new Request(`http://do.internal/key${key}`, { method: "PUT", body: body(value) }),
    );
  }
  function update(key: string, value: string, revision: number): Promise<Response> {
    return cluster.fetch(
      new Request(`http://do.internal/key${key}`, {
        method: "PUT",
        body: JSON.stringify({ value: btoa(value), revision }),
      }),
    );
  }
  function remove(key: string): Promise<Response> {
    return cluster.fetch(new Request(`http://do.internal/key${key}`, { method: "DELETE" }));
  }
  async function get(key: string, on: Fetcher = cluster): Promise<any> {
    return (await on.fetch(new Request(`http://do.internal/key${key}`))).json<any>();
  }
  async function list(prefix: string, on: Fetcher = cluster): Promise<any> {
    return (await on.fetch(new Request(`http://do.internal/list${prefix}`))).json<any>();
  }
  async function replay(prefix: string, revision: number, on: Fetcher = cluster): Promise<any> {
    const url = new URL("/replay", "http://do.internal");
    url.searchParams.set("prefix", prefix);
    url.searchParams.set("revision", String(revision));
    return (await on.fetch(new Request(url.toString()))).json<any>();
  }
  async function revisionOf(resp: Response): Promise<number> {
    return (await resp.json<any>()).revision;
  }

  it("stands, and is readable, when the facet refuses it", async () => {
    facet.failApply = true;
    const rev = await revisionOf(await create(POD, "v1"));

    expect((await get(POD)).kv.value).toBe(btoa("v1"));
    expect((await list(PREFIX)).kvs.map((k: any) => k.value)).toEqual([btoa("v1")]);
    expect((await replay(PREFIX, rev - 1)).events.map((e: any) => e.kv.value)).toEqual([
      btoa("v1"),
    ]);
  });

  it("hands the value to the facet and stops holding it", async () => {
    // The offload is the point of the facet: without this the parent keeps
    // every value and a namespace's data never leaves its 10GB budget.
    const rev = await revisionOf(await create(POD, "v1"));
    const parent = sql.exec("SELECT value, old_value FROM kine WHERE id = ?1", rev).toArray();
    expect(parent[0].value).toBeNull();
    expect(parent[0].old_value).toBeNull();
    expect((await get(POD)).kv.value).toBe(btoa("v1"));
  });

  it("lets the next write to that key through", async () => {
    facet.failApply = true;
    const rev = await revisionOf(await create(POD, "v1"));
    facet.failApply = false;

    expect((await update(POD, "v2", rev)).status).toBe(200);
    expect((await get(POD)).kv.value).toBe(btoa("v2"));
  });

  it("reports success for a write whose acknowledgement was lost after it landed", async () => {
    facet.loseApplyAckPermanently = true;
    const rev = await revisionOf(await create(POD, "v1"));

    expect((await get(POD)).kv.modRevision).toBe(rev);
    expect((await replay(PREFIX, rev - 1)).events.map((e: any) => e.kv.value)).toEqual([
      btoa("v1"),
    ]);
  });

  it("survives the retry that follows a platform reset", async () => {
    facet.loseApplyAck = true;
    const rev = await revisionOf(await create(POD, "v1"));

    expect(facet.applies).toBe(2);
    expect((await get(POD)).kv.modRevision).toBe(rev);
  });

  it("serves a write whose apply is still in flight", async () => {
    facet.hold(POD);
    const started = facet.whenApplyStarted(POD);
    const inflight = create(POD, "v1");
    await started;

    expect((await get(POD)).kv.value).toBe(btoa("v1"));
    expect((await list(PREFIX)).kvs).toHaveLength(1);

    facet.release(POD);
    await inflight;
  });

  it("delivers every event after the bookmark it handed out", async () => {
    // The umbrella invariant. A client takes its resume point from a fresh
    // replay, then asks for everything after it; whatever happened in
    // between -- held applies, refused applies, a delete -- must appear.
    await create("/registry/pods/default/a", "a1");
    const seed = await replay(PREFIX, 0);
    const bookmark: number = seed.bookmark;

    facet.hold("/registry/pods/default/b");
    const held = create("/registry/pods/default/b", "b1");
    facet.failApply = true;
    const refused = await revisionOf(await create("/registry/pods/default/c", "c1"));
    facet.failApply = false;
    facet.release("/registry/pods/default/b");
    const heldRevision = await revisionOf(await held);
    const removed = await revisionOf(await remove("/registry/pods/default/a"));

    const delta = await replay(PREFIX, bookmark);
    const seen = delta.events
      .map((e: any) => e.kv.modRevision)
      .sort((x: number, y: number) => x - y);
    expect(seen).toEqual([heldRevision, refused, removed].sort((x, y) => x - y));
    for (const e of delta.events) expect(e.kv.value).not.toBeNull();
  });

  it("does not lose a write that lands between a list's two halves", async () => {
    // The list reads the parent, then asks facets for the values it handed
    // over. A write committing in that gap must still be reachable by
    // watching from the revision the list reported.
    await create("/registry/pods/default/a", "a1");
    const listed = await list(PREFIX);

    const later = await revisionOf(await create("/registry/pods/default/b", "b1"));
    expect(later).toBeGreaterThan(listed.revision);

    const delta = await replay(PREFIX, listed.revision);
    expect(delta.events.map((e: any) => e.kv.modRevision)).toContain(later);
  });

  it("admits exactly one of two concurrent creates", async () => {
    facet.hold(POD);
    const first = create(POD, "v1");
    const second = await Promise.race([
      create(POD, "also-v1"),
      new Promise<Response>((_, reject) =>
        setTimeout(() => reject(new Error("the second create was admitted")), 1000),
      ),
    ]);
    expect(second.status).toBe(409);

    facet.release(POD);
    expect((await first).status).toBe(201);
  });

  it("keeps a create's compare-and-swap token still while another apply resolves", async () => {
    // Both racers must derive the SAME prev_revision, so it cannot be
    // anything that moves between their two reads.
    const other = "/registry/pods/default/other";
    facet.hold(other);
    const otherWrite = create(other, "other");

    facet.hold(POD);
    const first = create(POD, "v1");

    facet.release(other);
    expect((await otherWrite).status).toBe(201);

    const second = await Promise.race([
      create(POD, "also-v1"),
      new Promise<Response>((_, reject) =>
        setTimeout(() => reject(new Error("the second create was admitted")), 1000),
      ),
    ]);
    expect(second.status).toBe(409);

    facet.release(POD);
    expect((await first).status).toBe(201);
  });

  it("survives the instance dying between the write and the acknowledgement", async () => {
    facet.failApply = true;
    const rev = await revisionOf(await create(POD, "v1"));
    facet.failApply = false;

    const restarted = attach();
    expect((await get(POD, restarted)).kv.value).toBe(btoa("v1"));
    expect((await replay(PREFIX, rev - 1, restarted)).events).toHaveLength(1);
    expect((await list(PREFIX, restarted)).kvs).toHaveLength(1);
  });

  it("does not serve a key twice when both halves hold it", async () => {
    const rev = await revisionOf(await create(POD, "v1"));
    const restarted = attach();
    const listed = await list(PREFIX, restarted);
    expect(listed.kvs).toHaveLength(1);
    expect(listed.kvs[0].modRevision).toBe(rev);
  });

  it("fails loudly if a facet lost a write it acknowledged", async () => {
    await create(POD, "v1");
    facet.forgetAll();
    expect((await cluster.fetch(new Request(`http://do.internal/key${POD}`))).status).toBe(500);
  });

  it("refuses a namespaced row that carries nothing to tell apart from a handed-over one", async () => {
    await expect(storeInsert(sql, host, POD, true, false, 0, 0, 0, null, null)).rejects.toThrow(
      /neither a value nor a previous one/,
    );
  });

  it("reads the revision the parent chose, not the facet's newest", async () => {
    // The read picks a row from the parent, then asks the facet for it. If it
    // asked by key instead of by revision, an update landing in that gap
    // would make the facet answer about a different row and the read would
    // report the chosen one as lost.
    const first = await revisionOf(await create(POD, "v1"));
    facet.hold("/rows");
    const reading = get(POD);
    await update(POD, "v2", first);
    facet.release("/rows");

    const got = await reading;
    expect(got.kv.value).toBe(btoa("v1"));
    expect(facet.rowsRequested.flat()).toContain(first);
  });

  it("lists a past revision as it was", async () => {
    const first = await revisionOf(await create(POD, "v1"));
    await update(POD, "v2", first);

    const at = (
      await cluster.fetch(new Request(`http://do.internal/list${PREFIX}?revision=${first}`))
    ).json<any>();
    expect((await at).kvs.map((k: any) => k.value)).toEqual([btoa("v1")]);
  });

  it("asks a facet only for the rows the caller actually wants", async () => {
    for (const name of ["a", "b", "c", "d"]) await create(`/registry/pods/default/${name}`, name);
    facet.rowsRequested = [];
    const limited = await (
      await cluster.fetch(new Request(`http://do.internal/list${PREFIX}?limit=2`))
    ).json<any>();
    expect(limited.kvs).toHaveLength(2);
    expect(facet.rowsRequested.flat()).toHaveLength(2);
  });

  it("broadcasts one revision without dragging in whatever else committed", async () => {
    // broadcastEvent reads the parent's log from just below the committed
    // revision, so anything that commits while this write's apply is in
    // flight lands in the same range. Filling before narrowing pulls those
    // rows -- from other namespaces, whose facets can fail independently --
    // into a write that has nothing to do with them.
    const pod = "/registry/pods/default/held";
    facet.hold(pod);
    const started = facet.whenApplyStarted(pod);
    const held = create(pod, "held");
    await started;

    const later = await revisionOf(await create("/registry/pods/other/x", "x1"));

    facet.rowsRequested = [];
    facet.release(pod);
    expect((await held).status).toBe(201);

    expect(facet.rowsRequested.flat()).not.toContain(later);
  });

  it("never asks a facet for more revisions than workerd will bind", async () => {
    // The local SQLite binds as many parameters as asked; workerd stops at
    // 100. The harness cannot reproduce that, so the invariant is asserted
    // on the request instead of on the failure it would cause.
    for (let i = 0; i < 150; i++) await create(`/registry/pods/default/p${i}`, `v${i}`);
    facet.rowsRequested = [];
    const listed = await list(PREFIX);
    expect(listed.kvs).toHaveLength(150);
    expect(facet.rowsRequested.length).toBeGreaterThan(1);
    for (const batch of facet.rowsRequested) expect(batch.length).toBeLessThanOrEqual(100);
  });
});
