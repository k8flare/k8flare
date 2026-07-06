import { SCHEMA, LIST_SQL } from "./schema.ts";
import { apiserverFetch } from "../loader/apiserver.ts";
import { prefixEnd, base64ToArrayBuffer, jsonResponse } from "./helpers.ts";
import { currentRevision, type SqlExec } from "./queries.ts";
import { handleReplay, broadcastEvent, type WatchHost } from "./watch.ts";
import { storeGetCurrent, storeInsert, storeList } from "./store.ts";
import { allocateClusterIPs, needsServiceIPAttention } from "./serviceip.ts";
export { WatchHub } from "./watchhub.ts";

// serviceip.ts's ClusterIP allocation wakes on-demand (see
// armSafetyNetSoon) whenever a write needs its attention, so this is only
// a safety net for a missed trigger — not the primary mechanism. Keeping
// it long keeps idle clusters cheap.
//
// Phase 5 deleted this alarm loop's other three passes -- scheduler.ts
// (PodCIDR allocation), endpoints.ts (Endpoints/EndpointSlice from
// Service+Pod), and nodelifecycle.ts (Lease-staleness -> Unknown+taint+evict)
// -- on the premise that the real kube-controller-manager's
// nodeipam/endpoint/endpointslice/nodelifecycle/taint-eviction-controller
// controllers, WASM-resident in workers/controllers, would replace them.
// docs/platform-verification.md's Phase 5 findings later established that
// premise was wrong: kube-scheduler can't compile for GOOS=js/wasm at all,
// and KCM's real controller packages -- even without scheduler -- blow this
// project's WASM size budget by far more than the available margin (any one
// real controller costs +6MiB+ beyond an already-near-budget client-go
// base). Both remain host-process/BYO-VM-only; workers/controllers cannot
// currently host either for a deployed (non-BYO-VM) cluster.
//
// PodCIDR allocation (pkg/apiserver/nodecidr.go) and Endpoints/EndpointSlice
// (pkg/apiserver/endpoints.go) were restored as synchronous reconciles
// inside apiserver's Go WASM binary instead -- the same design ClusterIP
// allocation below already used (Phase 3), so this alarm loop doesn't gain
// any new work for either. ClusterIP allocation itself *did* eventually
// move to apiserver too (pkg/apiserver/clusterip.go, Phase 3, despite this
// comment previously and incorrectly claiming otherwise -- honest
// correction, CLAUDE.md rule 4): allocateClusterIPs below is now dead code
// on any path that goes through the Go create path, kept rather than
// deleted (Phase 3's "concurrent agent owns workers/storage" boundary), and
// this safety net is genuinely just a backstop for the -- currently
// unreachable in practice -- case where a Service's ClusterIP was somehow
// never assigned synchronously.
//
// Node lifecycle (Lease staleness -> Unknown+taint+evict,
// pkg/apiserver/nodelifecycle.go) could not move the same way: staleness is
// detected by the ABSENCE of an expected Lease renewal, so there's no write
// to hook a synchronous call to. It runs from this alarm instead, via a
// fire-and-forget ping to apiserver's
// POST /internal/reconcile-node-lifecycle (see reconcileNodeLifecycle
// below) -- the same event-armed-safety-net shape ClusterIP allocation
// already uses here, not a new polling mechanism.
const SAFETY_NET_INTERVAL_MS = 60_000;
// How long to wait after a write that needs ClusterIP allocation before
// waking the alarm, so a burst of writes coalesces into a single pass.
const DEBOUNCE_MS = 1_000;

// Coarse (prefix-only, no value inspection -- like the old
// needsEndpointsAttention) trigger for pinging workers/controllers: any
// write under one of these resource types is potentially relevant to the
// real kube-controller-manager's enabled controllers (see
// cmd/controller-manager/main.go's --controllers default). The real
// controllers watch continuously once running, so this ping only matters
// for first-ever startup and resurrection after a redeploy/panic/eviction
// reset the workers/controllers DO instance -- it does not need to be
// precise the way the old per-value needsSchedulerAttention checks were.
const CONTROLLER_RELEVANT_PREFIXES = [
  "/registry/nodes/",
  "/registry/pods/",
  "/registry/services/",
  "/registry/endpoints/",
  "/registry/endpointslices/",
  "/registry/leases/",
  "/registry/replicasets/",
  "/registry/deployments/",
  "/registry/daemonsets/",
  "/registry/jobs/",
  "/registry/cronjobs/",
];

