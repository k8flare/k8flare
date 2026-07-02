// Spike: S1 — DO Facets remaining verification questions.
//
// Throwaway code, not part of the production build. Exercises a real
// Supervisor DO + facets to answer (see FINDINGS.md for results):
//   1. Practical facet-count ceiling per parent DO.
//   2. Whether concurrent requests to sibling facets execute in parallel or
//      serialize on the parent DO's thread.
//   3. Whether ctx.storage.setAlarm() works inside a facet.
//   4. delete() semantics: recreate-after-delete, and in-flight requests
//      racing a delete().
//
// Prior finding (commit 46df0c0): a facet's `class` MUST come from the
// Dynamic Workers loader (env.LOADER...getDurableObjectClass(...)) — a
// plain statically-imported DurableObject subclass throws a StartupOptions
// TypeError. So the facet class here is loaded dynamically even though its
// source is a static string, to match that constraint.

import { DurableObject } from "cloudflare:workers";

interface Env {
  SUPERVISOR: DurableObjectNamespace<Supervisor>;
  // WorkerLoader — typed as any to avoid pulling in the experimental
  // workers-types subpath just for this spike.
  LOADER: any;
}

// Facet DO source, loaded dynamically via the LOADER (see note above for
// why this can't just be a statically-imported class).
const FACET_SOURCE = `
import { DurableObject } from "cloudflare:workers";

export class Facet extends DurableObject {
  async fetch(req) {
    const url = new URL(req.url);

    if (url.pathname === "/ping") {
      const sleepMs = Number(url.searchParams.get("sleepMs") || "0");
      if (sleepMs > 0) await new Promise((r) => setTimeout(r, sleepMs));
      let constructedAt = await this.ctx.storage.get("constructedAt");
      const freshlyConstructed = constructedAt === undefined;
      if (freshlyConstructed) {
        constructedAt = Date.now();
        await this.ctx.storage.put("constructedAt", constructedAt);
      }
      return Response.json({ ok: true, now: Date.now(), constructedAt, freshlyConstructed });
    }

    if (url.pathname === "/write" && req.method === "POST") {
      const body = await req.text();
      await this.ctx.storage.put("value", body);
      return Response.json({ ok: true });
    }

    if (url.pathname === "/read") {
      const value = await this.ctx.storage.get("value");
      return Response.json({ ok: true, value: value === undefined ? null : value });
    }

    if (url.pathname === "/set-alarm") {
      const ms = Number(url.searchParams.get("ms") || "2000");
      try {
        await this.ctx.storage.setAlarm(Date.now() + ms);
        const scheduled = await this.ctx.storage.getAlarm();
        return Response.json({ ok: true, scheduled });
      } catch (err) {
        return Response.json({ ok: false, error: String((err && err.message) || err) }, { status: 500 });
      }
    }

    if (url.pathname === "/alarm-status") {
      const fired = (await this.ctx.storage.get("alarmFired")) || false;
      const firedAt = await this.ctx.storage.get("alarmFiredAt");
      const scheduled = await this.ctx.storage.getAlarm();
      return Response.json({ ok: true, fired, firedAt: firedAt === undefined ? null : firedAt, scheduled });
    }

    if (url.pathname === "/sleep-slow") {
      const ms = Number(url.searchParams.get("ms") || "3000");
      await new Promise((r) => setTimeout(r, ms));
      return Response.json({ ok: true, slept: ms, at: Date.now() });
    }

    return Response.json({ ok: true, path: url.pathname });
  }

  async alarm() {
    await this.ctx.storage.put("alarmFired", true);
    await this.ctx.storage.put("alarmFiredAt", Date.now());
  }
}
`;

export class Supervisor extends DurableObject<Env> {
  private facetWorkerStub: any = null;

  private getFacetClass(): any {
    if (!this.facetWorkerStub) {
      this.facetWorkerStub = this.env.LOADER.get("s1-facets-facet-v1", () => ({
        compatibilityDate: "2026-03-24",
        mainModule: "facet.js",
        modules: { "facet.js": FACET_SOURCE },
      }));
    }
    return this.facetWorkerStub.getDurableObjectClass("Facet");
  }

  private facet(name: string): any {
    const cls = this.getFacetClass();
    return this.ctx.facets.get(name, () => ({ class: cls }));
  }

  async fetch(req: Request): Promise<Response> {
    const url = new URL(req.url);
    try {
      switch (url.pathname) {
        case "/":
          return Response.json({ ok: true, msg: "s1-facets supervisor" });
        case "/debug":
          return this.testDebug();
        case "/count":
          return await this.testCount(url);
        case "/parallel":
          return await this.testParallel(url);
        case "/alarm":
          return await this.testAlarm(url);
        case "/delete":
          return await this.testDelete(url);
        case "/delete-inflight":
          return await this.testDeleteInFlight(url);
        case "/delete-nonexistent":
          return this.testDeleteNonexistent(url);
        default:
          return Response.json({ ok: false, error: `unknown path ${url.pathname}` }, { status: 404 });
      }
    } catch (err: any) {
      return Response.json(
        { ok: false, error: String(err?.stack || err?.message || err) },
        { status: 500 },
      );
    }
  }

