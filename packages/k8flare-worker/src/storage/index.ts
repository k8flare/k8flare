import { SCHEMA, LIST_SQL } from "./schema.ts";
import { prefixEnd, base64ToArrayBuffer, jsonResponse } from "./helpers.ts";
import { currentRevision, type SqlExec } from "./queries.ts";
import { pumpTrace } from "../trace.ts";
import { handleReplay, broadcastEvent, type WatchHost } from "./watch.ts";
import { storeGetCurrent, storeInsert, storeList } from "./store.ts";
export { WatchHub } from "./watchhub.ts";

// Phase 5 deleted this alarm loop's other three passes -- scheduler.ts
// (PodCIDR allocation), endpoints.ts (Endpoints/EndpointSlice from
// Service+Pod), and nodelifecycle.ts (Lease-staleness -> Unknown+taint+evict)
// -- on the premise that the real kube-controller-manager's
// nodeipam/endpoint/endpointslice/nodelifecycle/taint-eviction-controller
// controllers, WASM-resident in the kcm dynamic worker, would replace them.
// docs/platform-verification.md's Phase 5 findings then measured that
// premise as wrong on size and the three passes came back as synchronous
// Go reconciles in pkg/apiserver (endpoints.go, nodecidr.go,
// nodelifecycle.go). As of 2026-09-09 the original premise holds after
// all: all five real controllers run in the kcm dynamic worker
// (pkg/controllers/controllermanager.go) at +1.87MiB, and the three Go
// stand-ins are deleted -- see docs/platform-verification.md S28.
//
// ClusterIP allocation (pkg/apiserver/clusterip.go) is the one that stays
// synchronous inside apiserver's Go WASM binary -- real kube-apiserver
// allocates it in its own registry, so there is no controller to hand it
// to -- and this alarm loop gains no work for it. It used to also carry a
// TS-side backstop here (serviceip.ts, ridden on this same alarm) for the
// case where a Service's ClusterIP was somehow never assigned
// synchronously; deleted 2026-07-10 as a v3-design cleanup once it was
// confirmed dead on every reachable path (Go's create path always
// allocates synchronously) -- see git history for serviceip.ts if this
// ever needs resurrecting.
//
// Node lifecycle is the one controller input with no write to hook a poke
// to: staleness is detected by the ABSENCE of an expected Lease renewal.
// So while a Node is live, every safety-net tick sends the Controllers DO
// its own alarm-origin poke (see alarm() below), giving the real
// nodelifecycle controller a pump window in which to notice -- an
// event-armed safety net (armed by any Node write, see
// needsNodeLifecycleAttention; parked once no Node is left), not a fixed
// polling loop.
const SAFETY_NET_INTERVAL_MS = 60_000;
// How long to wait after a write that needs node-lifecycle attention
// before waking the alarm, so a burst of writes coalesces into a single
// pass.
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
  "/registry/replicationcontrollers/",
  "/registry/deployments/",
  "/registry/daemonsets/",
  "/registry/jobs/",
  "/registry/cronjobs/",
  // The cluster operator's input. Deliberately NOT "/registry/secrets/":
  // the operator's own Secret writes would then poke the pump that
  // re-drives its reconcile (docs/cluster-api-design.md's "poke feedback
  // prevention"), and no other controller here reads Secrets.
  "/registry/clusters/",
];

function needsControllersPing(key: string): boolean {
  return CONTROLLER_RELEVANT_PREFIXES.some((prefix) => key.startsWith(prefix));
}

/**
 * Whether writing this key should poke workers/nodes'
 * cf-containers-scheduler (the per-Pod microVM NodeVM lifecycle manager
 * -- it no longer binds Pods, see nodes/scheduler.ts's doc comment):
 * pod writes only -- a Pod admission already pinned to a not-yet-booted
 * Node is its work queue, and pod status/deletion drives VM teardown.
 * Same event-armed shape as needsControllersPing; its own safety-net
 * alarm parks when it tracks nothing.
 */
function needsNodesPing(key: string): boolean {
  return key.startsWith("/registry/pods/");
}

/**
 * Whether writing this key means the node-lifecycle safety net (see
 * alarm()) should be pulled in to run soon (see armSafetyNetSoon).
 * Coarse (any write under nodes/, not just a brand-new Node): a
 * freshly-registered Node won't be stale for at least
 * pkg/controllers/controllermanager.go's nodeMonitorGracePeriod, so in
 * practice this mostly just guarantees the safety net is armed at all
 * once a cluster has its first Node -- without this, a cluster's freshly
 * created first Node would stay parked forever and never notice that
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
 * Whether the safety-net alarm should stay armed: the real nodelifecycle
 * controller's Lease-staleness check needs a live Node to ever have
 * anything to look at (see alarm() below -- it has no event to wake it
 * otherwise, so it rides this same alarm). A cluster with no Nodes has
 * nothing left for the safety net to do -- it parks (cost invariants
 * #1/#3: no alarm chain on an idle cluster).
 */