function needsControllersPing(key: string): boolean {
  return CONTROLLER_RELEVANT_PREFIXES.some((prefix) => key.startsWith(prefix));
}

/**
 * Whether writing this key should poke workers/nodes'
 * cf-containers-scheduler (the per-Pod microVM binder): pod writes only
 * -- a new unscheduled pod is its work queue, and pod status/deletion
 * drives VM teardown. Same event-armed shape as needsControllersPing;
 * the binder's own safety-net alarm parks when it tracks nothing.
 */
function needsNodesPing(key: string): boolean {
  return key.startsWith("/registry/pods/");
}

/**
 * Whether writing this key means the node-lifecycle safety net (see
 * reconcileNodeLifecycle) should be pulled in to run soon, mirroring
 * needsServiceIPAttention's role for ClusterIP allocation. Coarse (any
 * write under nodes/, not just a brand-new Node): a freshly-registered Node
 * won't be stale for at least pkg/apiserver/nodelifecycle.go's
 * nodeMonitorGracePeriod, so in practice this mostly just guarantees the
 * safety net is armed at all once a cluster has its first Node -- without
 * this, a cluster with zero Services (nothing else arms the safety net) and
 * one freshly created Node would stay parked forever and never notice that
 * Node's Lease going stale later.
 */
function needsNodeLifecycleAttention(key: string): boolean {
  return key.startsWith("/registry/nodes/");
}

/** Cheap local existence check -- no facet round trip needed since a key's
 * envelope (including its `deleted` flag) always lives in the parent, even
 * for namespaced keys whose value lives in a facet (see store.ts). */
function hasLiveKeyUnderPrefix(sql: SqlExec, prefix: string): boolean {
  const q = LIST_SQL("AND mkv.name > ?4") + " LIMIT 1";
  const rows = sql.exec(q, prefix, prefixEnd(prefix), 0, "").toArray();
  return rows.length > 0;
}

/**
 * Whether the safety-net alarm should stay armed: serviceip.ts's ClusterIP
 * allocation backstop needs a live Service, and pkg/apiserver/
 * nodelifecycle.go's Lease-staleness reconcile needs a live Node to ever
 * have anything to check (see reconcileNodeLifecycle below -- it has no
 * event to wake it otherwise, so it rides this same alarm). A cluster with
 * neither has nothing left for the safety net to do -- it parks (cost
 * invariants #1/#3: no alarm chain on an idle cluster).
 */
function hasPendingSafetyNetWork(sql: SqlExec): boolean {
  return (
    hasLiveKeyUnderPrefix(sql, "/registry/services/") ||
    hasLiveKeyUnderPrefix(sql, "/registry/nodes/")
  );
}

/** Minimal ctx shape Cluster needs: facets (FacetHost) plus DO storage/alarm access. */
interface ClusterContext {
  facets: {
    get(name: string, factory: () => { class: any }): { fetch(req: Request): Promise<Response> };
    delete(name: string): void;
  };
  storage: { sql: SqlExec; setAlarm(ms: number): void; getAlarm(): Promise<number | null> };
}

export class Cluster {
  private ctx: ClusterContext;
  private env: any;
  private sql: SqlExec;
  private host: WatchHost;
  private initialized: boolean;

  constructor(ctx: any, env: any) {
    this.ctx = ctx;
    this.env = env;
    this.sql = ctx.storage.sql;
    this.host = { env, ctx };
    this.initialized = false;
  }

