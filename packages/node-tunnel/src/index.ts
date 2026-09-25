import { DurableObject } from "cloudflare:workers";
import "../../control-plane-worker/assets/wasm/wasm_exec.js";
import nodeTunnelWasm from "../assets/node-tunnel.wasm";

declare const Go: new () => {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance, context: unknown): Promise<void>;
};

interface RequestOut {
  status: number;
  headers: [string, string][];
  body: Uint8Array | null;
}

interface Binding {
  handleRequest(req: unknown, env: unknown): Promise<RequestOut>;
  attach(nodeName: string, send: (bytes: Uint8Array) => void): void;
  message(bytes: Uint8Array): void;
  closed(): void;
  upgrade(url: string, headers: [string, string][], send: (bytes: Uint8Array) => void, onErr?: (err: string) => void, query?: string): void;
  upgradeMessage(bytes: Uint8Array): void;
  upgradeClosed(): void;
  tick(): void;
}

function stringVar(env: Env, key: string): string {
  return (env as unknown as Record<string, string | undefined>)[key] ?? "";
}

type SockKind = { kind: "agent" | "stream" };

function sockKind(ws: WebSocket): SockKind["kind"] {
  return (ws.deserializeAttachment() as SockKind | null)?.kind ?? "agent";
}

function toBytes(data: string | ArrayBuffer | ArrayBufferView): Uint8Array {
  if (typeof data === "string") {
    const out = new Uint8Array(data.length);
    for (let i = 0; i < data.length; i++) out[i] = data.charCodeAt(i) & 0xff;
    return out;
  }
  if (data instanceof ArrayBuffer) return new Uint8Array(data);
  return new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
}