function hasPendingSafetyNetWork(sql: SqlExec): boolean {
  return hasLiveKeyUnderPrefix(sql, "/registry/nodes/");
}

/** Minimal ctx shape Cluster needs: facets (FacetHost) plus DO storage/alarm access. */
interface ClusterContext {
  id?: { name?: string };
  facets: {
    get(name: string, factory: () => { class: any }): { fetch(req: Request): Promise<Response> };
    delete(name: string): void;
  };
  storage: {
    sql: SqlExec;
    setAlarm(ms: number): void;
    getAlarm(): Promise<number | null>;
    deleteAlarm(): Promise<void>;
    deleteAll(): Promise<void>;
    get(key: string): Promise<unknown>;
    put(key: string, value: unknown): Promise<void>;
    delete(key: string): Promise<boolean>;
  };
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
    this.host = { env, ctx, doName: ctx.id?.name ?? "default" };
    this.initialized = false;
  }

  private initialize(): void {
    if (this.initialized) return;
    for (const stmt of SCHEMA) {
      this.sql.exec(stmt);
    }
    this.initialized = true;
    // Only arm the safety net if there's already something for it to watch
    // (e.g. this DO woke from hibernation/eviction with a live Node from
    // before). A genuinely fresh/empty cluster stays parked until its
    // first write arms it via wakeSchedulerSoon.
    if (hasPendingSafetyNetWork(this.sql)) {
      this.ctx.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }
  }

  /**
   * Pull the safety-net alarm in to fire soon if it isn't already due
   * sooner, so node-lifecycle attention (see needsNodeLifecycleAttention)
   * gets handled promptly instead of waiting for the next safety-net
   * resync. Never pushes the alarm further out.
   */
  private async armSafetyNetSoon(): Promise<void> {
    const target = Date.now() + DEBOUNCE_MS;
    const current = await this.ctx.storage.getAlarm();
    if (current === null || current > target) {
      this.ctx.storage.setAlarm(target);
    }
  }

  /** This DO's own instance name IS the cluster identity ("default" or
   * "<id>@<uid>") -- sibling DOs of the same cluster share it. */
  private selfName(): string {
    return this.ctx.id?.name ?? "default";
  }

  /**
   * Fire-and-forget ping to the Controllers DO (see
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
  /** Post-write side effects shared by the create/update/delete paths:
   * arm the safety-net alarm and poke the controller/node reconcilers
   * when the written key warrants it.
   *
   * Pings are DOUBLY delivered: a detached fast-path promise (usually
   * lands within milliseconds) plus a persisted pending-ping flag backed
   * by this DO's alarm. The detached promise alone was NOT reliable in
   * production: it is torn down with the request's IoContext often
   * enough that an idle cluster's Cluster create sat unreconciled for
   * 15+ minutes with ZERO Controllers-DO invocations in wrangler tail
   * (2026-07-28, stall2 capture) -- warm clusters never showed it only
   * because a previous cycle's alarm was still armed to sweep the loss.
   * The alarm redelivery closes that hole while staying event-armed:
   * the flag exists only after a relevant write, and the alarm parks
   * again once delivery succeeds. */
  private async afterWrite(key: string, revision: number): Promise<void> {
    pumpTrace(this.env, "commit", "storage", { o: key, rv: revision });
    if (needsNodeLifecycleAttention(key)) await this.armSafetyNetSoon();
    if (needsControllersPing(key)) {
      await this.ctx.storage.put("pendingPing:controllers", true);
      await this.armSafetyNetSoon();
      void this.pingControllers();
    }
    if (needsNodesPing(key)) {
      await this.ctx.storage.put("pendingPing:nodes", true);
      await this.armSafetyNetSoon();
      void this.pingNodes();
    }
  }

  /** Alarm-context redelivery of pings whose fast-path promise died with
   * its request. Awaited (an alarm has no caller to cancel it); a ping
   * clears its own pending flag only on success, so a failed delivery
   * stays armed for the next tick. */
  private async deliverPendingPings(): Promise<boolean> {
    let pending = false;
    if (await this.ctx.storage.get("pendingPing:controllers")) {
      await this.pingControllers();
      if (await this.ctx.storage.get("pendingPing:controllers")) pending = true;
    }
    if (await this.ctx.storage.get("pendingPing:nodes")) {
      await this.pingNodes();
      if (await this.ctx.storage.get("pendingPing:nodes")) pending = true;
    }
    return pending;
  }

  private async pingControllers(): Promise<void> {
    const controllers = this.env.CONTROLLERS; // local DO binding post-consolidation
    if (!controllers) {
      // Not bound in some dev/test configs. Clear the flag rather than
      // leave it pending like a failed delivery would: there is nothing
      // for a later tick to retry, so keeping it re-arms the safety-net
      // alarm every 60s forever on an otherwise idle cluster (cost
      // invariants #1/#3). pingNodes does the same for SCHEDULER.
      await this.ctx.storage.delete("pendingPing:controllers");
      return;
    }
    if (this.env.KCM_DISABLED === "1") {
      await this.ctx.storage.delete("pendingPing:controllers"); // test kill switch
      return;
    }
    try {
      const stub = controllers.get(controllers.idFromName(this.selfName()));
      await stub.fetch("http://controllers.internal/");
      await this.ctx.storage.delete("pendingPing:controllers");
    } catch {
      // best-effort here; the pending flag keeps the alarm redelivering
    }
  }

  /** Alarm-origin sibling of pingControllers, for the node-lifecycle
   * safety net only: it opens a kcm pump window and nothing else. No
   * pending-ping flag either -- the next tick is 60s away for as long as
   * a Node is live, which is the retry. */
  private async pokeNodeLifecycle(): Promise<void> {
    const controllers = this.env.CONTROLLERS;
    if (!controllers) return; // not bound in some dev/test configs
    if (this.env.KCM_DISABLED === "1") return; // test kill switch
    try {
      const stub = controllers.get(controllers.idFromName(this.selfName()));
      await stub.fetch("http://controllers.internal/safety-net/node-lifecycle");
    } catch {
      // best-effort; the next safety-net tick retries
    }
  }

  /** Same best-effort contract as pingControllers, for the
   * cf-containers-scheduler -- a direct DO binding call post-consolidation
   * (no token needed: DOs are not publicly routable; the public /nodes
   * surface keeps its own token gate in nodes/index.ts). */
  private async pingNodes(): Promise<void> {
    const scheduler = this.env.SCHEDULER;
    if (!scheduler) {
      await this.ctx.storage.delete("pendingPing:nodes"); // not bound in some dev/test configs
      return;
    }
    try {
      const stub = scheduler.get(scheduler.idFromName(this.selfName()));
      await stub.fetch("http://nodes.internal/");
      await this.ctx.storage.delete("pendingPing:nodes");
    } catch {
      // best-effort here; the pending flag keeps the alarm redelivering
    }
  }

  async fetch(request: Request): Promise<Response> {
    this.initialize();
    const url = new URL(request.url);
    const path = url.pathname;

    try {
      // Cluster teardown (clusters/api.ts): delete every facet this DO
      // owns (ns/<name> per stored Namespace + events-log + ca-vault),
      // park the alarm, drop all parent storage. Facet deletion here is
      // safe from the name-reuse wedge (see handleDelete below): a
      // cluster's DO name embeds its uid, so a recreated cluster id gets
      // a brand-new DO, never a recycled facet name. Idempotent.
      if (path === "/admin/destroy" && request.method === "POST") {
        const { kvs } = await storeList(this.sql, this.host, "/registry/namespaces/", 0, 0);
        const facets = [
          ...kvs.map((kv: any) => `ns/${String(kv.key).slice("/registry/namespaces/".length)}`),
          "events-log",
          "ca-vault",
        ];
        for (const name of facets) {
          try {
            this.ctx.facets.delete(name);
          } catch (err) {
            console.log(`cluster destroy: facet ${name}: ${err}`);
          }
        }
        await this.ctx.storage.deleteAlarm();
        await this.ctx.storage.deleteAll();
        this.initialized = false;
        return jsonResponse({ destroyed: true, facets: facets.length });
      }

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
      await this.afterWrite(key, id);
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
      await this.afterWrite(key, id);
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
    await this.afterWrite(key, id);

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
    // Lease staleness has no write to arm a poke from (see this file's
    // header comment), so while a Node is live every tick opens one kcm
    // pump window: the real nodelifecycle controller in the kcm dynamic
    // worker only makes progress inside such a window. Deliberately NOT
    // the write path's pending-ping/pingControllers route -- that one is
    // a write-origin poke, and the Controllers DO answers it by arming
    // its own warmup window and 60s alarm chain, which an idle BYO node
    // must not do (see the /safety-net/node-lifecycle path in
    // controllers/index.ts and its alarm()'s comment on that regression).
    if (hasPendingSafetyNetWork(this.sql)) {
      await this.pokeNodeLifecycle();
    }
    const undelivered = await this.deliverPendingPings();
    // Re-arm the safety net if there's still a live Node that needs
    // ongoing Lease-staleness monitoring, or an undelivered ping to
    // retry; otherwise park (no alarm chain on an idle cluster -- cost
    // invariants #1/#3). A write needing sooner attention than the next
    // safety-net tick pulls this in via armSafetyNetSoon.
    if (undelivered || hasPendingSafetyNetWork(this.sql)) {
      this.ctx.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }
  }
}

// The DO classes here remain unreachable from public URLs -- nothing in
// gateway/index.ts routes raw storage paths to them.
