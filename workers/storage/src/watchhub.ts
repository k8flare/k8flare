// WatchHub: one upstream WebSocket to the Cluster DO's /watch firehose,
// re-broadcast (tag-filtered, preserving upstream order) to every
// subscribed client socket. Both legs use the WebSocket hibernation API
// (ctx.acceptWebSocket) -- cost invariant #4 -- so an idle hub (no client
// traffic, no upstream events) costs nothing beyond storage even while the
// upstream link stays logically "connected".
//
// Per docs/multi-tenancy-and-hosting.md: facets buy isolation/lifecycle, not
// their own thread, so a fan-out point that needs to serve up to 32,768
// client sockets (the platform's per-DO WebSocket ceiling) stays a
// top-level DO rather than a facet -- the one deliberate exception to
// "facet everything" in this phase.
const UPSTREAM_TAG = "__upstream__";
const LAST_SEEN_KEY = "lastSeenRevision";

interface WatchHubContext {
  acceptWebSocket(ws: WebSocket, tags?: string[]): void;
  getWebSockets(tag?: string): WebSocket[];
  getTags(ws: WebSocket): string[];
  storage: {
    get<T = unknown>(key: string): Promise<T | undefined>;
    put(key: string, value: unknown): Promise<void>;
  };
}

interface ReplayBatch {
  events: any[];
  bookmark: number;
}

export class WatchHub {
  private ctx: WatchHubContext;
  private env: any;
  private upstreamConnecting: Promise<void> | null = null;

  constructor(ctx: any, env: any) {
    this.ctx = ctx;
    this.env = env;
  }

  private clusterStub(): any {
    const ns = this.env.CLUSTER;
    return ns.get(ns.idFromName("default"));
  }

  async fetch(request: Request): Promise<Response> {
    if (request.headers.get("Upgrade") !== "websocket") {
      return new Response("Expected WebSocket", { status: 400 });
    }

    const url = new URL(request.url);
    const prefix = url.searchParams.get("prefix") || "/";
    const revision = parseInt(url.searchParams.get("revision") || "0");

    await this.ensureUpstream();

    const pair = new WebSocketPair();
    const [client, server] = Object.values(pair);
    this.ctx.acceptWebSocket(server, [prefix]);
    (server as any).serializeAttachment({ prefix, revision });

    // Seed just this new client with its own initial batch; it then
    // receives live events from the shared upstream firehose below.
    const { events, bookmark } = await this.replay(prefix, revision);
    for (const event of events) {
      server.send(JSON.stringify({ events: [event] }));
    }
    server.send(JSON.stringify({ bookmark }));

    return new Response(null, { status: 101, webSocket: client });
  }

  /** Open the shared upstream connection if it isn't already open. */
  private async ensureUpstream(): Promise<void> {
    if (this.ctx.getWebSockets(UPSTREAM_TAG).length > 0) return;
    if (!this.upstreamConnecting) {
      this.upstreamConnecting = this.openUpstream().finally(() => {
        this.upstreamConnecting = null;
      });
    }
    await this.upstreamConnecting;
  }

  private async openUpstream(): Promise<void> {
    if (this.ctx.getWebSockets(UPSTREAM_TAG).length > 0) return; // lost the race, someone else already opened it
    const lastSeen = (await this.ctx.storage.get<number>(LAST_SEEN_KEY)) || 0;
    const wsUrl = new URL("/watch", "http://do.internal");
    wsUrl.searchParams.set("prefix", "/");
    wsUrl.searchParams.set("revision", String(lastSeen));
    const resp = await this.clusterStub().fetch(
      new Request(wsUrl.toString(), { headers: { Upgrade: "websocket" } }),
    );
    const ws = resp.webSocket;
    if (!ws) throw new Error("failed to establish upstream watch connection to Cluster");
    this.ctx.acceptWebSocket(ws, [UPSTREAM_TAG]);
  }

  private async replay(prefix: string, revision: number): Promise<ReplayBatch> {
    const url = new URL("/replay", "http://do.internal");
    url.searchParams.set("prefix", prefix);
    url.searchParams.set("revision", String(revision));
    const resp = await this.clusterStub().fetch(new Request(url.toString()));
    return await resp.json();
  }

  async webSocketMessage(ws: WebSocket, message: string | ArrayBuffer): Promise<void> {
    if (this.ctx.getTags(ws).includes(UPSTREAM_TAG)) {
      await this.handleUpstreamMessage(message);
      return;
    }
    if (message === "ping") ws.send("pong");
  }

  private async handleUpstreamMessage(message: string | ArrayBuffer): Promise<void> {
    if (typeof message !== "string") return;
    let data: any;
    try {
      data = JSON.parse(message);
    } catch {
      return;
    }

    if (typeof data.bookmark === "number") {
      await this.ctx.storage.put(LAST_SEEN_KEY, data.bookmark);
    }
    if (!Array.isArray(data.events) || data.events.length === 0) return;

    const clients = this.ctx
      .getWebSockets()
      .filter((s) => !this.ctx.getTags(s).includes(UPSTREAM_TAG));
    let maxRev = 0;
    for (const event of data.events) {
      const key: unknown = event?.kv?.key;
      if (typeof key !== "string") continue;
      if (typeof event?.kv?.modRevision === "number")
        maxRev = Math.max(maxRev, event.kv.modRevision);
      if (clients.length === 0) continue; // still track maxRev even with nobody listening
      const msg = JSON.stringify({ events: [event] });
      for (const client of clients) {
        const tags = this.ctx.getTags(client);
        const prefix = tags[0] || "/";
        const matches = prefix.endsWith("/") ? key.startsWith(prefix) : key === prefix;
        if (matches) {
          try {
            client.send(msg);
          } catch (_) {}
        }
      }
    }
    if (maxRev > 0) await this.ctx.storage.put(LAST_SEEN_KEY, maxRev);
  }

  async webSocketClose(
    ws: WebSocket,
    code: number,
    reason: string,
    _wasClean: boolean,
  ): Promise<void> {
    const isUpstream = this.ctx.getTags(ws).includes(UPSTREAM_TAG);
    ws.close(code, reason);
    if (isUpstream) return; // reopens lazily on the next client fetch()/message

    // If that was the last client, close the upstream link too -- no
    // reason to keep a Cluster connection open with nobody listening
    // (demand-start applied to the fan-out link itself, cost invariant #1).
    const remainingClients = this.ctx
      .getWebSockets()
      .filter((s) => s !== ws && !this.ctx.getTags(s).includes(UPSTREAM_TAG));
    if (remainingClients.length === 0) {
      for (const up of this.ctx.getWebSockets(UPSTREAM_TAG)) {
        try {
          up.close(1000, "no clients remaining");
        } catch (_) {}
      }
    }
  }

  async webSocketError(ws: WebSocket, _error: unknown): Promise<void> {
    ws.close(1011, "WebSocket error");
  }
}