  private initialize(): void {
    if (this.initialized) return;
    for (const stmt of SCHEMA) {
      this.sql.exec(stmt);
    }
    this.initialized = true;
    // Only arm the safety net if there's already something for it to watch
    // (e.g. this DO woke from hibernation/eviction with live Nodes/Services
    // from before). A genuinely fresh/empty cluster stays parked until its
    // first write arms it via wakeSchedulerSoon.
    if (hasPendingSafetyNetWork(this.sql)) {
      this.ctx.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }
  }

  /**
   * Pull the safety-net alarm in to fire soon if it isn't already due
   * sooner, so a Service needing a ClusterIP gets handled promptly instead
   * of waiting for the next safety-net resync. Never pushes the alarm
   * further out.
   */
  private async armSafetyNetSoon(): Promise<void> {
    const target = Date.now() + DEBOUNCE_MS;
    const current = await this.ctx.storage.getAlarm();
    if (current === null || current > target) {
      this.ctx.storage.setAlarm(target);
    }
  }

  /**
   * Fire-and-forget ping to workers/controllers (see
   * CONTROLLER_RELEVANT_PREFIXES/needsControllersPing above), ensuring the
   * real kube-controller-manager it hosts is instantiated and its resident
   * reconcile loop running. Best-effort AND fire-and-forget: a failure
   * must not fail the write, and the write must NOT wait on the ping --
   * these used to be awaited inline, which meant every relevant write
   * held this DO's input gate for a full cross-worker round trip; under
   * bursts that serialized/starved every other request on this instance
   * and propagated caller cancellations into the ping chain (the
   * "GET / - Canceled" storms and minutes-long fast-500 wedge episodes
   * observed in production, see docs/platform-verification.md). The
   * detached promise keeps running past the response; errors are
   * swallowed here.
   */
  private async pingControllers(): Promise<void> {
    const controllers = this.env.CONTROLLERS; // local DO binding post-consolidation
    if (!controllers) return; // not bound in some dev/test configs
    if (this.env.KCM_DISABLED === "1") return; // test kill switch (see Env.KCM_DISABLED)
    try {
      const stub = controllers.get(controllers.idFromName("default"));
      await stub.fetch("http://controllers.internal/");
    } catch {
      // best-effort; see doc comment above
    }
  }

  /** Same best-effort contract as pingControllers, for the
   * cf-containers-scheduler -- a direct DO binding call post-consolidation
   * (no token needed: DOs are not publicly routable; the public /nodes
   * surface keeps its own token gate in nodes/index.ts). */
  private async pingNodes(): Promise<void> {
    const scheduler = this.env.SCHEDULER;
    if (!scheduler) return; // not bound in some dev/test configs
    try {
      const stub = scheduler.get(scheduler.idFromName("default"));
      await stub.fetch("http://nodes.internal/");
    } catch {
      // best-effort
    }
  }

  async fetch(request: Request): Promise<Response> {
    this.initialize();
    const url = new URL(request.url);
    const path = url.pathname;

    try {
      if (path === "/revision" && request.method === "GET") {
        return jsonResponse({ revision: currentRevision(this.sql) });
      }

      if (path === "/replay" && request.method === "GET") {
        return await handleReplay(this.host, this.sql, request);
      }

      if (path.startsWith("/key/")) {
        const key = "/" + path.slice(5);
        switch (request.method) {
          case "GET":
            return await this.handleGet(key);
          case "PUT":
            return await this.handlePut(key, await request.json());
          case "DELETE":
            return await this.handleDelete(key, parseInt(url.searchParams.get("revision") || "0"));
        }
      }

      if (path.startsWith("/list/") && request.method === "GET") {
        let prefix = "/" + path.slice(6);
        if (!prefix.endsWith("/")) prefix += "/";
        return await this.handleList(
          prefix,
          parseInt(url.searchParams.get("limit") || "0"),
          parseInt(url.searchParams.get("revision") || "0"),
        );
      }

      return new Response("Not found", { status: 404 });
    } catch (e: any) {
      if (e.message && e.message.includes("UNIQUE constraint failed")) {
        return jsonResponse({ error: "key already exists" }, 409);
      }
      return jsonResponse({ error: e.message || String(e) }, 500);
    }
  }

