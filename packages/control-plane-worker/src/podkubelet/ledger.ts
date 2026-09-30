import { DurableObject } from "cloudflare:workers";
import { pickPodIP } from "./spec.ts";

export interface LedgerEntry {
  uid: string;
  namespace: string;
  name: string;
  podIP: string;
  running: boolean;
  updatedAt: number;
}

export class PodLedger extends DurableObject<Env> {
  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    ctx.blockConcurrencyWhile(async () => {
      ctx.storage.sql.exec(
        "CREATE TABLE IF NOT EXISTS pods (uid TEXT PRIMARY KEY, namespace TEXT NOT NULL, name TEXT NOT NULL, pod_ip TEXT NOT NULL, running INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL)",
      );
      ctx.storage.sql.exec("CREATE UNIQUE INDEX IF NOT EXISTS pods_ip ON pods (pod_ip)");
      ctx.storage.sql.exec("CREATE INDEX IF NOT EXISTS pods_key ON pods (namespace, name)");
    });
  }

  private row(r: Record<string, SqlStorageValue>): LedgerEntry {
    return { uid: r.uid as string, namespace: r.namespace as string, name: r.name as string, podIP: r.pod_ip as string, running: Boolean(r.running), updatedAt: r.updated_at as number };
  }

  async claim(uid: string, namespace: string, name: string, cidr: string): Promise<LedgerEntry> {
    const existing = this.ctx.storage.sql.exec("SELECT * FROM pods WHERE uid = ?", uid).toArray();
    if (existing.length > 0) return this.row(existing[0]);
    const taken = new Set(this.ctx.storage.sql.exec("SELECT pod_ip FROM pods").toArray().map((r) => r.pod_ip as string));
    const podIP = pickPodIP(cidr, taken);
    if (!podIP) throw new Error(`no free Pod IP left in ${cidr}`);
    const now = Date.now();
    this.ctx.storage.sql.exec("INSERT INTO pods (uid, namespace, name, pod_ip, running, updated_at) VALUES (?, ?, ?, ?, 0, ?)", uid, namespace, name, podIP, now);
    return { uid, namespace, name, podIP, running: false, updatedAt: now };
  }

  async setRunning(uid: string, running: boolean): Promise<void> {
    this.ctx.storage.sql.exec("UPDATE pods SET running = ?, updated_at = ? WHERE uid = ?", running ? 1 : 0, Date.now(), uid);
  }

  async release(uid: string): Promise<void> {
    this.ctx.storage.sql.exec("DELETE FROM pods WHERE uid = ?", uid);
  }

  async lookup(namespace: string, name: string): Promise<LedgerEntry[]> {
    return this.ctx.storage.sql.exec("SELECT * FROM pods WHERE namespace = ? AND name = ?", namespace, name).toArray().map((r) => this.row(r));
  }

  async byIP(podIP: string): Promise<LedgerEntry | null> {
    const rows = this.ctx.storage.sql.exec("SELECT * FROM pods WHERE pod_ip = ?", podIP).toArray();
    return rows.length > 0 ? this.row(rows[0]) : null;
  }

  async list(): Promise<LedgerEntry[]> {
    return this.ctx.storage.sql.exec("SELECT * FROM pods ORDER BY updated_at").toArray().map((r) => this.row(r));
  }
}
