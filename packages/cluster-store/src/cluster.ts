import { DurableObject } from "cloudflare:workers";

// Cluster holds one cluster's state: a kine-style revisioned key-value log
// in SQLite. Every write appends a row; the row id is the revision. A
// bootstrap row makes the first revision 1, as in kine, because a resource
// version of 0 is illegal for a list. Watchers are hibernatable WebSockets
// tagged with the key prefix they asked for.
const RETAINED_REVISIONS = 1000;
const COMPACT_BATCH = 200;
const WATCH_LEASE_MS = 360_000;
const NODE_LEASE_PREFIX = "/registry/leases/kube-node-lease/";
const LEASE_CHECK_DELAY_S = 60;
const LEASE_CHECK_EVERY_MS = 50_000;
const OUTBOX_BATCH = 100;
const MAX_DELAY_S = 86_400;

type Target = "scheduler" | "workloads" | "crds" | "gc" | "accounts" | "extensions" | "metrics" | "containers";
const targets: Target[] = ["scheduler", "workloads", "crds", "gc", "accounts", "extensions", "metrics", "containers"];
const SCHEMA_VERSION = 1;
const controllerAnnot = "k8flare.io/controller";
const NAMESPACE_PREFIX = "/registry/namespaces/";
const ACCOUNT_PREFIXES = [NAMESPACE_PREFIX, "/registry/serviceaccounts/", "/registry/configmaps/"];
const CRD_PREFIX = "/registry/apiextensions.k8s.io/customresourcedefinitions/";
const WORKLOAD_PREFIXES = ["/registry/replicasets/", "/registry/deployments/", "/registry/replicationcontrollers/", "/registry/services/", "/registry/endpoints/", "/registry/endpointslices/", "/registry/jobs/", "/registry/statefulsets/", "/registry/daemonsets/", "/registry/controllerrevisions/", "/registry/persistentvolumeclaims/", "/registry/persistentvolumes/", "/registry/storage.k8s.io/", "/registry/storageclasses/", "/registry/certificatesigningrequests/", "/registry/certificates.k8s.io/", "/registry/clusterroles/", "/registry/rbac.authorization.k8s.io/", "/registry/cronjobs/", "/registry/horizontalpodautoscalers/", "/registry/gateway.networking.k8s.io/", "/registry/resourcequotas/", "/registry/secrets/", "/registry/configmaps/", "/registry/poddisruptionbudgets/"];

export type QueueMessage =
  | { kind: "change"; key: string; type: string; rev: number }
  | { kind: "lease-check"; node: string }
  | { kind: "retry"; attempt?: number; changed?: string[]; names?: string[] };

function closeQuietly(ws: WebSocket, reason: string): void {
  console.log(`cluster: closing watch socket reason=${reason}`);
  try {
    ws.close(1000, reason);
  } catch {}
}