  // --- 1. Facet count ceiling -------------------------------------------
  // Creates (forces instantiation of, via /ping) `target` facets named
  // `${prefix}${startAt}`..`${prefix}${startAt+target-1}`, in concurrent
  // batches of `batch`. Re-running with a larger target and the same
  // prefix/startAt=0 is cheap for the already-created range (facets are
  // idempotently .get()-able by name).
  async testCount(url: URL): Promise<Response> {
    const target = Number(url.searchParams.get("target") || "100");
    const batchSize = Number(url.searchParams.get("batch") || "20");
    const startAt = Number(url.searchParams.get("startAt") || "0");
    const prefix = url.searchParams.get("prefix") || "c";

    let succeeded = 0;
    let firstError: string | null = null;
    let firstErrorAt = -1;
    let stoppedEarly = false;
    const t0 = Date.now();

    for (let i = startAt; i < startAt + target; i += batchSize) {
      const end = Math.min(i + batchSize, startAt + target);
      const idxs = Array.from({ length: end - i }, (_, j) => i + j);
      const results = await Promise.allSettled(
        idxs.map(async (idx) => {
          const stub = this.facet(`${prefix}${idx}`);
          const resp = await stub.fetch("http://facet.internal/ping");
          if (!resp.ok) throw new Error(`http ${resp.status}`);
          const body: any = await resp.json();
          if (!body.ok) throw new Error(`body not ok: ${JSON.stringify(body)}`);
          return idx;
        }),
      );
      let batchFailures = 0;
      for (const r of results) {
        if (r.status === "fulfilled") {
          succeeded++;
        } else {
          batchFailures++;
          if (firstError === null) {
            firstError = String((r as PromiseRejectedResult).reason?.message || (r as PromiseRejectedResult).reason);
            firstErrorAt = i;
          }
        }
      }
      if (batchFailures === idxs.length) {
        // Whole batch failed — stop early rather than burning through the
        // rest of `target` for no signal.
        stoppedEarly = true;
        break;
      }
    }

    return Response.json({
      ok: true,
      target,
      startAt,
      batchSize,
      succeeded,
      firstError,
      firstErrorAt,
      stoppedEarly,
      elapsedMs: Date.now() - t0,
    });
  }

  // --- 2. Facet execution parallelism -------------------------------------
  // Spins up N distinct facets, each with a /ping?sleepMs=<sleepMs> handler
  // that awaits setTimeout(sleepMs) before responding. If facets run truly
  // concurrently, total wall time stays close to sleepMs regardless of N.
  // If sibling facet requests serialize on the parent DO's thread, total
  // wall time grows toward N * sleepMs.
  async testParallel(url: URL): Promise<Response> {
    const n = Number(url.searchParams.get("n") || "10");
    const sleepMs = Number(url.searchParams.get("sleepMs") || "200");
    const prefix = url.searchParams.get("prefix") || "p";

    const t0 = Date.now();
    const results = await Promise.all(
      Array.from({ length: n }, (_, i) => i).map(async (i) => {
        const stub = this.facet(`${prefix}${i}`);
        const t1 = Date.now();
        const resp = await stub.fetch(`http://facet.internal/ping?sleepMs=${sleepMs}`);
        const body: any = await resp.json();
        return { i, tookMs: Date.now() - t1, ...body };
      }),
    );
    const totalElapsedMs = Date.now() - t0;

    return Response.json({
      ok: true,
      n,
      sleepMs,
      totalElapsedMs,
      // If parallel: ~sleepMs. If serialized: ~n * sleepMs.
      expectedIfSerializedMs: n * sleepMs,
      perFacetTookMs: results.map((r) => r.tookMs),
      results,
    });
  }

  // --- 3. Alarm inside a facet ---------------------------------------------
  async testAlarm(url: URL): Promise<Response> {
    const ms = Number(url.searchParams.get("ms") || "2000");
    const waitMs = Number(url.searchParams.get("waitMs") || "8000");
    const pollIntervalMs = Number(url.searchParams.get("pollIntervalMs") || "500");
    const name = url.searchParams.get("name") || `alarm-${Date.now()}`;

    const stub = this.facet(name);
    const setResp = await stub.fetch(`http://facet.internal/set-alarm?ms=${ms}`);
    const setBody: any = await setResp.json();
    if (!setBody.ok) {
      return Response.json({ ok: false, phase: "set-alarm", error: setBody.error });
    }

    const t0 = Date.now();
    let fired = false;
    let firedAt: number | null = null;
    let pollCount = 0;
    let lastScheduled: number | null = null;
    while (Date.now() - t0 < waitMs) {
      await new Promise((r) => setTimeout(r, pollIntervalMs));
      pollCount++;
      const statusResp = await stub.fetch("http://facet.internal/alarm-status");
      const status: any = await statusResp.json();
      lastScheduled = status.scheduled;
      if (status.fired) {
        fired = true;
        firedAt = status.firedAt;
        break;
      }
    }

    return Response.json({
      ok: true,
      name,
      setAlarmScheduled: setBody.scheduled,
      scheduledDelayMs: ms,
      fired,
      firedAt,
      observedDelayMs: firedAt ? firedAt - t0 : null,
      pollCount,
      pollIntervalMs,
      waitMs,
      lastScheduled,
    });
  }