  private async handleGet(key: string): Promise<Response> {
    const { rev, event } = await storeGetCurrent(this.sql, this.host, key, false);
    if (!event || event.delete) return jsonResponse({ revision: rev, kv: null });
    return jsonResponse({ revision: rev, kv: event.kv });
  }

  private async handlePut(key: string, body: any): Promise<Response> {
    const value = body.value ? base64ToArrayBuffer(body.value) : null;
    const lease = body.lease || 0;
    const revision = body.revision || 0;

    if (revision === 0) {
      const { rev, event } = await storeGetCurrent(this.sql, this.host, key, true);
      let prevRevision = rev;
      if (event && !event.delete) return jsonResponse({ error: "key already exists" }, 409);
      if (event) prevRevision = event.kv.modRevision;
      const id = await storeInsert(
        this.sql,
        this.host,
        key,
        true,
        false,
        0,
        prevRevision,
        lease,
        value,
        null,
      );
      await broadcastEvent(this.host, this.sql, key, id);
      if (needsServiceIPAttention(key, value)) await this.armSafetyNetSoon();
      if (needsNodeLifecycleAttention(key)) await this.armSafetyNetSoon();
      if (needsControllersPing(key)) void this.pingControllers();
      if (needsNodesPing(key)) void this.pingNodes();
      return jsonResponse({ revision: id }, 201);
    } else {
      const { rev, event } = await storeGetCurrent(this.sql, this.host, key, false);
      if (!event || event.delete) return jsonResponse({ revision: rev, kv: null, updated: false });
      if (event.kv.modRevision !== revision)
        return jsonResponse({ revision: rev, kv: event.kv, updated: false }, 409);
      const oldValue = body.value ? base64ToArrayBuffer(event.kv.value) : null;
      const id = await storeInsert(
        this.sql,
        this.host,
        key,
        false,
        false,
        event.kv.createRevision,
        event.kv.modRevision,
        lease,
        value,
        oldValue,
      );
      const kv = {
        key,
        createRevision: event.kv.createRevision,
        modRevision: id,
        value: body.value,
        lease,
      };
      await broadcastEvent(this.host, this.sql, key, id);
      if (needsServiceIPAttention(key, value)) await this.armSafetyNetSoon();
      if (needsNodeLifecycleAttention(key)) await this.armSafetyNetSoon();
      if (needsControllersPing(key)) void this.pingControllers();
      if (needsNodesPing(key)) void this.pingNodes();
      return jsonResponse({ revision: id, kv, updated: true });
    }
  }

  private async handleDelete(key: string, revision: number): Promise<Response> {
    const { rev, event } = await storeGetCurrent(this.sql, this.host, key, true);
    if (!event) return jsonResponse({ revision: rev, kv: null, deleted: true });
    if (event.delete) return jsonResponse({ revision: rev, kv: event.kv, deleted: true });
    if (revision !== 0 && event.kv.modRevision !== revision)
      return jsonResponse({ revision: rev, kv: event.kv, deleted: false });
    const oldValue = event.kv.value ? base64ToArrayBuffer(event.kv.value) : null;
    const id = await storeInsert(
      this.sql,
      this.host,
      key,
      false,
      true,
      event.kv.createRevision,
      event.kv.modRevision,
      0,
      oldValue,
      oldValue,
    );
    await broadcastEvent(this.host, this.sql, key, id);
    if (needsServiceIPAttention(key, oldValue)) await this.armSafetyNetSoon();
    if (needsNodeLifecycleAttention(key)) await this.armSafetyNetSoon();
    if (needsControllersPing(key)) void this.pingControllers();
    if (needsNodesPing(key)) void this.pingNodes();

    // Namespace deletion does NOT call ctx.facets.delete() here, despite the
    // original plan calling for it as a GC nicety. Empirically reproduced
    // (2026-07-02, via repeated create/delete/recreate of the same namespace
    // name against real wrangler dev): deleting and recreating a facet under
    // the *same name* works for the first few cycles, then every subsequent
    // create through that facet name permanently returns a false "already
    // exists" (create sees a live row that a parallel delete-by-key call
    // reports as already gone -- an internal inconsistency, not a race:
    // reproduced deterministically at the 4th cycle, unaffected by adding
    // delays between requests). The same test with a fresh, never-reused
    // facet name every time never fails. This looks like an undocumented
    // platform limitation around repeated ctx.facets.delete()+get() cycles
    // on one name, not a bug in this file's logic -- S1's spike
    // (spikes/s1-facets/FINDINGS.md) only ever exercised a single
    // delete-then-recreate, not a repeated cycle.
    //
    // Correctness doesn't depend on the facet being destroyed: every
    // namespaced object is already individually tombstoned by the cascade
    // delete (pkg/apiserver/namespacedelete.go) before the Namespace object
    // itself is deleted, so storeGetCurrent's includeDeleted-aware check
    // already lets a same-named object be recreated correctly. Skipping the
    // facet-level delete only forgoes the storage-GC nicety, not
    // correctness -- kine's own log already carries tombstones indefinitely
    // in the same way. See docs/multi-tenancy-and-hosting.md's honest
    // correction for the full writeup and the production-verification
    // follow-up this leaves open.
    return jsonResponse({ revision: id, kv: event.kv, deleted: true });
  }