export class Cluster extends DurableObject<Env> {
  private watchers = new Map<WebSocket, Watcher>();

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    ctx.blockConcurrencyWhile(async () => {
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS kine (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        deleted INTEGER NOT NULL DEFAULT 0,
        value BLOB
      )`);
      ctx.storage.sql.exec(`CREATE INDEX IF NOT EXISTS kine_name_id ON kine (name, id)`);
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value INTEGER NOT NULL)`);
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS outbox (id INTEGER PRIMARY KEY AUTOINCREMENT, target TEXT NOT NULL, rev INTEGER NOT NULL, key TEXT NOT NULL, type TEXT NOT NULL)`);
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS lease_checks (node TEXT PRIMARY KEY, sent INTEGER NOT NULL)`);
      ctx.storage.sql.exec(
        `INSERT INTO kine (name, deleted, value) SELECT '/k8flare/bootstrap', 0, X'' WHERE NOT EXISTS (SELECT 1 FROM kine)`,
      );
      ctx.storage.sql.exec(
        `INSERT INTO meta (key, value) VALUES ('schema_version', ?) ON CONFLICT(key) DO NOTHING`,
        SCHEMA_VERSION,
      );
      this.sweepNamespaces();
      this.seedMetrics();
    });
  }

  private seedMetrics(): void {
    const now = Date.now();
    const rows = this.ctx.storage.sql.exec("SELECT value FROM meta WHERE key = 'metrics_seed'").toArray();
    const last = rows.length === 0 ? 0 : (rows[0].value as number);
    if (now - last < 15_000) return;
    this.ctx.storage.sql.exec(
      "INSERT INTO meta (key, value) VALUES ('metrics_seed', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
      now,
    );
    this.ctx.waitUntil(this.env.METRICS_Q.send({ kind: "retry" }));
  }

  private sweepNamespaces(): void {
    const now = Date.now();
    const rows = this.ctx.storage.sql.exec("SELECT value FROM meta WHERE key = 'namespace_sweep'").toArray();
    const last = rows.length === 0 ? 0 : (rows[0].value as number);
    if (now - last < 60_000) return;
    this.ctx.storage.sql.exec(
      "INSERT INTO meta (key, value) VALUES ('namespace_sweep', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
      now,
    );
    this.ctx.waitUntil(this.env.ACCT_Q.send({ kind: "retry" }));
  }

  private schemaVersion(): number {
    const rows = this.ctx.storage.sql.exec("SELECT value FROM meta WHERE key = 'schema_version'").toArray();
    return rows.length === 0 ? 0 : (rows[0].value as number);
  }

  private storageBytes(): number {
    const rows = this.ctx.storage.sql.exec("SELECT COALESCE(SUM(LENGTH(value)), 0) AS n FROM kine").toArray();
    return rows.length === 0 ? 0 : (rows[0].n as number);
  }

  private revision(): number {
    return this.ctx.storage.sql.exec("SELECT COALESCE(MAX(id), 0) AS rev FROM kine").one().rev as number;
  }

  private compactRevision(): number {
    const rows = this.ctx.storage.sql.exec("SELECT value FROM meta WHERE key = 'compact_revision'").toArray();
    return rows.length === 0 ? 0 : (rows[0].value as number);
  }

  private compactBefore(target: number): void {
    const removed = this.ctx.storage.sql.exec(
      `DELETE FROM kine WHERE id IN (
         SELECT old.id FROM kine AS old
         WHERE old.id <= ?
           AND (
             old.deleted = 1
             OR EXISTS (SELECT 1 FROM kine AS newer WHERE newer.name = old.name AND newer.id > old.id)
           )
         LIMIT ?
       )`,
      target,
      COMPACT_BATCH,
    ).rowsWritten;
    if (removed >= COMPACT_BATCH) return;
    this.ctx.storage.sql.exec(
      "INSERT INTO meta (key, value) VALUES ('compact_revision', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
      target,
    );
  }

  private current(name: string): KV | null {
    const rows = this.ctx.storage.sql
      .exec("SELECT id, name, deleted, value FROM kine WHERE id = (SELECT MAX(id) FROM kine WHERE name = ?)", name)
      .toArray();
    if (rows.length === 0 || rows[0].deleted) return null;
    return rowToKV(rows[0]);
  }

  private latest(prefix: string, exact: boolean, from: string, limit: number, at = 0): KV[] {
    const atRev = at > 0 ? at : Number.MAX_SAFE_INTEGER;
    return this.ctx.storage.sql
      .exec(
        `SELECT kv.id, kv.name, kv.deleted, kv.value FROM kine AS kv
         JOIN (SELECT MAX(id) AS id FROM kine WHERE name >= ? AND name < ? AND id <= ? GROUP BY name) AS latest ON latest.id = kv.id
         WHERE kv.deleted = 0 ORDER BY kv.name ASC LIMIT ?`,
        from > prefix ? from : prefix,
        rangeEnd(prefix, exact),
        atRev,
        limit,
      )
      .toArray()
      .map(rowToKV);
  }

  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/watch" && request.headers.get("Upgrade") === "websocket") {
      return this.watch(url);
    }
    switch (`${request.method} ${url.pathname}`) {
      case "GET /revision":
        return Response.json({ revision: this.revision() });
      case "GET /stats":
        return Response.json({
          revision: this.revision(),
          compactRevision: this.compactRevision(),
          schemaVersion: this.schemaVersion(),
          bytes: this.storageBytes(),
          rows: this.ctx.storage.sql.exec("SELECT COUNT(*) AS n FROM kine").one().n as number,
          outbox: this.ctx.storage.sql.exec("SELECT COUNT(*) AS n FROM outbox").one().n as number,
          watchers: this.ctx.getWebSockets().length,
        });
      case "GET /kv": {
        const kv = this.current(url.searchParams.get("key") ?? "");
        return Response.json({ revision: this.revision(), kv: kv && encodeKV(kv) });
      }
      case "PUT /kv": {
        const body = (await request.json()) as { key: string; value: string; revision: number };
        const cur = this.current(body.key);
        if (body.revision === 0 && cur) return conflict(this.revision(), "exists");
        if (body.revision !== 0 && !cur) return Response.json({ revision: this.revision() }, { status: 404 });
        if (body.revision !== 0 && cur!.modRevision !== body.revision) return conflict(this.revision(), "conflict");
        const rev = this.insert(body.key, 0, fromBase64(body.value), cur);
        return Response.json({ revision: rev }, { status: body.revision === 0 ? 201 : 200 });
      }
      case "DELETE /kv": {
        const body = (await request.json()) as { key: string; revision: number };
        const cur = this.current(body.key);
        if (!cur) return Response.json({ revision: this.revision() }, { status: 404 });
        if (body.revision !== 0 && cur.modRevision !== body.revision) return conflict(this.revision(), "conflict");
        return Response.json({ revision: this.insert(body.key, 1, cur.value, cur) });
      }
      case "POST /enqueue": {
        const body = (await request.json()) as { target: Target; delayMs: number };
        if (!targets.includes(body.target)) return new Response("unknown target", { status: 400 });
        const delaySeconds = Math.min(MAX_DELAY_S, Math.max(0, Math.ceil(body.delayMs / 1000)));
        await this.queue(body.target).send({ kind: "retry" } satisfies QueueMessage, { delaySeconds });
        return Response.json({ ok: true });
      }
      case "POST /snapshot": {
        const bucket = (this.env as Env & { PODS_R2?: R2Bucket }).PODS_R2;
        if (!bucket) return new Response("r2 unbound", { status: 503 });
        const rows = this.ctx.storage.sql
          .exec(
            `SELECT kv.id, kv.name, kv.deleted, kv.value FROM kine AS kv
             JOIN (SELECT MAX(id) AS id FROM kine GROUP BY name) AS latest ON latest.id = kv.id
             WHERE kv.deleted = 0 ORDER BY kv.name ASC`,
          )
          .toArray()
          .map(rowToKV);
        const revision = this.revision();
        const taken = new Date().toISOString();
        const cluster = (this.env as Env).CLUSTER_UID || "default";
        const object = `clusters/${cluster}/snapshots/${taken.replace(/[:.]/g, "-")}.json`;
        const body = JSON.stringify({
          schemaVersion: this.schemaVersion(),
          revision,
          taken,
          cluster,
          keys: rows.map(encodeKV),
        });
        await bucket.put(object, body, { httpMetadata: { contentType: "application/json" } });
        return Response.json({ ok: true, key: object, revision, count: rows.length, bytes: body.length });
      }
      case "POST /restore": {
        const at = parseRestoreTime(url.searchParams.get("to") ?? "");
        if (at === null) return new Response("invalid to", { status: 400 });
        const bookmark = await this.ctx.storage.getBookmarkForTime(at);
        await this.ctx.storage.onNextSessionRestoreBookmark(bookmark);
        this.ctx.abort("pitr restore");
        return Response.json({ ok: true, bookmark, to: new Date(at).toISOString() });
      }
      case "GET /list": {
        const prefix = url.searchParams.get("prefix") ?? "";
        const limit = Number(url.searchParams.get("limit") ?? "0");
        const at = Number(url.searchParams.get("revision") ?? "0");
        const compact = this.compactRevision();
        if (at > 0 && compact > 0 && at < compact) {
          return Response.json({ revision: this.revision(), compactRevision: compact, error: "compacted" }, { status: 410 });
        }
        const kvs = this.latest(prefix, false, url.searchParams.get("from") ?? prefix, limit > 0 ? limit + 1 : -1, at);
        const more = limit > 0 && kvs.length > limit;
        return Response.json({
          revision: at > 0 ? at : this.revision(),
          kvs: (more ? kvs.slice(0, limit) : kvs).map(encodeKV),
          more,
        });
      }
    }
    return new Response("not found", { status: 404 });
  }

  private insert(name: string, deleted: number, value: Uint8Array, prev: KV | null): number {
    const rev = this.ctx.storage.sql
      .exec(
        "INSERT INTO kine (name, deleted, value) VALUES (?, ?, ?) RETURNING id",
        name,
        deleted,
        value.buffer.slice(value.byteOffset, value.byteOffset + value.byteLength),
      )
      .one().id as number;
    if (rev - this.compactRevision() >= RETAINED_REVISIONS) this.compactBefore(rev - RETAINED_REVISIONS);
    const type = deleted ? "deleted" : prev ? "modified" : "created";
    this.record(name, type, rev, value, prev);
    this.restoreWatchers();
    const notified = new Set<WebSocket>();
    const matched = [...this.watchers].filter(([, w]) => (w.exact ? name === w.prefix : name.startsWith(w.prefix)));
    if (matched.length > 0) {
      const msg = JSON.stringify({
        rev,
        type,
        key: name,
        value: toBase64(value),
        prev: type === "modified" ? toBase64(prev!.value) : "",
      });
      for (const [ws] of matched) {
        notified.add(ws);
        try {
          ws.send(msg);
        } catch {
          closeQuietly(ws, "send failed");
        }
      }
    }
    if (name.startsWith(NODE_LEASE_PREFIX)) {
      for (const [ws] of this.watchers) {
        if (!notified.has(ws)) this.sendProgress(ws, rev);
      }
    }
    this.expireWatchers();
    return rev;
  }

  // A watcher is first caught up from `since`, or given the current state
  // when `initial` is set (followed by a snapshot-end marker), then receives
  // every later write.
  private watch(url: URL): Response {
    const watcher: Watcher = {
      prefix: url.searchParams.get("prefix") ?? "/",
      exact: url.searchParams.get("exact") === "1",
      openedAt: Date.now(),
    };
    const since = Number(url.searchParams.get("since") ?? "0");
    const initial = url.searchParams.get("initial") === "1";
    const pair = new WebSocketPair();
    const [client, server] = [pair[0], pair[1]];
    const compacted = this.compactRevision();
    if (!initial && since > 0 && since < compacted) {
      server.accept();
      server.send(JSON.stringify({ rev: compacted, type: "compacted", key: "", value: "", prev: "" }));
      server.close(1000, "compacted");
      return new Response(null, { status: 101, webSocket: client });
    }
    this.ctx.acceptWebSocket(server);
    server.serializeAttachment(watcher);
    this.restoreWatchers();
    this.expireWatchers();
    this.watchers.set(server, watcher);
    this.ctx.waitUntil(this.flushOutbox());
    this.ctx.waitUntil(this.armWatchLease());
    if (initial) {
      for (const kv of this.latest(watcher.prefix, watcher.exact, watcher.prefix, -1)) {
        server.send(JSON.stringify({ rev: kv.modRevision, type: "created", key: kv.key, value: toBase64(kv.value), prev: "" }));
      }
      server.send(JSON.stringify({ rev: this.revision(), type: "snapshot-end", key: "", value: "", prev: "" }));
    } else {
      const rows = this.ctx.storage.sql
        .exec(
          `SELECT id, name, deleted, value, prev FROM (
             SELECT id, name, deleted, value, LAG(value) OVER (PARTITION BY name ORDER BY id) AS prev
             FROM kine WHERE name >= ? AND name < ?)
           WHERE id > ? ORDER BY id ASC`,
          watcher.prefix,
          rangeEnd(watcher.prefix, watcher.exact),
          since,
        )
        .toArray();
      for (const r of rows) {
        const kv = rowToKV(r);
        const type = r.deleted ? "deleted" : r.prev ? "modified" : "created";
        const prev = type === "modified" ? toBase64(new Uint8Array(r.prev as ArrayBuffer)) : "";
        server.send(JSON.stringify({ rev: kv.modRevision, type, key: kv.key, value: toBase64(kv.value), prev }));
      }
    }
    return new Response(null, { status: 101, webSocket: client });
  }

  private expireWatchers(): void {
    const cutoff = Date.now() - WATCH_LEASE_MS;
    const rev = this.revision();
    for (const [ws, w] of this.watchers) {
      if (w.openedAt < cutoff) {
        this.watchers.delete(ws);
        this.sendProgress(ws, rev);
        closeQuietly(ws, "lease");
      }
    }
  }

  private sendProgress(ws: WebSocket, rev: number): void {
    try {
      ws.send(JSON.stringify({ rev, type: "progress", key: "", value: "", prev: "" }));
    } catch {
      closeQuietly(ws, "send failed");
    }
  }

  async alarm(): Promise<void> {
    this.restoreWatchers();
    this.expireWatchers();
    await this.armWatchLease();
  }

  private async armWatchLease(): Promise<void> {
    this.restoreWatchers();
    let next = 0;
    for (const w of this.watchers.values()) {
      const due = w.openedAt + WATCH_LEASE_MS;
      if (next === 0 || due < next) next = due;
    }
    if (next === 0) {
      await this.ctx.storage.deleteAlarm();
      return;
    }
    const soonest = Math.max(next, Date.now() + 1000);
    const existing = await this.ctx.storage.getAlarm();
    if (existing === null || existing > soonest) {
      await this.ctx.storage.setAlarm(soonest);
    }
  }

  private restoreWatchers(): void {
    const live = new Map<WebSocket, Watcher>();
    for (const ws of this.ctx.getWebSockets()) {
      const w = this.watchers.get(ws) ?? (ws.deserializeAttachment() as Watcher | null);
      if (w) live.set(ws, w);
    }
    this.watchers = live;
  }

  private queue(target: Target): Queue<QueueMessage> {
    if (target === "scheduler") return this.env.SCHED_Q;
    if (target === "workloads") return this.env.WL_Q;
    if (target === "crds") return this.env.CRD_Q;
    if (target === "gc") return this.env.GC_Q;
    if (target === "accounts") return this.env.ACCT_Q;
    if (target === "extensions") return this.env.EXT_Q;
    if (target === "metrics") return this.env.METRICS_Q;
    if (target === "containers") return this.env.CONTAINERS_Q;
    return this.env.CTRL_Q;
  }

  // record turns a committed write into queue messages: the scheduler hears
  // about unbound pods, pod deletions and node changes that affect
  // placement; the controllers hear about every registry write except
  // events and node leases. A node lease instead schedules a delayed
  // check, at most once per node every LEASE_CHECK_EVERY_MS.
  private record(name: string, type: string, rev: number, value: Uint8Array, prev: KV | null): void {
    if (!name.startsWith("/registry/") || name.startsWith("/registry/events/")) return;
    if (name.startsWith(NODE_LEASE_PREFIX)) {
      if (type !== "deleted") this.ctx.waitUntil(this.checkLeaseLater(name.slice(NODE_LEASE_PREFIX.length)));
      return;
    }
    const routes: Target[] = [];
    if (name.startsWith("/registry/endpointslices/") || name.startsWith("/registry/endpoints/")) {
      if (type !== "modified" || !prev || endpointPublishChanged(prev.value, value)) routes.push("workloads");
    } else if (WORKLOAD_PREFIXES.some((p) => name.startsWith(p))) routes.push("workloads");
    if (ACCOUNT_PREFIXES.some((p) => name.startsWith(p))) routes.push("accounts");
    if (name.startsWith(CRD_PREFIX)) routes.push("crds");
    if (isExtensionKey(name, value, type)) routes.push("extensions");
    if (type === "deleted" || collectable(value)) routes.push("gc");
    if (name.startsWith("/registry/pods/")) {
      if (type !== "modified" || !prev || podWorkChanged(prev.value, value)) routes.push("workloads");
      if (type === "deleted" || !podBound(value)) routes.push("scheduler");
      if (wantsContainers(value) || (prev && wantsContainers(prev.value))) routes.push("containers");
    } else if (name.startsWith("/registry/minions/") || name.startsWith("/registry/nodes/")) {
      if (type !== "modified" || !prev || nodeChanged(prev.value, value)) routes.push("scheduler", "workloads", "metrics");
    }
    for (const target of routes) {
      this.ctx.storage.sql.exec("INSERT INTO outbox (target, rev, key, type) VALUES (?, ?, ?, ?)", target, rev, name, type);
    }
    if (routes.length > 0) this.ctx.waitUntil(this.flushOutbox());
  }

  private async checkLeaseLater(node: string): Promise<void> {
    const now = Date.now();
    const rows = this.ctx.storage.sql.exec("SELECT sent FROM lease_checks WHERE node = ?", node).toArray();
    if (rows.length > 0 && now - (rows[0].sent as number) < LEASE_CHECK_EVERY_MS) return;
    this.ctx.storage.sql.exec(
      "INSERT INTO lease_checks (node, sent) VALUES (?, ?) ON CONFLICT(node) DO UPDATE SET sent = excluded.sent",
      node,
      now,
    );
    await this.env.CTRL_Q.send({ kind: "lease-check", node } satisfies QueueMessage, { delaySeconds: LEASE_CHECK_DELAY_S });
  }

  private flushing = false;
  private flushAgain = false;

  private async flushOutbox(): Promise<void> {
    if (this.flushing) {
      this.flushAgain = true;
      return;
    }
    this.flushing = true;
    try {
      do {
        this.flushAgain = false;
        for (;;) {
          const rows = this.ctx.storage.sql.exec("SELECT id, target, rev, key, type FROM outbox ORDER BY id LIMIT ?", OUTBOX_BATCH).toArray();
          if (rows.length === 0) break;
          for (const target of targets) {
            const batch = rows.filter((r) => r.target === target);
            if (batch.length === 0) continue;
            await this.queue(target).sendBatch(
              batch.map((r) => ({ body: { kind: "change", key: r.key as string, type: r.type as string, rev: r.rev as number } satisfies QueueMessage })),
            );
          }
          this.ctx.storage.sql.exec("DELETE FROM outbox WHERE id <= ?", rows[rows.length - 1].id);
        }
      } while (this.flushAgain);
    } catch (err) {
      console.error("outbox flush:", err);
    } finally {
      this.flushing = false;
      if (this.flushAgain) this.ctx.waitUntil(this.flushOutbox());
    }
  }

  async webSocketMessage(): Promise<void> {}

  async webSocketClose(ws: WebSocket): Promise<void> {
    this.watchers.delete(ws);
    closeQuietly(ws, "peer");
    this.ctx.waitUntil(this.armWatchLease());
  }

  async webSocketError(ws: WebSocket): Promise<void> {
    this.watchers.delete(ws);
    closeQuietly(ws, "error");
  }
}