  // --- 4. delete() semantics ------------------------------------------------
  async testDelete(url: URL): Promise<Response> {
    const name = url.searchParams.get("name") || `delete-${Date.now()}`;

    const stub1 = this.facet(name);
    await stub1.fetch("http://facet.internal/write", { method: "POST", body: "hello-before-delete" });
    const readBefore: any = await (await stub1.fetch("http://facet.internal/read")).json();
    const pingBefore: any = await (await stub1.fetch("http://facet.internal/ping")).json();

    this.ctx.facets.delete(name);

    const stub2 = this.facet(name);
    const readAfterDelete: any = await (await stub2.fetch("http://facet.internal/read")).json();
    const pingAfterDelete: any = await (await stub2.fetch("http://facet.internal/ping")).json();

    return Response.json({
      ok: true,
      name,
      readBefore,
      pingBefore,
      readAfterDelete,
      pingAfterDelete, // freshlyConstructed should be true if delete() truly wipes storage
    });
  }

  // In-flight request racing a delete() of the same facet.
  async testDeleteInFlight(url: URL): Promise<Response> {
    const name = url.searchParams.get("name") || `inflight-${Date.now()}`;
    const sleepMs = Number(url.searchParams.get("sleepMs") || "3000");
    const deleteAfterMs = Number(url.searchParams.get("deleteAfterMs") || "500");

    const stub = this.facet(name);
    // Make sure the facet actually exists (and has state) before racing the delete.
    await stub.fetch("http://facet.internal/write", { method: "POST", body: "pre-inflight" });

    const t0 = Date.now();
    const slowPromise = stub
      .fetch(`http://facet.internal/sleep-slow?ms=${sleepMs}`)
      .then(async (r: Response) => ({ ok: true, status: r.status, body: await r.json(), tookMs: Date.now() - t0 }))
      .catch((err: any) => ({ ok: false, error: String(err?.message || err), tookMs: Date.now() - t0 }));

    await new Promise((r) => setTimeout(r, deleteAfterMs));
    let deleteThrew: string | null = null;
    try {
      this.ctx.facets.delete(name);
    } catch (err: any) {
      deleteThrew = String(err?.message || err);
    }
    const deleteCalledAtMs = Date.now() - t0;

    const slowResult = await slowPromise;

    const stub2 = this.facet(name);
    const postDeletePing: any = await (await stub2.fetch("http://facet.internal/ping")).json();
    const postDeleteRead: any = await (await stub2.fetch("http://facet.internal/read")).json();

    return Response.json({
      ok: true,
      name,
      sleepMs,
      deleteAfterMs,
      deleteThrew,
      deleteCalledAtMs,
      slowResult,
      postDeletePing,
      postDeleteRead,
    });
  }

  testDebug(): Response {
    const stub = this.env.LOADER.get("s1-facets-facet-v1", () => ({
      compatibilityDate: "2026-03-24",
      mainModule: "facet.js",
      modules: { "facet.js": FACET_SOURCE },
    }));
    const props: string[] = [];
    let obj = stub;
    while (obj) {
      for (const k of Object.getOwnPropertyNames(obj)) props.push(k);
      obj = Object.getPrototypeOf(obj);
      if (obj === Object.prototype || obj === null) break;
    }

    const facetsProps: string[] = [];
    let fobj: any = this.ctx.facets;
    while (fobj) {
      for (const k of Object.getOwnPropertyNames(fobj)) facetsProps.push(k);
      fobj = Object.getPrototypeOf(fobj);
      if (fobj === Object.prototype || fobj === null) break;
    }

    return Response.json({
      ok: true,
      stubType: typeof stub,
      stubKeys: Object.keys(stub),
      props,
      hasGetDurableObjectClass: typeof stub.getDurableObjectClass,
      hasGetEntrypoint: typeof stub.getEntrypoint,
      hasCtxFacets: typeof this.ctx.facets,
      facetsProps,
      hasFacetsGet: typeof this.ctx.facets?.get,
      hasFacetsAbort: typeof this.ctx.facets?.abort,
      hasFacetsDelete: typeof this.ctx.facets?.delete,
    });
  }

  testDeleteNonexistent(url: URL): Response {
    const name = url.searchParams.get("name") || `never-existed-${Date.now()}`;
    let threw: string | null = null;
    try {
      this.ctx.facets.delete(name);
    } catch (err: any) {
      threw = String(err?.message || err);
    }
    return Response.json({ ok: true, name, threw });
  }
}

export default {
  async fetch(req: Request, env: Env): Promise<Response> {
    const id = env.SUPERVISOR.idFromName("default");
    const stub = env.SUPERVISOR.get(id);
    return stub.fetch(req);
  },
};
