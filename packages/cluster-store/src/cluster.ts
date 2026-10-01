import { DurableObject } from "cloudflare:workers";
import { compactionTarget, nextAlarmAt, snapshotSchedule, snapshotsToPrune, type SnapshotVars } from "./schedule.ts";

// Cluster holds one cluster's state: a kine-style revisioned key-value log
// in SQLite. Every write appends a row; the row id is the revision. A
// bootstrap row makes the first revision 1, as in kine, because a resource
// version of 0 is illegal for a list. Watchers are hibernatable WebSockets
// tagged with the key prefix they asked for.
const COMPACT_RETAIN_MS = 300_000;
const COMPACT_INTERVAL_MS = 300_000;
const MAX_RETAINED_REVISIONS = 100_000;
const COMPACT_SLACK_REVISIONS = 10_000;
const PROGRESS_INTERVAL_MS = 30_000;
const SNAPSHOT_RETRY_MS = 600_000;
const COMPACT_BATCH = 200;
const WATCH_LEASE_MS = 360_000;
const NODE_LEASE_PREFIX = "/registry/leases/kube-node-lease/";
const LEASE_CHECK_DELAY_S = 60;
const LEASE_CHECK_EVERY_MS = 50_000;
const OUTBOX_BATCH = 100;
const MAX_DELAY_S = 86_400;

type Target = "scheduler" | "workloads" | "crds" | "gc" | "accounts" | "extensions" | "metrics" | "containers" | "attachdetach" | "addons";
const targets: Target[] = ["scheduler", "workloads", "crds", "gc", "accounts", "extensions", "metrics", "containers", "attachdetach", "addons"];
const SCHEMA_VERSION = 1;
const controllerAnnot = "k8flare.io/controller";
const REGISTRY_PREFIX = "/registry/";
const NAMESPACE_PREFIX = "/registry/namespaces/";
const ACCOUNT_PREFIXES = [NAMESPACE_PREFIX, "/registry/serviceaccounts/", "/registry/configmaps/"];
const ADDON_PREFIX = "/registry/k3s.cattle.io/addons/";
const HELM_PREFIXES = ["/registry/helm.cattle.io/helmcharts/", "/registry/helm.cattle.io/helmchartconfigs/"];
const CRD_PREFIX = "/registry/apiextensions.k8s.io/customresourcedefinitions/";
const WORKLOAD_PREFIXES = ["/registry/replicasets/", "/registry/deployments/", "/registry/replicationcontrollers/", "/registry/services/", "/registry/endpoints/", "/registry/endpointslices/", "/registry/jobs/", "/registry/statefulsets/", "/registry/daemonsets/", "/registry/controllerrevisions/", "/registry/persistentvolumeclaims/", "/registry/persistentvolumes/", "/registry/storage.k8s.io/", "/registry/storageclasses/", "/registry/volumeattributesclasses/", "/registry/certificatesigningrequests/", "/registry/certificates.k8s.io/", "/registry/clusterroles/", "/registry/rbac.authorization.k8s.io/", "/registry/cronjobs/", "/registry/horizontalpodautoscalers/", "/registry/gateway.networking.k8s.io/", "/registry/ingresses/", "/registry/ingressclasses/", "/registry/resourcequotas/", "/registry/secrets/", "/registry/configmaps/", "/registry/poddisruptionbudgets/", "/registry/servicecidrs/", "/registry/validatingadmissionpolicies/", "/registry/resourceclaims/", "/registry/resourceslices/"];
const ATTACH_PREFIXES = ["/registry/pods/", "/registry/minions/", "/registry/nodes/", "/registry/persistentvolumeclaims/", "/registry/persistentvolumes/", "/registry/storage.k8s.io/", "/registry/storageclasses/"];
const SCHEDULER_VOLUME_PREFIXES = ["/registry/persistentvolumeclaims/", "/registry/persistentvolumes/", "/registry/storageclasses/", "/registry/csinodes/", "/registry/csidrivers/", "/registry/csistoragecapacities/", "/registry/volumeattachments/", "/registry/storage.k8s.io/", "/registry/resourceclaims/", "/registry/resourceslices/", "/registry/deviceclasses/"];