interface Watcher {
  prefix: string;
  exact: boolean;
  openedAt: number;
}

interface KV {
  key: string;
  value: Uint8Array;
  modRevision: number;
}

function rowToKV(row: Record<string, SqlStorageValue>): KV {
  return { key: row.name as string, value: new Uint8Array(row.value as ArrayBuffer), modRevision: row.id as number };
}

function encodeKV(kv: KV) {
  return { key: kv.key, value: toBase64(kv.value), modRevision: kv.modRevision };
}

// rangeEnd is the exclusive upper bound of the key range for a prefix, or
// for one exact key.
function rangeEnd(prefix: string, exact: boolean): string {
  if (exact) return prefix + "\u0000";
  return prefix.slice(0, -1) + String.fromCharCode(prefix.charCodeAt(prefix.length - 1) + 1);
}

function toBase64(bytes: Uint8Array): string {
  let bin = "";
  for (let i = 0; i < bytes.length; i += 8192) {
    bin += String.fromCharCode.apply(null, bytes.subarray(i, i + 8192) as unknown as number[]);
  }
  return btoa(bin);
}

function fromBase64(s: string): Uint8Array {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function conflict(revision: number, error: string): Response {
  return Response.json({ revision, error }, { status: 409 });
}

function decodeJSON(value: Uint8Array): Record<string, any> | null {
  try {
    return JSON.parse(new TextDecoder().decode(value));
  } catch {
    return null;
  }
}

const builtinGroups = new Set([
  "apiextensions.k8s.io",
  "admissionregistration.k8s.io",
  "rbac.authorization.k8s.io",
  "authentication.k8s.io",
  "authorization.k8s.io",
  "coordination.k8s.io",
  "discovery.k8s.io",
  "node.k8s.io",
  "storage.k8s.io",
  "resource.k8s.io",
  "scheduling.k8s.io",
  "networking.k8s.io",
  "certificates.k8s.io",
  "flowcontrol.apiserver.k8s.io",
  "apiregistration.k8s.io",
]);

function isExtensionKey(name: string, value: Uint8Array, type: string): boolean {
  const parts = name.split("/");
  if (parts.length < 3 || !parts[2].includes(".")) return false;
  if (name.startsWith(CRD_PREFIX)) {
    const crd = decodeJSON(value) as { metadata?: { annotations?: Record<string, string> } } | null;
    return Boolean(crd?.metadata?.annotations?.[controllerAnnot]) || type === "deleted";
  }
  return !builtinGroups.has(parts[2]);
}

function collectable(value: Uint8Array): boolean {
  const meta = (decodeJSON(value) as { metadata?: { ownerReferences?: unknown[]; finalizers?: string[]; deletionTimestamp?: string } } | null)?.metadata;
  if (!meta) return false;
  if (meta.ownerReferences && meta.ownerReferences.length > 0) return true;
  if (meta.deletionTimestamp && meta.finalizers && meta.finalizers.length > 0) return true;
  return false;
}

function wantsContainers(value: Uint8Array): boolean {
  const pod = decodeJSON(value);
  return pod?.metadata?.annotations?.["k8flare.com/compute"] === "containers" || pod?.spec?.nodeSelector?.["k8flare.com/backend"] === "containers";
}

function podBound(value: Uint8Array): boolean {
  return Boolean(decodeJSON(value)?.spec?.nodeName);
}

function podWorkChanged(before: Uint8Array, after: Uint8Array): boolean {
  const a = decodeJSON(before);
  const b = decodeJSON(after);
  if (!a || !b) return true;
  return (
    JSON.stringify(a.spec ?? {}) !== JSON.stringify(b.spec ?? {}) ||
    JSON.stringify(a.metadata?.labels ?? {}) !== JSON.stringify(b.metadata?.labels ?? {}) ||
    a.metadata?.deletionTimestamp !== b.metadata?.deletionTimestamp ||
    a.status?.phase !== b.status?.phase ||
    a.status?.podIP !== b.status?.podIP ||
    podReady(a) !== podReady(b)
  );
}

function podReady(pod: Record<string, any> | null): string {
  const conditions = (pod?.status?.conditions ?? []) as { type: string; status: string }[];
  return conditions.find((c) => c.type === "Ready")?.status ?? "Unknown";
}

function readyStatus(node: Record<string, any> | null): string {
  const conditions = (node?.status?.conditions ?? []) as { type: string; status: string }[];
  return conditions.find((c) => c.type === "Ready")?.status ?? "Unknown";
}

export function parseRestoreTime(to: string): number | null {
  if (!to) return null;
  const n = Number(to);
  if (Number.isFinite(n) && n > 0) return n < 1e12 ? n * 1000 : n;
  const ms = Date.parse(to);
  return Number.isFinite(ms) ? ms : null;
}

function endpointPublishChanged(before: Uint8Array, after: Uint8Array): boolean {
  const a = decodeJSON(before);
  const b = decodeJSON(after);
  if (!a || !b) return true;
  return JSON.stringify(publishView(a)) !== JSON.stringify(publishView(b));
}

function publishView(obj: Record<string, any>): Record<string, any> {
  const meta = (obj.metadata ?? {}) as Record<string, any>;
  const annotations = { ...((meta.annotations ?? {}) as Record<string, string>) };
  delete annotations["endpoints.kubernetes.io/last-change-trigger-time"];
  return {
    labels: meta.labels ?? {},
    annotations,
    deletionTimestamp: meta.deletionTimestamp,
    subsets: obj.subsets,
    endpoints: obj.endpoints,
    ports: obj.ports,
    addressType: obj.addressType,
  };
}

function nodeChanged(before: Uint8Array, after: Uint8Array): boolean {
  const a = decodeJSON(before);
  const b = decodeJSON(after);
  if (!a || !b) return true;
  return (
    JSON.stringify(a.spec ?? {}) !== JSON.stringify(b.spec ?? {}) ||
    JSON.stringify(a.metadata?.labels ?? {}) !== JSON.stringify(b.metadata?.labels ?? {}) ||
    JSON.stringify(a.status?.allocatable ?? {}) !== JSON.stringify(b.status?.allocatable ?? {}) ||
    readyStatus(a) !== readyStatus(b)
  );
}