function instantiate(env: Record<string, unknown>): Promise<Binding> {
  return new Promise((resolve, reject) => {
    const go = new Go();
    const binding: Partial<Binding> = {};
    const context = { env, ctx: null, binding, ready: () => resolve(binding as Binding) };
    go.run(new WebAssembly.Instance(nodeTunnelWasm as WebAssembly.Module, go.importObject), context).then(
      () => reject(new Error("node-tunnel: go program exited before ready")),
      (err: unknown) => reject(err),
    );
  });
}
export class NodeTunnel extends DurableObject<Env> {
  private goInstance: Promise<Binding> | null = null;
  private attached = false;

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    ctx.blockConcurrencyWhile(() => this.rebind());
  }

  private async rebind(): Promise<void> {
    const sockets = this.ctx.getWebSockets();
    if (sockets.length === 0) return;
    const nodeName = (await this.ctx.storage.get<string>("node")) ?? "";
    if (!nodeName) {
      for (const ws of sockets) ws.close(1012, "no tunnel session, reconnect");
      return;
    }
    const binding = await this.go();
    const server = sockets.find((ws) => sockKind(ws) === "agent");
    if (!server) return;
    binding.attach(nodeName, (bytes) => {
      try {
        server.send(bytes);
      } catch {
        server.close(1011, "send failed");
      }
    });
    this.attached = true;
  }

  private go(): Promise<Binding> {
    if (!this.goInstance) {
      this.goInstance = instantiate({
        ADMIN_TOKEN: this.env.ADMIN_TOKEN,
        KUBELET_CLIENT_CERT: stringVar(this.env, "KUBELET_CLIENT_CERT"),
        KUBELET_CLIENT_KEY: stringVar(this.env, "KUBELET_CLIENT_KEY"),
        KUBELET_CA: stringVar(this.env, "KUBELET_CA"),
      });
    }
    return this.goInstance;
  }

  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/v1-k3s/connect") return this.acceptTunnel(request);
    if (url.pathname === "/stream-write" && (request.headers.get("Upgrade") || "").toLowerCase() === "websocket") {
      return this.acceptStreamWrite();
    }
    if ((request.headers.get("Upgrade") || "").toLowerCase() === "websocket" && url.pathname.startsWith("/node/")) {
      return this.acceptStream(request);
    }
    const binding = await this.go();
    const raw = await request.arrayBuffer();
    const out = await binding.handleRequest(
      { method: request.method, url: request.url, headers: [...request.headers], body: raw.byteLength === 0 ? null : new Uint8Array(raw) },
      {},
    );
    return new Response(out.body, { status: out.status, headers: out.headers });
  }

  private async acceptTunnel(request: Request): Promise<Response> {
    if (request.headers.get("Upgrade") !== "websocket") {
      return new Response("websocket upgrade required", { status: 426 });
    }
    const nodeName = request.headers.get("X-K8flare-Node") ?? "";
    if (!nodeName) return new Response("missing node name", { status: 400 });
    const pair = new WebSocketPair();
    const [client, server] = [pair[0], pair[1]];
    server.serializeAttachment({ kind: "agent" } satisfies SockKind);
    this.ctx.acceptWebSocket(server);
    await this.ctx.storage.put("node", nodeName);
    const binding = await this.go();
    binding.attach(nodeName, (bytes) => {
      try {
        server.send(bytes);
      } catch {
        server.close(1011, "send failed");
      }
    });
    this.attached = true;
    return new Response(null, { status: 101, webSocket: client });
  }

  private async acceptStream(request: Request): Promise<Response> {
    const pair = new WebSocketPair();
    const server = pair[1];
    server.serializeAttachment({ kind: "stream" } satisfies SockKind);
    this.ctx.acceptWebSocket(server);
    const streamURL = new URL(request.url);
    const streamQuery = request.headers.get("X-Stream-Query") ?? "";
    if (streamQuery && !streamURL.search) streamURL.search = streamQuery;
    console.log(`stream accept ${streamURL.pathname} q=${streamURL.search.length} xq=${streamQuery.length}`);
    const binding = await this.go();
    const headers: [string, string][] = [];
    request.headers.forEach((value, key) => headers.push([key, value]));
    if (streamQuery) headers.push(["X-Stream-Query", streamQuery]);
    let inflight = 0;
    let pendingClose: (() => void) | null = null;
    let finished: () => void = () => {};
    const done = new Promise<void>((resolve) => {
      finished = resolve;
    });
    this.ctx.waitUntil(done);
    const flushClose = () => {
      if (pendingClose && inflight === 0) {
        const fn = pendingClose;
        pendingClose = null;
        fn();
      }
    };
    binding.upgrade(
      streamURL.toString(),
      headers,
      (bytes: Uint8Array) => {
        try {
          const copy = new Uint8Array(bytes.byteLength);
          copy.set(bytes);
          inflight++;
          server.send(copy);
          console.log(`stream send ${copy.byteLength}`);
          queueMicrotask(() => {
            inflight--;
            flushClose();
          });
        } catch {
          server.close(1011, "send failed");
        }
      },
      (err: string) => {
        const finish = () => {
          try {
            if (err) server.close(1011, String(err).slice(0, 120));
            else server.close(1000, "done");
          } catch {}
          finished();
        };
        pendingClose = finish;
        flushClose();
        setTimeout(() => {
          if (pendingClose) {
            pendingClose();
            pendingClose = null;
          }
        }, 100);
      },
      streamURL.search.slice(1) || streamQuery,
    );
    const proto = request.headers.get("Sec-WebSocket-Protocol") ?? "";
    const upgradeHeaders = new Headers();
    if (proto) upgradeHeaders.set("Sec-WebSocket-Protocol", proto.split(",")[0].trim());
    return new Response(null, { status: 101, webSocket: pair[0], headers: upgradeHeaders });
  }

  private acceptStreamWrite(): Response {
    const pair = new WebSocketPair();
    const server = pair[1];
    server.serializeAttachment({ kind: "stream" } satisfies SockKind);
    this.ctx.acceptWebSocket(server);
    return new Response(null, { status: 101, webSocket: pair[0] });
  }

  async openKubeletStream(
    url: string,
    headers: [string, string][],
    query: string,
    stdin?: ReadableStream<Uint8Array>,
    initial?: ArrayBuffer[],
  ): Promise<ReadableStream<Uint8Array>> {
    const { readable, writable } = new TransformStream<Uint8Array, Uint8Array>();
    const writer = writable.getWriter();
    const binding = await this.go();
    binding.upgrade(
      url,
      headers,
      (bytes: Uint8Array) => {
        const copy = new Uint8Array(bytes.byteLength);
        copy.set(bytes);
        void writer.write(copy).catch(() => {});
      },
      (err: string) => {
        if (err) void writer.abort(new Error(err)).catch(() => {});
        else void writer.close().catch(() => {});
      },
      query,
    );
    if (initial) {
      for (const chunk of initial) {
        const bytes = new Uint8Array(chunk);
        if (bytes.byteLength === 0) continue;
        console.log(`stream-rpc initial ${bytes.byteLength}`);
        binding.upgradeMessage(bytes);
      }
    }
    if (stdin) {
      this.ctx.waitUntil(this.pumpStdin(stdin, binding));
    }
    return readable;
  }

  private async pumpStdin(stdin: ReadableStream<Uint8Array>, binding: Binding): Promise<void> {
    const reader = stdin.getReader();
    try {
      for (;;) {
        const { value, done } = await reader.read();
        if (done) {
          binding.upgradeClosed();
          return;
        }
        if (value) {
          console.log(`stream-rpc in ${value.byteLength}`);
          binding.upgradeMessage(value);
        }
      }
    } catch {
      try {
        binding.upgradeClosed();
      } catch {}
    }
  }

  async pushStdin(chunk: ArrayBuffer): Promise<void> {
    const bytes = new Uint8Array(chunk);
    console.log(`stream-rpc in ${bytes.byteLength}`);
    const binding = await this.go();
    binding.upgradeMessage(bytes);
  }

  async closeStdin(): Promise<void> {
    try {
      (await this.go()).upgradeClosed();
    } catch {}
  }

  async webSocketMessage(ws: WebSocket, message: string | ArrayBuffer): Promise<void> {
    const kind = sockKind(ws);
    if (kind === "stream") {
      const bytes = toBytes(message);
      console.log(`stream-write in ${bytes.byteLength}`);
      const binding = await this.go();
      binding.upgradeMessage(bytes);
      return;
    }
    if (!this.attached) {
      ws.close(1012, "no tunnel session, reconnect");
      return;
    }
    const binding = await this.go();
    binding.message(toBytes(message));
  }

  async webSocketClose(ws: WebSocket): Promise<void> {
    if (sockKind(ws) === "stream") {
      try {
        (await this.go()).upgradeClosed();
      } catch {}
      return;
    }
    if (this.attached) (await this.go()).closed();
    this.attached = false;
    await this.ctx.storage.delete("node");
  }

  async webSocketError(ws: WebSocket): Promise<void> {
    if (sockKind(ws) === "stream") {
      try {
        (await this.go()).upgradeClosed();
      } catch {}
      return;
    }
    if (this.attached) (await this.go()).closed();
    this.attached = false;
    await this.ctx.storage.delete("node");
  }
}