export type QueueMessage =
  | { kind: "change"; key: string; type: string; rev: number }
  | { kind: "lease-check"; node: string }
  | { kind: "retry"; attempt?: number; changed?: string[]; names?: string[] };

function versionStamp(version: string): number {
  let hash = 2166136261;
  for (let i = 0; i < version.length; i++) hash = Math.imul(hash ^ version.charCodeAt(i), 16777619) >>> 0;
  return hash;
}

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
        value BLOB,
        ts INTEGER NOT NULL DEFAULT 0
      )`);
      if (!ctx.storage.sql.exec("PRAGMA table_info(kine)").toArray().some((c) => c.name === "ts")) {
        ctx.storage.sql.exec("ALTER TABLE kine ADD COLUMN ts INTEGER NOT NULL DEFAULT 0");
      }
      ctx.storage.sql.exec(`CREATE INDEX IF NOT EXISTS kine_name_id ON kine (name, id)`);
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value INTEGER NOT NULL)`);
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS outbox (id INTEGER PRIMARY KEY AUTOINCREMENT, target TEXT NOT NULL, rev INTEGER NOT NULL, key TEXT NOT NULL, type TEXT NOT NULL)`);
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS passes (target TEXT PRIMARY KEY, triggered INTEGER NOT NULL, finished INTEGER NOT NULL)`);
      ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS lease_checks (node TEXT PRIMARY KEY, sent INTEGER NOT NULL, due INTEGER NOT NULL DEFAULT 0)`);
      if (!ctx.storage.sql.exec("PRAGMA table_info(lease_checks)").toArray().some((c) => c.name === "due")) {
        ctx.storage.sql.exec("ALTER TABLE lease_checks ADD COLUMN due INTEGER NOT NULL DEFAULT 0");
      }
      ctx.storage.sql.exec(
        `INSERT INTO kine (name, deleted, value) SELECT '/k8flare/bootstrap', 0, X'' WHERE NOT EXISTS (SELECT 1 FROM kine)`,
      );
      ctx.storage.sql.exec(
        `INSERT INTO meta (key, value) VALUES ('schema_version', ?) ON CONFLICT(key) DO NOTHING`,
        SCHEMA_VERSION,
      );
      this.sweepNamespaces();
      this.seedMetrics();
      this.seedAddons();
      ctx.waitUntil(this.armAlarm());
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

  private seedAddons(): void {
    const stamp = versionStamp(this.env.CF_VERSION?.id ?? "");
    const rows = this.ctx.storage.sql.exec("SELECT value FROM meta WHERE key = 'addons_seed'").toArray();
    if (rows.length > 0 && rows[0].value === stamp) return;
    this.ctx.waitUntil(
      (async () => {
        await this.env.ADDON_Q.send({ kind: "retry" } satisfies QueueMessage);
        this.ctx.storage.sql.exec(
          "INSERT INTO meta (key, value) VALUES ('addons_seed', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
          stamp,
        );
      })(),
    );
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

  private setMeta(key: string, value: number): void {
    this.ctx.storage.sql.exec("INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value);
  }

  private dueAt(key: string, intervalMs: number): number {
    const rows = this.ctx.storage.sql.exec("SELECT value FROM meta WHERE key = ?", key).toArray();
    if (rows.length > 0) return rows[0].value as number;
    const due = Date.now() + intervalMs;
    this.setMeta(key, due);
    return due;
  }

  private compactTo(target: number): void {
    if (target <= this.compactRevision()) return;
    this.setMeta("compact_revision", target);
    let removed: number;
    do {
      removed = this.ctx.storage.sql.exec(
        `DELETE FROM kine WHERE id IN (
           SELECT old.id FROM kine AS old
           WHERE old.id <= ?1
             AND (
               old.deleted = 1
               OR EXISTS (SELECT 1 FROM kine AS newer WHERE newer.name = old.name AND newer.id > old.id AND newer.id <= ?1)
             )
           ORDER BY old.id ASC
           LIMIT ?2
         )`,
        target,
        COMPACT_BATCH,
      ).rowsWritten;
    } while (removed >= COMPACT_BATCH);
  }

  private compactByTime(now: number): void {
    const newest = this.ctx.storage.sql.exec("SELECT id FROM kine WHERE ts <= ? ORDER BY id DESC LIMIT 1", now - COMPACT_RETAIN_MS).toArray();
    this.compactTo(compactionTarget({ revision: this.revision(), timeTarget: newest.length === 0 ? 0 : (newest[0].id as number), maxRetained: MAX_RETAINED_REVISIONS }));
    this.setMeta("compact_due", now + COMPACT_INTERVAL_MS);
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
      case "GET /health": {
        const now = Date.now();
        const failing = this.ctx.storage.sql.exec("SELECT value FROM meta WHERE key = 'flush_failed'").toArray();
        return Response.json({
          outbox: this.ctx.storage.sql.exec("SELECT COUNT(*) AS n FROM outbox").one().n as number,
          flushFailingMs: failing.length === 0 ? 0 : now - (failing[0].value as number),
          passes: this.ctx.storage.sql
            .exec("SELECT target, triggered, finished FROM passes ORDER BY target")
            .toArray()
            .map((r) => ({ target: r.target as string, pendingMs: (r.triggered as number) > (r.finished as number) ? now - (r.triggered as number) : 0 })),
        });
      }
      case "POST /pass": {
        const body = (await request.json()) as { target: Target };
        if (!targets.includes(body.target)) return new Response("unknown target", { status: 400 });
        this.ctx.storage.sql.exec(
          "INSERT INTO passes (target, triggered, finished) VALUES (?, 0, ?) ON CONFLICT(target) DO UPDATE SET finished = excluded.finished",
          body.target,
          Date.now(),
        );
        return Response.json({ ok: true });
      }
      case "POST /lease-check": {
        const body = (await request.json()) as { node: string; delayMs: number };
        if (!body.node) return new Response("missing node", { status: 400 });
        const scheduled = await this.scheduleLeaseCheck(body.node, Math.max(0, body.delayMs ?? 0));
        return Response.json({ scheduled });
      }
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
        const bucket = this.snapshotBucket();
        if (!bucket) return new Response("r2 unbound", { status: 503 });
        return Response.json(await this.takeSnapshot(bucket, false));
      }
      case "POST /progress": {
        this.restoreWatchers();
        this.sendProgressToAll();
        return Response.json({ ok: true, watchers: this.watchers.size });
      }
      case "POST /restore": {
        const at = parseRestoreTime(url.searchParams.get("to") ?? "");
        if (at === null) return new Response("invalid to", { status: 400 });
        const bookmark = await this.ctx.storage.getBookmarkForTime(at);
        await this.ctx.storage.onNextSessionRestoreBookmark(bookmark);
        return Response.json({ ok: true, bookmark, to: new Date(at).toISOString() });
      }
      case "POST /restore/apply": {
        this.ctx.abort("pitr restore");
        return Response.json({ ok: true });
      }
      case "GET /snapshots": {
        const bucket = (this.env as Env & { PODS_R2?: R2Bucket }).PODS_R2;
        if (!bucket) return new Response("r2 unbound", { status: 503 });
        const prefix = `clusters/${(this.env as Env).CLUSTER_UID || "default"}/snapshots/`;
        const items: { key: string; size: number; uploaded: string }[] = [];
        let cursor: string | undefined;
        do {
          const page = await bucket.list({ prefix, cursor });
          for (const o of page.objects) items.push({ key: o.key, size: o.size, uploaded: o.uploaded.toISOString() });
          cursor = page.truncated ? page.cursor : undefined;
        } while (cursor);
        return Response.json({ items });
      }
      case "POST /snapshot/restore": {
        const bucket = (this.env as Env & { PODS_R2?: R2Bucket }).PODS_R2;
        if (!bucket) return new Response("r2 unbound", { status: 503 });
        const body = (await request.json()) as { key?: string; force?: boolean };
        if (!body.key || !body.key.startsWith("clusters/") || !body.key.endsWith(".json")) {
          return Response.json({ error: "key must be a snapshot object under clusters/" }, { status: 400 });
        }
        const object = await bucket.get(body.key);
        if (!object) return Response.json({ error: "snapshot not found" }, { status: 404 });
        const snapshot = (await object.json()) as { schemaVersion: number; keys: { key: string; value: string }[] };
        if (snapshot.schemaVersion !== this.schemaVersion()) {
          return Response.json({ error: `snapshot schema ${snapshot.schemaVersion} does not match ${this.schemaVersion()}` }, { status: 409 });
        }
        const wanted = new Map(snapshot.keys.filter((k) => k.key.startsWith(REGISTRY_PREFIX)).map((k) => [k.key, fromBase64(k.value)]));
        const live = this.latest(REGISTRY_PREFIX, false, REGISTRY_PREFIX, -1);
        if (live.length > 0 && !body.force) {
          return Response.json({ error: "cluster is not empty; restoring replaces its contents, pass force to proceed", keys: live.length }, { status: 409 });
        }
        let removed = 0;
        let written = 0;
        for (const kv of live) {
          if (wanted.has(kv.key)) continue;
          this.insert(kv.key, 1, kv.value, kv);
          removed++;
        }
        for (const [key, value] of wanted) {
          const cur = this.current(key);
          if (cur && bytesEqual(cur.value, value)) continue;
          this.insert(key, 0, value, cur);
          written++;
        }
        return Response.json({ ok: true, key: body.key, written, removed, revision: this.revision() });
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
        "INSERT INTO kine (name, deleted, value, ts) VALUES (?, ?, ?, ?) RETURNING id",
        name,
        deleted,
        value.buffer.slice(value.byteOffset, value.byteOffset + value.byteLength),
        Date.now(),
      )
      .one().id as number;
    if (rev - this.compactRevision() >= MAX_RETAINED_REVISIONS + COMPACT_SLACK_REVISIONS) this.compactTo(rev - MAX_RETAINED_REVISIONS);
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
    if (this.progressDue <= Date.now()) this.progressDue = Date.now() + PROGRESS_INTERVAL_MS;
    this.ctx.waitUntil(this.flushOutbox());
    this.ctx.waitUntil(this.armAlarm());
    if (initial) {
      for (const kv of this.latest(watcher.prefix, watcher.exact, watcher.prefix, -1)) {
        server.send(JSON.stringify({ rev: kv.modRevision, type: "created", key: kv.key, value: toBase64(kv.value), prev: "" }));
      }
      server.send(JSON.stringify({ rev: this.revision(), type: "snapshot-end", key: "", value: "", prev: "" }));
    } else {
      const rows = this.ctx.storage.sql
        .exec(
          `SELECT id, name, deleted, value, prev, prev_deleted FROM (
             SELECT id, name, deleted, value, LAG(value) OVER (PARTITION BY name ORDER BY id) AS prev, LAG(deleted) OVER (PARTITION BY name ORDER BY id) AS prev_deleted
             FROM kine WHERE name >= ? AND name < ?)
           WHERE id > ? ORDER BY id ASC`,
          watcher.prefix,
          rangeEnd(watcher.prefix, watcher.exact),
          since,
        )
        .toArray();
      for (const r of rows) {
        const kv = rowToKV(r);
        const type = r.deleted ? "deleted" : r.prev_deleted === 0 ? "modified" : "created";
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

  private progressDue = 0;

  private sendProgressToAll(): void {
    const rev = this.revision();
    for (const ws of this.watchers.keys()) this.sendProgress(ws, rev);
    this.progressDue = Date.now() + PROGRESS_INTERVAL_MS;
  }

  async alarm(): Promise<void> {
    const now = Date.now();
    this.restoreWatchers();
    this.expireWatchers();
    if (this.watchers.size > 0 && now >= this.progressDue) this.sendProgressToAll();
    if (now >= this.dueAt("compact_due", COMPACT_INTERVAL_MS)) this.compactByTime(now);
    await this.takeScheduledSnapshot(now);
    await this.armAlarm();
  }

  private async armAlarm(): Promise<void> {
    this.restoreWatchers();
    const schedule = snapshotSchedule(this.env as Env & SnapshotVars);
    const next = nextAlarmAt(
      [
        ...[...this.watchers.values()].map((w) => w.openedAt + WATCH_LEASE_MS),
        this.watchers.size > 0 ? this.progressDue : null,
        this.dueAt("compact_due", COMPACT_INTERVAL_MS),
        schedule && this.snapshotBucket() ? this.dueAt("snapshot_due", schedule.intervalMs) : null,
      ],
      Date.now(),
    );
    const existing = await this.ctx.storage.getAlarm();
    if (next !== null && (existing === null || existing > next)) {
      await this.ctx.storage.setAlarm(next);
    }
  }

  private snapshotBucket(): R2Bucket | undefined {
    return (this.env as Env & { PODS_R2?: R2Bucket }).PODS_R2;
  }

  private snapshotPrefix(): string {
    return `clusters/${(this.env as Env).CLUSTER_UID || "default"}/snapshots/`;
  }

  private async takeSnapshot(bucket: R2Bucket, scheduled: boolean) {
    const rows = this.ctx.storage.sql
      .exec(
        `SELECT kv.id, kv.name, kv.deleted, kv.value FROM kine AS kv
         JOIN (SELECT MAX(id) AS id FROM kine GROUP BY name) AS latest ON latest.id = kv.id
         WHERE kv.deleted = 0 AND substr(kv.name, 1, 7) <> '/vault/' ORDER BY kv.name ASC`,
      )
      .toArray()
      .map(rowToKV);
    const revision = this.revision();
    const taken = new Date().toISOString();
    const cluster = (this.env as Env).CLUSTER_UID || "default";
    const object = `${this.snapshotPrefix()}${scheduled ? "scheduled-" : ""}${taken.replace(/[:.]/g, "-")}.json`;
    const body = JSON.stringify({
      schemaVersion: this.schemaVersion(),
      revision,
      taken,
      cluster,
      keys: rows.map(encodeKV),
    });
    await bucket.put(object, body, { httpMetadata: { contentType: "application/json" } });
    return { ok: true, key: object, revision, count: rows.length, bytes: body.length };
  }

  private async takeScheduledSnapshot(now: number): Promise<void> {
    const schedule = snapshotSchedule(this.env as Env & SnapshotVars);
    const bucket = this.snapshotBucket();
    if (!schedule || !bucket || now < this.dueAt("snapshot_due", schedule.intervalMs)) return;
    try {
      await this.takeSnapshot(bucket, true);
      const keys: string[] = [];
      let cursor: string | undefined;
      do {
        const page = await bucket.list({ prefix: `${this.snapshotPrefix()}scheduled-`, cursor });
        keys.push(...page.objects.map((o) => o.key));
        cursor = page.truncated ? page.cursor : undefined;
      } while (cursor);
      const stale = snapshotsToPrune(keys, `${this.snapshotPrefix()}scheduled-`, schedule.retention);
      if (stale.length > 0) await bucket.delete(stale);
      this.setMeta("snapshot_due", Date.now() + schedule.intervalMs);
    } catch (err) {
      console.error("scheduled snapshot:", err);
      this.setMeta("snapshot_due", Date.now() + Math.min(SNAPSHOT_RETRY_MS, schedule.intervalMs));
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
    if (target === "attachdetach") return this.env.AD_Q;
    if (target === "addons") return this.env.ADDON_Q;
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
    if (name.startsWith("/registry/pods/")) {
      if (type !== "modified" || !prev || podWorkChanged(prev.value, value)) routes.push("attachdetach");
    } else if (name.startsWith("/registry/nodes/") || name.startsWith("/registry/minions/")) {
      if (type !== "modified" || !prev || nodeChanged(prev.value, value)) routes.push("attachdetach");
    } else if (ATTACH_PREFIXES.some((p) => name.startsWith(p))) routes.push("attachdetach");
    if (SCHEDULER_VOLUME_PREFIXES.some((p) => name.startsWith(p))) routes.push("scheduler");
    if (ACCOUNT_PREFIXES.some((p) => name.startsWith(p))) routes.push("accounts");
    if (name.startsWith(CRD_PREFIX)) routes.push("crds");
    if (name.startsWith(ADDON_PREFIX) && type === "deleted") routes.push("addons");
    if (HELM_PREFIXES.some((p) => name.startsWith(p))) routes.push("addons");
    if (isExtensionKey(name, value, type)) routes.push("extensions");
    if (type === "deleted" || collectable(value, type === "modified" && prev ? prev.value : null)) routes.push("gc");
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
    await this.scheduleLeaseCheck(node, LEASE_CHECK_DELAY_S * 1000);
  }

  private async scheduleLeaseCheck(node: string, delayMs: number): Promise<boolean> {
    const now = Date.now();
    const rows = this.ctx.storage.sql.exec("SELECT due FROM lease_checks WHERE node = ?", node).toArray();
    if (rows.length > 0 && (rows[0].due as number) > now) return false;
    this.ctx.storage.sql.exec(
      "INSERT INTO lease_checks (node, sent, due) VALUES (?, ?, ?) ON CONFLICT(node) DO UPDATE SET sent = excluded.sent, due = excluded.due",
      node,
      now,
      now + delayMs,
    );
    await this.env.CTRL_Q.send({ kind: "lease-check", node } satisfies QueueMessage, { delaySeconds: Math.ceil(delayMs / 1000) });
    return true;
  }

  private markTriggered(target: Target): void {
    this.ctx.storage.sql.exec(
      "INSERT INTO passes (target, triggered, finished) VALUES (?, ?, 0) ON CONFLICT(target) DO UPDATE SET triggered = CASE WHEN finished >= triggered THEN excluded.triggered ELSE triggered END",
      target,
      Date.now(),
    );
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
            this.markTriggered(target);
          }
          this.ctx.storage.sql.exec("DELETE FROM outbox WHERE id <= ?", rows[rows.length - 1].id);
          this.ctx.storage.sql.exec("DELETE FROM meta WHERE key = 'flush_failed'");
        }
      } while (this.flushAgain);
    } catch (err) {
      this.ctx.storage.sql.exec("INSERT INTO meta (key, value) VALUES ('flush_failed', ?) ON CONFLICT(key) DO NOTHING", Date.now());
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
    this.ctx.waitUntil(this.armAlarm());
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

function bytesEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
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

function collectable(value: Uint8Array, before: Uint8Array | null): boolean {
  const meta = ownerMeta(value);
  if (!meta) return false;
  if (meta.deletionTimestamp && meta.finalizers && meta.finalizers.length > 0) return true;
  if (!meta.ownerReferences || meta.ownerReferences.length === 0) return false;
  if (!before) return true;
  return JSON.stringify(ownerMeta(before)?.ownerReferences ?? []) !== JSON.stringify(meta.ownerReferences);
}

function ownerMeta(value: Uint8Array): { ownerReferences?: unknown[]; finalizers?: string[]; deletionTimestamp?: string } | null {
  return (decodeJSON(value) as { metadata?: { ownerReferences?: unknown[]; finalizers?: string[]; deletionTimestamp?: string } } | null)?.metadata ?? null;
}

function wantsContainers(value: Uint8Array): boolean {
  const pod = decodeJSON(value);
  return pod?.metadata?.annotations?.["k8flare.com/compute"] === "containers" || pod?.spec?.nodeSelector?.["k8flare.com/backend"] === "containers" || pod?.spec?.nodeName === "cloudflare" || pod?.spec?.schedulerName === "k8flare-containers";
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
