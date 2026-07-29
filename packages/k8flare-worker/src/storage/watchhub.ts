// WatchHub: fans out watch events to hibernating client WebSockets,
// tag-filtered by prefix, preserving the order Cluster pushes them in.
//
// Design note (found empirically, 2026-07-02, superseding an earlier
// "single upstream WebSocket to Cluster" design): a Durable Object cannot
// call ctx.acceptWebSocket() on a WebSocket it received as `resp.webSocket`
// from calling fetch() on a *different* DO -- the platform rejects it with
// "Cannot call `acceptWebSocket()` on this WebSocket because its pair has
// already been accepted or used in a Response." Hibernatable accept is only
// for a WebSocketPair half this same fetch() invocation just created;
// relaying a socket obtained from another DO's response isn't supported.
// Reproduced with a single, first-ever watch connection on fresh state (not
// a race, not reconnect-specific) via `curl .../pods?watch=true`.
//
// So there is no "upstream WebSocket" here at all. Instead, Cluster pushes
// each event directly to WatchHub via a plain POST (see watch.ts's
// broadcastEvent) immediately after committing it -- ordering is preserved
// because Cluster awaits that push before returning from the write that
// produced it, so at most one push is ever in flight. This is simpler than
// the WebSocket-relay design it replaces (no upstream connect/reconnect
// bookkeeping) and keeps WatchHub hibernation-eligible between events: with
// no upstream socket to hold open, nothing keeps it artificially resident.
const LAST_SEEN_KEY = "lastSeenRevision";

interface WatchHubContext {
  id?: { name?: string };
  acceptWebSocket(ws: WebSocket, tags?: string[]): void;
  getWebSockets(tag?: string): WebSocket[];
  getTags(ws: WebSocket): string[];
  storage: {
    get<T = unknown>(key: string): Promise<T | undefined>;
    put(key: string, value: unknown): Promise<void>;
    deleteAll(): Promise<void>;
  };
}

interface ReplayBatch {
  events: any[];
  bookmark: number;
}

export class WatchHub {
  private ctx: WatchHubContext;
  private env: any;

  constructor(ctx: any, env: any) {
    this.ctx = ctx;
    this.env = env;
  }

  private clusterStub(): any {
    const ns = this.env.CLUSTER;
    // Multi-cluster: this hub's own instance name IS the cluster doName
    // (the gateway's wrapped WATCHHUB namespace and the Cluster DO's
    // push both address it that way).
    return ns.get(ns.idFromName(this.ctx.id?.name ?? "default"));
  }

  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);

    // Cluster teardown (clusters/api.ts): close every client socket and
    // drop hub state. Idempotent.
    if (url.pathname === "/admin/destroy" && request.method === "POST") {
      for (const ws of this.ctx.getWebSockets()) {
        try {
          ws.close(1001, "cluster deleted");
        } catch {
          // already closed
        }
      }
      await this.ctx.storage.deleteAll();
      return Response.json({ destroyed: true });
    }

    if (url.pathname === "/push" && request.method === "POST") {
      return this.handlePush(request);
    }

    if (request.headers.get("Upgrade") !== "websocket") {
      return new Response("Expected WebSocket", { status: 400 });
    }

    const prefix = url.searchParams.get("prefix") || "/";
    const revision = parseInt(url.searchParams.get("revision") || "0");

    const pair = new WebSocketPair();
    const [client, server] = Object.values(pair);
    this.ctx.acceptWebSocket(server, [prefix]);
    (server as any).serializeAttachment({ prefix, revision });

    // Seed this new client with its own initial batch; it then receives
    // live events via handlePush below, fed by Cluster's broadcastEvent.
    const { events, bookmark } = await this.replay(prefix, revision);
    for (const event of events) {
      server.send(JSON.stringify({ events: [event] }));
    }
    server.send(JSON.stringify({ bookmark }));

    return new Response(null, { status: 101, webSocket: client });
  }

  /** Cluster calls this immediately after committing a write, once per write. */
  private async handlePush(request: Request): Promise<Response> {
    const body: any = await request.json();
    const events: any[] = Array.isArray(body.events) ? body.events : [];
    const bookmark: number | undefined = body.bookmark;

    if (events.length > 0) {
      const clients = this.ctx.getWebSockets();
      if (clients.length > 0) {
        for (const event of events) {
          const key: unknown = event?.kv?.key;
          if (typeof key !== "string") continue;
          const msg = JSON.stringify({ events: [event] });
          for (const client of clients) {
            const tags = this.ctx.getTags(client);
            const prefix = tags[0] || "/";
            const matches = prefix.endsWith("/") ? key.startsWith(prefix) : key === prefix;
            if (matches) {
              try {
                client.send(msg);
              } catch {
                // A dead socket here is normal (hibernated peer went away);
                // the close handler reaps it.
              }
            }
          }
        }
      }
    }
    if (typeof bookmark === "number") {
      await this.ctx.storage.put(LAST_SEEN_KEY, bookmark);
    }
    return Response.json({ ok: true });
  }

  private async replay(prefix: string, revision: number): Promise<ReplayBatch> {
    const url = new URL("/replay", "http://do.internal");
    url.searchParams.set("prefix", prefix);
    url.searchParams.set("revision", String(revision));
    const resp = await this.clusterStub().fetch(new Request(url.toString()));
    return await resp.json();
  }

  async webSocketMessage(ws: WebSocket, message: string | ArrayBuffer): Promise<void> {
    if (message === "ping") ws.send("pong");
  }

  async webSocketClose(
    ws: WebSocket,
    code: number,
    reason: string,
    _wasClean: boolean,
  ): Promise<void> {
    // Acknowledge the close. Echoing the peer's code verbatim throws
    // InvalidAccessError for reserved codes -- an abruptly-killed client
    // (e.g. an evicted KCM dynamic-worker isolate) surfaces as 1006, and
    // an over-long reason also throws. Observed live 2026-07-25 as an
    // exception storm (one per subscribed watch) that skipped this ack
    // for every socket at once. Fall back to a bare legal close.
    try {
      ws.close(code, reason);
    } catch {
      ws.close(1000);
    }
  }

  async webSocketError(ws: WebSocket, _error: unknown): Promise<void> {
    ws.close(1011, "WebSocket error");
  }
}
