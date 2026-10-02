import { DatabaseSync } from "node:sqlite";
import { Cluster } from "../src/cluster.ts";

const NativeResponse = globalThis.Response;

class UpgradeResponse extends NativeResponse {
  webSocket?: FakeSocket;
  constructor(body?: BodyInit | null, init?: ResponseInit & { webSocket?: FakeSocket }) {
    const { webSocket, status, ...rest } = init ?? {};
    super(body, { ...rest, status: status === 101 ? 200 : status });
    this.webSocket = webSocket;
  }
}

export class FakeSocket {
  sent: any[] = [];
  closed: string | null = null;
  attachment: unknown = null;
  accept(): void {}
  send(data: string): void {
    this.sent.push(JSON.parse(data));
  }
  close(_code: number, reason: string): void {
    this.closed = reason;
  }
  serializeAttachment(value: unknown): void {
    this.attachment = structuredClone(value);
  }
  deserializeAttachment(): unknown {
    return this.attachment;
  }
}

class FakeSocketPair {
  0 = new FakeSocket();
  1 = new FakeSocket();
}

Object.assign(globalThis, { Response: UpgradeResponse, WebSocketPair: FakeSocketPair });

function bind(value: unknown): unknown {
  return value instanceof ArrayBuffer ? new Uint8Array(value) : value;
}

class FakeSql {
  db = new DatabaseSync(":memory:");
  exec(query: string, ...bindings: unknown[]) {
    const stmt = this.db.prepare(query);
    const args = bindings.map(bind) as any[];
    if (/^\s*(SELECT|PRAGMA|WITH)/i.test(query) || /\bRETURNING\b/i.test(query)) {
      const rows = stmt.all(...args) as Record<string, unknown>[];
      return { rowsWritten: 0, toArray: () => rows, one: () => rows[0] };
    }
    const info = stmt.run(...args);
    return { rowsWritten: Number(info.changes), toArray: () => [], one: () => undefined };
  }
}

export class FakeBucket {
  objects = new Map<string, { body: string; uploaded: Date }>();
  async put(key: string, body: string) {
    this.objects.set(key, { body, uploaded: new Date() });
  }
  async get(key: string) {
    const o = this.objects.get(key);
    return o && { json: async () => JSON.parse(o.body) };
  }
  async delete(keys: string | string[]) {
    for (const key of Array.isArray(keys) ? keys : [keys]) this.objects.delete(key);
  }
  async list({ prefix = "" }: { prefix?: string; cursor?: string } = {}) {
    const objects = [...this.objects]
      .filter(([key]) => key.startsWith(prefix))
      .sort(([a], [b]) => (a < b ? -1 : 1))
      .map(([key, o]) => ({ key, size: o.body.length, uploaded: o.uploaded }));
    return { objects, truncated: false, cursor: undefined };
  }
}

export interface Rig {
  cluster: Cluster;
  sql: FakeSql;
  bucket: FakeBucket;
  sockets: FakeSocket[];
  alarm: { at: number | null };
  settle(): Promise<void>;
  fire(): Promise<void>;
  put(key: string, value: string, revision?: number): Promise<Response>;
  remove(key: string): Promise<Response>;
  watch(query: Record<string, string>): FakeSocket;
  get(path: string): Promise<any>;
  post(path: string, body?: unknown): Promise<Response>;
}

export function rig(vars: Record<string, string> = {}): Rig {
  const sql = new FakeSql();
  const alarm: { at: number | null } = { at: null };
  const pending: Promise<unknown>[] = [];
  const sockets: FakeSocket[] = [];
  const queue = { send: async () => {}, sendBatch: async () => {} };
  const bucket = new FakeBucket();
  const ctx = {
    storage: {
      sql,
      getAlarm: async () => alarm.at,
      setAlarm: async (at: number) => void (alarm.at = at),
      deleteAlarm: async () => void (alarm.at = null),
    },
    waitUntil: (p: Promise<unknown>) => void pending.push(p),
    blockConcurrencyWhile: (fn: () => Promise<void>) => fn(),
    acceptWebSocket: (ws: FakeSocket) => void sockets.push(ws),
    getWebSockets: () => sockets.filter((s) => s.closed === null),
  };
  const env = { CTRL_Q: queue, SCHED_Q: queue, WL_Q: queue, CRD_Q: queue, GC_Q: queue, ACCT_Q: queue, EXT_Q: queue, CONTAINERS_Q: queue, AD_Q: queue, ADDON_Q: queue, HPA_Q: queue, PODS_R2: bucket, CLUSTER_UID: "test", ...vars };
  const cluster = new Cluster(ctx as any, env as any);
  const call = (path: string, init?: RequestInit) => cluster.fetch(new Request(`http://cluster.internal${path}`, init));
  const json = (method: string, path: string, body: unknown) => call(path, { method, body: JSON.stringify(body), headers: { "content-type": "application/json" } });
  const b64 = (s: string) => Buffer.from(s).toString("base64");
  return {
    cluster,
    sql,
    bucket,
    sockets,
    alarm,
    fire: async () => {
      alarm.at = null;
      await cluster.alarm();
    },
    settle: async () => {
      while (pending.length) await pending.shift();
    },
    put: (key, value, revision = 0) => json("PUT", "/kv", { key, value: b64(value), revision }),
    remove: (key) => json("DELETE", "/kv", { key, revision: 0 }),
    watch: (query) => {
      const before = sockets.length;
      (cluster as any).watch(new URL(`http://cluster.internal/watch?${new URLSearchParams(query)}`));
      return sockets[before];
    },
    get: async (path) => (await call(path)).json(),
    post: (path, body) => json("POST", path, body ?? {}),
  };
}