  private async handleList(prefix: string, limit: number, revision: number): Promise<Response> {
    const {
      revision: rev,
      count,
      kvs,
    } = await storeList(this.sql, this.host, prefix, limit, revision);
    return jsonResponse({ revision: rev, count, kvs });
  }

  async alarm(): Promise<void> {
    this.initialize();
    await allocateClusterIPs(this.host, this.sql);
    await this.reconcileNodeLifecycle();
    // Re-arm the safety net only if there's still a live Service or Node
    // that needs ongoing monitoring (ClusterIP allocation, Lease-staleness
    // detection); otherwise park (no alarm chain on an idle cluster -- cost
    // invariants #1/#3). A write needing sooner attention than the next
    // safety-net tick pulls this in via armSafetyNetSoon.
    if (hasPendingSafetyNetWork(this.sql)) {
      this.ctx.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }
  }

  /**
   * Fire-and-forget ping to apiserver's
   * POST /internal/reconcile-node-lifecycle (pkg/apiserver/
   * nodelifecycle.go's RegisterInternalHandlers), which detects Nodes whose
   * Lease has gone stale, marks them Unknown + taints them unreachable, and
   * evicts their Pods once stale for long enough. Runs on every safety-net
   * tick (like allocateClusterIPs above) rather than being event-triggered:
   * staleness is detected by the ABSENCE of an expected Lease renewal, so
   * there is no write to arm this from the way armSafetyNetSoon arms
   * ClusterIP allocation. Best-effort, same reasoning as pingControllers:
   * a failure here must not fail whatever write happened to trigger this
   * alarm tick, and the next tick (while hasPendingSafetyNetWork stays
   * true) retries.
   */
  private async reconcileNodeLifecycle(): Promise<void> {
    // Post-consolidation: straight to the apiserver dynamic worker
    // (loader/apiserver.ts) -- a DO-origin LOADER.get shares the same
    // loaded isolate (S19 G3). Requires LOADER/ASSETS/STORAGE on this
    // DO's env, absent in some dev/test configs.
    if (!this.env.LOADER || !this.env.ASSETS || !this.env.STORAGE) return;
    try {
      await apiserverFetch(
        this.env,
        new Request("http://apiserver.internal/internal/reconcile-node-lifecycle", {
          method: "POST",
        }),
      );
    } catch {
      // best-effort; see doc comment above
    }
  }
}

// (The former standalone-Worker default export is gone: post-consolidation
// this module only exports the Cluster/WatchHub DO classes, re-exported by
// ../index.ts. The DOs remain unreachable from public URLs -- nothing in
// gateway/index.ts routes raw storage paths to them.)
